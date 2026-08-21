//go:build tsnative && cgo

package tsnative

import (
	"fmt"
	"sync"
	"unsafe"

	tree_sitter_asm "github.com/RubixDev/tree-sitter-asm/bindings/go"
	tree_sitter_devicetree "github.com/joelspadin/tree-sitter-devicetree/bindings/go"
	tree_sitter_kconfig "github.com/tree-sitter-grammars/tree-sitter-kconfig/bindings/go"
	tree_sitter_make "github.com/tree-sitter-grammars/tree-sitter-make/bindings/go"
	tree_sitter_yaml "github.com/tree-sitter-grammars/tree-sitter-yaml/bindings/go"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_bash "github.com/tree-sitter/tree-sitter-bash/bindings/go"
	tree_sitter_c "github.com/tree-sitter/tree-sitter-c/bindings/go"
	tree_sitter_cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tree_sitter_java "github.com/tree-sitter/tree-sitter-java/bindings/go"
	tree_sitter_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tree_sitter_json "github.com/tree-sitter/tree-sitter-json/bindings/go"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tree_sitter_rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"

	"github.com/ishanjainn/superopen/internal/graph/engine/tsnative/rstlang"
)

var languageFuncs = map[string]func() unsafe.Pointer{
	"assembly":   tree_sitter_asm.Language,
	"bash":       tree_sitter_bash.Language,
	"c":          tree_sitter_c.Language,
	"cpp":        tree_sitter_cpp.Language,
	"devicetree": tree_sitter_devicetree.Language,
	"go":         tree_sitter_go.Language,
	"java":       tree_sitter_java.Language,
	"javascript": tree_sitter_javascript.Language,
	"json":       tree_sitter_json.Language,
	"kconfig":    tree_sitter_kconfig.Language,
	"makefile":   tree_sitter_make.Language,
	"python":     tree_sitter_python.Language,
	"rst":        rstlang.Language,
	"rust":       tree_sitter_rust.Language,
	"tsx":        tree_sitter_typescript.LanguageTSX,
	"typescript": tree_sitter_typescript.LanguageTypescript,
	"yaml":       tree_sitter_yaml.Language,
}

var languageCache sync.Map

func Supported(grammar string) bool {
	_, ok := languageFuncs[grammar]
	return ok
}

func Language(grammar string) (*tree_sitter.Language, error) {
	return language(grammar)
}

func language(grammar string) (*tree_sitter.Language, error) {
	if cached, ok := languageCache.Load(grammar); ok {
		return cached.(*tree_sitter.Language), nil
	}
	loader, ok := languageFuncs[grammar]
	if !ok {
		return nil, fmt.Errorf("native grammar %s is not linked", grammar)
	}
	lang := tree_sitter.NewLanguage(loader())
	if lang == nil {
		return nil, fmt.Errorf("native grammar %s failed to load", grammar)
	}
	actual, _ := languageCache.LoadOrStore(grammar, lang)
	return actual.(*tree_sitter.Language), nil
}
