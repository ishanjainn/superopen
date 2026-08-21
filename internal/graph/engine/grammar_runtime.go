package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// GrammarRuntime owns compiled Tree-sitter grammar WASM modules. Parsing is
// enabled only after every pinned module is embedded and validated; this layer
// establishes the pure-Go sandbox and its resource boundary without claiming
// grammar readiness early.
type GrammarRuntime struct {
	runtime  wazero.Runtime
	mu       sync.Mutex
	compiled map[string]wazero.CompiledModule
}

func NewGrammarRuntime(ctx context.Context) *GrammarRuntime {
	var config wazero.RuntimeConfig
	if raceEnabled {
		// wazero's AOT compiler (wazevo) allocates heavily. Under the race
		// detector that makes compiling Tree-sitter grammars exceed CI's
		// package timeout; the interpreter is semantically equivalent.
		config = wazero.NewRuntimeConfigInterpreter()
	} else {
		config = wazero.NewRuntimeConfigCompiler()
	}
	config = config.WithMemoryLimitPages(8192).WithCloseOnContextDone(true)
	runtime := wazero.NewRuntimeWithConfig(ctx, config)
	_, _ = wasi_snapshot_preview1.Instantiate(ctx, runtime)
	return &GrammarRuntime{
		runtime:  runtime,
		compiled: make(map[string]wazero.CompiledModule, len(Languages)),
	}
}

// SyntaxNode is the provider-neutral portion of a Tree-sitter syntax tree.
// Byte ranges refer to the exact input passed to Parse. Children preserve the
// full Tree-sitter order, including anonymous punctuation, plus field names and
// namedness; Superopen extraction and structural fingerprints rely on all three.
type SyntaxNode struct {
	Type     string       `json:"type"`
	Field    string       `json:"field,omitempty"`
	Named    bool         `json:"named"`
	Start    uint32       `json:"start_byte"`
	End      uint32       `json:"end_byte"`
	HasError bool         `json:"has_error,omitempty"`
	Children []SyntaxNode `json:"children,omitempty"`
}

// Parse executes a combined Tree-sitter runtime+grammar WASI module. Grammar
// assets use a deliberately tiny ABI so no C pointers escape into Go.
func (r *GrammarRuntime) Parse(ctx context.Context, language string, source []byte) (SyntaxNode, error) {
	r.mu.Lock()
	compiled := r.compiled[language]
	runtime := r.runtime
	r.mu.Unlock()
	if compiled == nil {
		return SyntaxNode{}, fmt.Errorf("grammar %s is not compiled", language)
	}
	module, err := instantiateGrammarModule(ctx, runtime, compiled, language)
	if err != nil {
		return SyntaxNode{}, err
	}
	defer module.Close(ctx)
	return parseWASMModule(ctx, module, language, source)
}

func readSyntaxNode(ctx context.Context, module api.Module, handle uint32, count *int) (SyntaxNode, error) {
	*count++
	if *count > 1_000_000 {
		return SyntaxNode{}, fmt.Errorf("syntax tree exceeds one million named nodes")
	}
	typePointer, err := callOne(ctx, module, "so_node_type", uint64(handle))
	if err != nil {
		return SyntaxNode{}, err
	}
	typeName, ok := readCString(module.Memory(), uint32(typePointer), 1024)
	if !ok {
		return SyntaxNode{}, fmt.Errorf("invalid node type string")
	}
	start, err := callOne(ctx, module, "so_node_start_byte", uint64(handle))
	if err != nil {
		return SyntaxNode{}, err
	}
	end, err := callOne(ctx, module, "so_node_end_byte", uint64(handle))
	if err != nil {
		return SyntaxNode{}, err
	}
	errorFlag, err := callOne(ctx, module, "so_node_has_error", uint64(handle))
	if err != nil {
		return SyntaxNode{}, err
	}
	named, err := callOne(ctx, module, "so_node_is_named", uint64(handle))
	if err != nil {
		return SyntaxNode{}, err
	}
	childCount, err := callOne(ctx, module, "so_node_child_count", uint64(handle))
	if err != nil {
		return SyntaxNode{}, err
	}
	if childCount > 1_000_000 {
		return SyntaxNode{}, fmt.Errorf("node child count exceeds parser limit")
	}
	node := SyntaxNode{Type: typeName, Named: named != 0, Start: uint32(start), End: uint32(end), HasError: errorFlag != 0}
	for index := uint64(0); index < childCount; index++ {
		fieldPointer, err := callOne(ctx, module, "so_node_child_field_name", uint64(handle), index)
		if err != nil {
			return SyntaxNode{}, err
		}
		field := ""
		if fieldPointer != 0 {
			var ok bool
			field, ok = readCString(module.Memory(), uint32(fieldPointer), 1024)
			if !ok {
				return SyntaxNode{}, fmt.Errorf("invalid child field name string")
			}
		}
		child, err := callOne(ctx, module, "so_node_child", uint64(handle), index)
		if err != nil {
			return SyntaxNode{}, err
		}
		if child == 0 {
			return SyntaxNode{}, fmt.Errorf("grammar returned a null named child")
		}
		value, err := readSyntaxNode(ctx, module, uint32(child), count)
		if err != nil {
			return SyntaxNode{}, err
		}
		value.Field = field
		node.Children = append(node.Children, value)
	}
	return node, nil
}

