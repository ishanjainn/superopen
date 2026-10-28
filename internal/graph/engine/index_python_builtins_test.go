package engine

import (
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestPythonBuiltinsArePinnedWithoutGuessingCallTargets(t *testing.T) {
	files := []ParsedSyntaxFile{{
		File:       FileRecord{Project: "fixture", Path: "main.py", Language: "python"},
		Extraction: FileResult{Calls: []SyntaxFact{{Kind: "call", Name: "len", StartLine: 1}}},
	}}
	graph := goGraph{nodes: []api.Node{{Project: "fixture", Label: "File", Name: "main.py", QualifiedName: fileQualifiedName("main.py")}}}
	indexPythonBuiltins("fixture", files, &graph)
	foundDefinition := false
	for _, edge := range graph.edges {
		foundDefinition = foundDefinition || (edge.source == fileQualifiedName("main.py") && edge.target == "builtins.len" && edge.kind == "DEFINES")
		if edge.kind == "CALLS" {
			t.Fatalf("builtin call must be resolved by the call pass: %#v", edge)
		}
	}
	if !hasNode(graph.nodes, "builtins.len") || !foundDefinition {
		t.Fatalf("builtin graph missing: nodes=%#v edges=%#v", graph.nodes, graph.edges)
	}
}
