package engine

import (
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestPythonBuiltinsArePinnedAndGroundedInPythonFiles(t *testing.T) {
	files := []ParsedSyntaxFile{{
		File:       FileRecord{Project: "fixture", Path: "main.py", Language: "python"},
		Extraction: SyntaxExtraction{Calls: []SyntaxFact{{Kind: "call", Name: "len", StartLine: 1}}},
	}}
	graph := goGraph{nodes: []api.Node{{Project: "fixture", Label: "File", Name: "main.py", QualifiedName: fileQualifiedName("main.py")}}}
	indexPythonBuiltins("fixture", files, &graph)
	foundEdge := false
	for _, edge := range graph.edges {
		foundEdge = foundEdge || (edge.source == fileQualifiedName("main.py") && edge.target == "builtins.len" && edge.kind == "CALLS")
	}
	if !hasNode(graph.nodes, "builtins.len") || !foundEdge {
		t.Fatalf("builtin graph missing: nodes=%#v edges=%#v", graph.nodes, graph.edges)
	}
}