func callOne(ctx context.Context, module api.Module, name string, params ...uint64) (uint64, error) {
	function := module.ExportedFunction(name)
	if function == nil {
		return 0, fmt.Errorf("parser ABI lacks %s", name)
	}
	result, err := function.Call(ctx, params...)
	if err != nil {
		return 0, fmt.Errorf("parser ABI %s: %w", name, err)
	}
	if len(result) != 1 {
		return 0, fmt.Errorf("parser ABI %s returned %d values", name, len(result))
	}
	return result[0], nil
}

func readCString(memory api.Memory, pointer, limit uint32) (string, bool) {
	for size := uint32(0); size < limit; size++ {
		value, ok := memory.ReadByte(pointer + size)
		if !ok {
			return "", false
		}
		if value == 0 {
			bytes, ok := memory.Read(pointer, size)
			return string(bytes), ok
		}
	}
	return "", false
}

func (r *GrammarRuntime) Compile(ctx context.Context, language string, wasm []byte) error {
	if r == nil || r.runtime == nil {
		return fmt.Errorf("grammar runtime is closed")
	}
	if !knownLanguage(language) {
		return fmt.Errorf("unknown pinned grammar %q", language)
	}
	if len(wasm) == 0 {
		return fmt.Errorf("grammar %s is empty", language)
	}
	compiled, err := r.runtime.CompileModule(ctx, wasm)
	if err != nil {
		return fmt.Errorf("compile grammar %s: %w", language, err)
	}
	expected := grammarExport(language)
	if _, ok := compiled.ExportedFunctions()[expected]; !ok {
		_ = compiled.Close(ctx)
		return fmt.Errorf("grammar %s does not export %s", language, expected)
	}
	r.mu.Lock()
	prior := r.compiled[language]
	r.compiled[language] = compiled
	r.mu.Unlock()
	if prior != nil {
		_ = prior.Close(ctx)
	}
	return nil
}

func (r *GrammarRuntime) Count() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.compiled)
}

func (r *GrammarRuntime) Complete() bool { return r.Count() == len(Languages) }

func (r *GrammarRuntime) Close(ctx context.Context) error {
	if r == nil || r.runtime == nil {
		return nil
	}
	r.mu.Lock()
	r.compiled = nil
	r.mu.Unlock()
	err := r.runtime.Close(ctx)
	r.runtime = nil
	return err
}

func knownLanguage(language string) bool {
	for _, candidate := range Languages {
		if candidate == language {
			return true
		}
	}
	return false
}

func grammarExport(language string) string {
	overrides := map[string]string{
		"assembly": "asm", "cobol": "COBOL", "elisp": "elisp", "gomod": "gomod",
		"gotemplate": "gotmpl", "janet": "janet_simple", "makefile": "make",
		"php": "php_only", "protobuf": "proto", "qml": "qmljs", "sshconfig": "ssh_config", "vim": "vim",
	}
	if name := overrides[language]; name != "" {
		language = name
	}
	return "tree_sitter_" + strings.TrimSpace(language)
}

// GrammarExport returns the generated Tree-sitter language symbol for a
// pinned grammar. It is exported for the development asset compiler.
func GrammarExport(language string) (string, bool) {
	if !knownLanguage(language) {
		return "", false
	}
	return grammarExport(language), true
}
