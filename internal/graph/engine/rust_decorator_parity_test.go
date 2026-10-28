package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestRustAttributesDecorateDefinitions(t *testing.T) {
	root := t.TempDir()
	code := `#[derive(Debug, Clone)]
struct Settings { name: String }

#[cfg(test)]
#[test]
fn checks_settings() {}
`
	if err := os.WriteFile(filepath.Join(root, "sample.rs"), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime, _, err := loadSelectedGrammarAssets(context.Background(), EngineAssets, "assets/grammars/manifest.json", false, []string{"rust"})
	if err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), runtime, root, "fixture", []string{"sample.rs"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	if !hasPendingEdge(graph.edges, "sample.Settings", "DECORATES", "<decorator:derive>") {
		t.Fatalf("missing derive decorator: %#v", repository.Files[0].Extraction.Decorators)
	}
	if !hasPendingEdge(graph.edges, "sample.checks_settings", "DECORATES", "<decorator:test>") {
		t.Fatalf("missing test decorator: %#v", repository.Files[0].Extraction.Decorators)
	}
}

func TestRustQualifiedTypeCallRejectsWrongOwner(t *testing.T) {
	registry := newSymbolRegistry([]api.Node{{Label: "Method", Name: "new", QualifiedName: "src.hyperlink.PathUrl.new"}})
	guess := symbolResolution{qn: "src.hyperlink.PathUrl.new", strategy: "suffix_match"}
	if !rustQualifiedTypeMismatch("rust", "Path::new", guess, registry) {
		t.Fatal("Path::new incorrectly resolved to PathUrl.new")
	}
	if rustQualifiedTypeMismatch("rust", "PathUrl::new", guess, registry) {
		t.Fatal("matching receiver rejected")
	}
}

func TestRustQualifiedConstructorResolvesNamedType(t *testing.T) {
	root := t.TempDir()
	code := "struct Alpha;\nstruct Beta;\nimpl Alpha { fn new() -> Self { Alpha } }\nimpl Beta { fn new() -> Self { Beta } }\nfn run() { Alpha::new(); Beta::new(); }\n"
	if err := os.WriteFile(filepath.Join(root, "sample.rs"), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime, _, err := loadSelectedGrammarAssets(context.Background(), EngineAssets, "assets/grammars/manifest.json", false, []string{"rust"})
	if err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), runtime, root, "fixture", []string{"sample.rs"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	for _, target := range []string{"sample.Alpha.new", "sample.Beta.new"} {
		if !hasPendingEdge(graph.edges, "sample.run", "CALLS", target) {
			t.Fatalf("missing qualified constructor %s: %#v", target, graph.edges)
		}
	}
}

func TestRustNestedModuleCallsOuterSymbols(t *testing.T) {
	root := t.TempDir()
	code := "struct CommandSet;\nimpl CommandSet { fn new() -> Self { Self } }\nfn helper() {}\n#[cfg(test)] mod tests { use super::*; fn check() { assert_eq!(CommandSet::new(), CommandSet::new()); helper(); } }\n"
	if err := os.WriteFile(filepath.Join(root, "sample.rs"), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime, _, err := loadSelectedGrammarAssets(context.Background(), EngineAssets, "assets/grammars/manifest.json", false, []string{"rust"})
	if err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), runtime, root, "fixture", []string{"sample.rs"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	for _, target := range []string{"sample.CommandSet.new", "sample.helper"} {
		if !hasPendingEdge(graph.edges, "sample.tests.check", "CALLS", target) {
			t.Fatalf("missing nested call to %s; facts=%#v", target, repository.Files[0].Extraction.Calls)
		}
	}
}

func TestRustEnumAndTraitOwnMethods(t *testing.T) {
	root := t.TempDir()
	code := "enum Size { Small }\nimpl Size { fn fits(&self) -> bool { true } }\ntrait Check { fn check(&self) -> bool; }\n"
	if err := os.WriteFile(filepath.Join(root, "sample.rs"), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime, _, err := loadSelectedGrammarAssets(context.Background(), EngineAssets, "assets/grammars/manifest.json", false, []string{"rust"})
	if err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), runtime, root, "fixture", []string{"sample.rs"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	for _, pair := range [][2]string{{"sample.Size", "sample.Size.fits"}, {"sample.Check", "sample.Check.check"}} {
		if !hasPendingEdge(graph.edges, pair[0], "DEFINES_METHOD", pair[1]) {
			t.Fatalf("missing owner link %v", pair)
		}
	}
}

func TestRustNestedTurbofishResolves(t *testing.T) {
	registry := newSymbolRegistry([]api.Node{{Label: "Method", Name: "new", QualifiedName: "sample.CommandTemplate.new"}})
	call := "CommandTemplate::new::<Vec<_>, &'static str>"
	if got := rustResolveQualifiedCall(registry, "sample", call, nil); got.qn != "sample.CommandTemplate.new" {
		t.Fatalf("qualified generic constructor: %#v", got)
	}
}
