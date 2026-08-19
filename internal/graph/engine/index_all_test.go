package engine

import (
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestReplaceGenericGoGraphKeepsOtherLanguagesAndResolvedGo(t *testing.T) {
	generic := goGraph{
		files: []FileRecord{{Path: "main.go", Language: "go"}, {Path: "app.py", Language: "python"}},
		nodes: []api.Node{
			{QualifiedName: "project:p", Label: "Project"},
			{QualifiedName: "file:main.go", Label: "File"},
			{QualifiedName: "go:main.go#weak", Label: "Function"},
			{QualifiedName: "file:app.py", Label: "File"},
			{QualifiedName: "python:app.py#strong", Label: "Function"},
		},
		edges: []pendingEdge{
			{source: "file:main.go", target: "go:main.go#weak", kind: "DEFINES"},
			{source: "file:app.py", target: "python:app.py#strong", kind: "DEFINES"},
		},
	}
	resolved := goGraph{
		files: []FileRecord{{Path: "main.go", Language: "go"}},
		nodes: []api.Node{
			{QualifiedName: "project:p", Label: "Project"},
			{QualifiedName: "file:main.go", Label: "File", Properties: api.Properties{"resolver": "go_types"}},
			{QualifiedName: "p.Strong", Label: "Function"},
		},
		edges: []pendingEdge{{source: "file:main.go", target: "p.Strong", kind: "DEFINES"}},
	}
	got := replaceGenericGoGraph(generic, resolved, map[string]bool{"main.go": true})
	if len(got.files) != 2 || !hasNode(got.nodes, "python:app.py#strong") || !hasNode(got.nodes, "p.Strong") || hasNode(got.nodes, "go:main.go#weak") {
		t.Fatalf("hybrid graph = %#v", got)
	}
}
