package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestJSIndexTopLevelCallsBelongToModule(t *testing.T) {
	root := t.TempDir()
	path := "pkg/index.js"
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, path), []byte("function handle() {}\nhandle();\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", []string{path}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	if !hasPendingEdge(graph.edges, "pkg.index", "CALLS", "pkg.handle") {
		t.Fatalf("top-level call lost its module source: %#v", graph.edges)
	}
}

func TestJSRelativeRootImportTargetsProject(t *testing.T) {
	index := newImportTargetIndex([]api.Node{{Project: "fixture", Label: "Project", QualifiedName: "fixture"}})
	if got := jsSyntaxImportTarget("examples/auth/index.js", "../..", index); got != "fixture" {
		t.Fatalf("relative package-root import = %q", got)
	}
}
