package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMarkdownSetextHeadingCreatesSection(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("Release 5.0\n===========\n\n## Changes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime, _, err := loadSelectedGrammarAssets(context.Background(), EngineAssets, "assets/grammars/manifest.json", false, []string{"markdown"})
	if err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), runtime, root, "fixture", []string{"README.md"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	if !hasNode(graph.nodes, "README.Release-5.0") {
		t.Fatalf("missing setext section: %#v", repository.Files[0].Extraction.Sections)
	}
}
