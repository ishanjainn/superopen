package engine

import "testing"

func TestRSTGraphvizDirectiveCreatesDiagram(t *testing.T) {
	files := []ParsedSyntaxFile{{File: FileRecord{Path: "doc/flow.rst", Language: "rst"}, Body: []byte(".. digraph:: flow\n   :caption: Flow\n\n   start -> end\n")}}
	graph := goGraph{}
	indexRSTDiagrams(files, &graph)
	labels := map[string]int{}
	for _, n := range graph.nodes {
		labels[n.Label]++
	}
	for _, e := range graph.edges {
		labels[e.kind]++
	}
	if labels["Diagram"] != 1 || labels["DiagramElement"] != 2 || labels["DIAGRAM_ARROW"] != 1 {
		t.Fatalf("graphviz: %+v", labels)
	}
}
