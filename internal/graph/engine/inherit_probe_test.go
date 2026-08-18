package engine

import (
	"context"
	"os"
	"testing"
)

func TestExtractSyntaxFactsCapturesTSHeritageNames(t *testing.T) {
	source := []byte(`
export interface GuardOptions { timeout?: number }
export class Guard {
  constructor() {}
  run() {}
}
export interface PIIOptions extends GuardOptions { redact?: boolean }
export class PII extends Guard {
  constructor() { super() }
}
`)
	ctx := context.Background()
	runtime, _, err := LoadGrammarAssets(ctx, EngineAssets, "assets/grammars/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(ctx)
	root := t.TempDir()
	path := "sample.ts"
	if err := os.WriteFile(root+"/"+path, source, 0o600); err != nil {
		t.Fatal(err)
	}
	repo, err := ParseSyntaxRepository(ctx, runtime, root, "fixture", []string{path}, nil, 1)
	if err != nil || len(repo.Files) != 1 {
		t.Fatalf("parse = %#v, %v", repo.Files, err)
	}
	got := repo.Files[0].Extraction
	bases := map[string][]string{}
	for _, def := range got.Definitions {
		if len(def.BaseClasses) > 0 {
			bases[def.Name] = def.BaseClasses
		}
	}
	if got := bases["PII"]; len(got) != 1 || got[0] != "Guard" {
		t.Fatalf("PII bases = %#v, want [Guard]", got)
	}
	if got := bases["PIIOptions"]; len(got) != 1 || got[0] != "GuardOptions" {
		t.Fatalf("PIIOptions bases = %#v, want [GuardOptions]", got)
	}
	for _, inh := range got.Inheritance {
		if inh.Name == "extends Guard" || inh.Name == "extends GuardOptions" {
			t.Fatalf("heritage captured keyword text: %#v", inh)
		}
		if inh.Name == "Guard" && inh.Scope != "PII" {
			t.Fatalf("inheritance scope = %q, want PII", inh.Scope)
		}
	}
}

func TestAssembleSyntaxGraphEmitsImplementsInheritsOverride(t *testing.T) {
	repository := SyntaxRepository{Files: []ParsedSyntaxFile{
		{
			File: FileRecord{Project: "fixture", Path: "src/base.ts", Language: "typescript"},
			Extraction: SyntaxExtraction{RootModule: true, Definitions: []SyntaxFact{
				{Kind: "class", NodeType: "interface_declaration", Name: "GuardOptions", StartLine: 1},
				{Kind: "class", NodeType: "class_declaration", Name: "Guard", StartLine: 2},
				{Kind: "function", NodeType: "method_definition", Name: "constructor", Scope: "Guard", StartLine: 3},
				{Kind: "function", NodeType: "method_definition", Name: "run", Scope: "Guard", StartLine: 4},
			}},
		},
		{
			File: FileRecord{Project: "fixture", Path: "src/pii.ts", Language: "typescript"},
			Extraction: SyntaxExtraction{
				RootModule: true,
				Definitions: []SyntaxFact{
					{Kind: "class", NodeType: "interface_declaration", Name: "PIIOptions", StartLine: 2, BaseClasses: []string{"GuardOptions"}},
					{Kind: "class", NodeType: "class_declaration", Name: "PII", StartLine: 3, BaseClasses: []string{"Guard"}},
					{Kind: "function", NodeType: "method_definition", Name: "constructor", Scope: "PII", StartLine: 4},
				},
				Imports: []SyntaxFact{
					{Kind: "import", Name: "./base", LocalName: "Guard", StartLine: 1},
					{Kind: "import", Name: "./base", LocalName: "GuardOptions", StartLine: 1},
				},
				Inheritance: []SyntaxFact{
					{Kind: "inheritance", Name: "GuardOptions", Scope: "PIIOptions", StartLine: 2},
					{Kind: "inheritance", Name: "Guard", Scope: "PII", StartLine: 3},
				},
			},
		},
	}}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	registry := newSymbolRegistry(graph.nodes)
	indexSyntaxInheritance("fixture", repository.Files, &graph, registry)
	indexSyntaxExplicitOverrides(&graph)

	if !hasPendingEdge(graph.edges, "src.pii.PII", "INHERITS", "src.base.Guard") {
		t.Fatalf("missing INHERITS: %#v", graph.edges)
	}
	if !hasPendingEdge(graph.edges, "src.pii.PIIOptions", "IMPLEMENTS", "src.base.GuardOptions") {
		t.Fatalf("missing IMPLEMENTS: %#v", graph.edges)
	}
	if !hasPendingEdge(graph.edges, "src.pii.PII.constructor", "OVERRIDE", "src.base.Guard.constructor") {
		t.Fatalf("missing OVERRIDE: %#v", graph.edges)
	}
}
