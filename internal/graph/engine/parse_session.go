package engine

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// ParseSession is a worker-local SyntaxParser that reuses instantiated WASM
// modules across files via a thread-local parser pool.
type ParseSession interface {
	SyntaxParser
	Close(context.Context) error
}

type parseSessionFactory interface {
	NewParseSession(context.Context) ParseSession
}

type wasmParseSession struct {
	runtime   *GrammarRuntime
	instances map[string]*wasmInstance
}

type wasmInstance struct {
	module api.Module
}

func (r *GrammarRuntime) NewParseSession(context.Context) ParseSession {
	return &wasmParseSession{runtime: r, instances: map[string]*wasmInstance{}}
}

func (r *GrammarRuntime) ExtractFacts(ctx context.Context, language, grammar string, source []byte) (FileResult, error) {
	session := r.NewParseSession(ctx)
	defer session.Close(ctx)
	return session.(factExtractor).ExtractFacts(ctx, language, grammar, source)
}

func (s *wasmParseSession) Parse(ctx context.Context, language string, source []byte) (SyntaxNode, error) {
	if s == nil || s.runtime == nil {
		return SyntaxNode{}, fmt.Errorf("grammar runtime is closed")
	}
	instance, err := s.instance(ctx, language)
	if err != nil {
		return SyntaxNode{}, err
	}
	tree, err := parseWASMModule(ctx, instance.module, language, source)
	if err != nil {
		s.drop(ctx, language)
	}
	return tree, err
}

func (s *wasmParseSession) ExtractFacts(ctx context.Context, language, grammar string, source []byte) (FileResult, error) {
	if s == nil || s.runtime == nil {
		return FileResult{}, fmt.Errorf("grammar runtime is closed")
	}
	instance, err := s.instance(ctx, grammar)
	if err != nil {
		return FileResult{}, err
	}
	handle, err := wasmParseHandle(ctx, instance.module, grammar, source)
	if err != nil {
		s.drop(ctx, grammar)
		return FileResult{}, err
	}
	return ExtractSyntaxFacts(language, &wasmView{ctx: ctx, module: instance.module, handle: handle}, source)
}

func (s *wasmParseSession) instance(ctx context.Context, language string) (*wasmInstance, error) {
	if current := s.instances[language]; current != nil {
		return current, nil
	}
	s.runtime.mu.Lock()
	compiled := s.runtime.compiled[language]
	s.runtime.mu.Unlock()
	if compiled == nil {
		return nil, fmt.Errorf("grammar %s is not compiled", language)
	}
	module, err := instantiateGrammarModule(ctx, s.runtime.runtime, compiled, language)
	if err != nil {
		return nil, err
	}
	instance := &wasmInstance{module: module}
	s.instances[language] = instance
	return instance, nil
}

func (s *wasmParseSession) drop(ctx context.Context, language string) {
	if s == nil {
		return
	}
	instance := s.instances[language]
	if instance == nil {
		return
	}
	_ = instance.module.Close(ctx)
	delete(s.instances, language)
}

func (s *wasmParseSession) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	var first error
	for language, instance := range s.instances {
		if err := instance.module.Close(ctx); err != nil && first == nil {
			first = err
		}
		delete(s.instances, language)
	}
	return first
}

func instantiateGrammarModule(ctx context.Context, runtime wazero.Runtime, compiled wazero.CompiledModule, language string) (api.Module, error) {
	module, err := runtime.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName(""))
	if err != nil {
		return nil, fmt.Errorf("instantiate grammar %s: %w", language, err)
	}
	if initialize := module.ExportedFunction("_initialize"); initialize != nil {
		if _, err := initialize.Call(ctx); err != nil {
			_ = module.Close(ctx)
			return nil, fmt.Errorf("initialize grammar %s: %w", language, err)
		}
	}
	return module, nil
}

func wasmParseHandle(ctx context.Context, module api.Module, language string, source []byte) (uint32, error) {
	if len(source) > 32<<20 {
		return 0, fmt.Errorf("source exceeds 32 MiB parser limit")
	}
	memory := module.Memory()
	allocate := module.ExportedFunction("so_alloc")
	parse := module.ExportedFunction("so_parse")
	if memory == nil || allocate == nil || parse == nil {
		return 0, fmt.Errorf("grammar %s lacks the so-graph parser ABI", language)
	}
	allocated, err := allocate.Call(ctx, uint64(len(source)))
	if err != nil || len(allocated) != 1 || allocated[0] == 0 {
		return 0, fmt.Errorf("allocate grammar input: %w", err)
	}
	pointer := uint32(allocated[0])
	if !memory.Write(pointer, source) {
		return 0, fmt.Errorf("write grammar input outside WASM memory")
	}
	if release := module.ExportedFunction("so_free"); release != nil {
		defer release.Call(ctx, uint64(pointer)) //nolint:errcheck
	}
	root, err := parse.Call(ctx, uint64(pointer), uint64(len(source)))
	if err != nil || len(root) != 1 || root[0] == 0 {
		return 0, fmt.Errorf("parse %s source: %w", language, err)
	}
	return uint32(root[0]), nil
}

func parseWASMModule(ctx context.Context, module api.Module, language string, source []byte) (SyntaxNode, error) {
	handle, err := wasmParseHandle(ctx, module, language, source)
	if err != nil {
		return SyntaxNode{}, err
	}
	count := 0
	return readSyntaxNode(ctx, module, handle, &count)
}

var _ parseSessionFactory = (*GrammarRuntime)(nil)
var _ ParseSession = (*wasmParseSession)(nil)
var _ factExtractor = (*wasmParseSession)(nil)
var _ factExtractor = (*GrammarRuntime)(nil)
