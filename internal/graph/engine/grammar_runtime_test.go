package engine

import (
	"context"
	"os"
	"testing"
)

func TestGrammarRuntimeValidatesPinnedExport(t *testing.T) {
	ctx := context.Background()
	runtime := NewGrammarRuntime(ctx)
	defer runtime.Close(ctx)
	if err := runtime.Compile(ctx, "go", minimalGoGrammarWASM); err != nil {
		t.Fatal(err)
	}
	if runtime.Count() != 1 || runtime.Complete() {
		t.Fatalf("unexpected runtime state count=%d complete=%t", runtime.Count(), runtime.Complete())
	}
	if err := runtime.Compile(ctx, "not-a-language", minimalGoGrammarWASM); err == nil {
		t.Fatal("expected unknown-language failure")
	}
}

func TestGrammarRuntimeParsesCombinedModule(t *testing.T) {
	path := os.Getenv("SUPEROPEN_GO_GRAMMAR_WASM")
	if path == "" {
		t.Skip("set SUPEROPEN_GO_GRAMMAR_WASM to exercise a generated combined parser module")
	}
	wasm, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	runtime := NewGrammarRuntime(ctx)
	defer runtime.Close(ctx)
	if err := runtime.Compile(ctx, "go", wasm); err != nil {
		t.Fatal(err)
	}
	tree, err := runtime.Parse(ctx, "go", []byte("package sample\nfunc Run() { helper(1) }\n"))
	if err != nil {
		t.Fatal(err)
	}
	if tree.Type != "source_file" || tree.HasError || tree.Start != 0 || tree.End == 0 {
		t.Fatalf("unexpected syntax root: %+v", tree)
	}
	foundFunction := false
	foundNameField := false
	foundAnonymous := false
	var walk func(SyntaxNode)
	walk = func(node SyntaxNode) {
		foundFunction = foundFunction || node.Type == "function_declaration"
		foundNameField = foundNameField || node.Field == "name" && node.Type == "identifier" && node.Named
		foundAnonymous = foundAnonymous || !node.Named
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(tree)
	if !foundFunction {
		t.Fatalf("function declaration missing from syntax tree: %+v", tree)
	}
	if !foundNameField || !foundAnonymous {
		t.Fatalf("field names or anonymous syntax nodes missing: %+v", tree)
	}
	facts, err := ExtractSyntaxFacts("go", tree, []byte("package sample\nfunc Run() { helper(1) }\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.Definitions) != 1 || facts.Definitions[0].Name != "Run" || len(facts.Calls) != 1 || facts.Calls[0].Name != "helper" {
		t.Fatalf("real grammar extraction = %#v", facts)
	}
}

// (module (func (export "tree_sitter_go") (result i32) (i32.const 1)))
var minimalGoGrammarWASM = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
	0x01, 0x05, 0x01, 0x60, 0x00, 0x01, 0x7f,
	0x03, 0x02, 0x01, 0x00,
	0x07, 0x12, 0x01, 0x0e, 't', 'r', 'e', 'e', '_', 's', 'i', 't', 't', 'e', 'r', '_', 'g', 'o', 0x00, 0x00,
	0x0a, 0x06, 0x01, 0x04, 0x00, 0x41, 0x01, 0x0b,
}
