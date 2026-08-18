package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPythonLabelGoldenHistogram(t *testing.T) {
	ctx := context.Background()
	runtime, _, err := LoadGrammarAssets(ctx, EngineAssets, "assets/grammars/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(ctx)
	root := t.TempDir()
	path := "sample.py"
	body := "class Client:\n    def run(self):\n        return 1\n\ndef helper():\n    return helper()\n"
	abs := filepath.Join(root, path)
	if err := os.WriteFile(abs, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(ctx, runtime, root, "fixture", []string{path}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	counts := map[string]int{}
	for _, node := range graph.nodes {
		switch node.Label {
		case "File", "Folder", "Project", "Module":
			continue
		}
		if node.Location.File == "<python-builtins>" {
			continue
		}
		counts[node.Label]++
	}
	if counts["Class"] != 1 || counts["Function"] != 1 || counts["Method"] != 1 {
		t.Fatalf("label histogram = %#v", counts)
	}
}
