package engine

import "testing"

func TestDiagramFilesExposeNodesAndArrows(t *testing.T) {
	files := []ParsedSyntaxFile{{File: FileRecord{Path: "docs/flow.dot", Language: "diagram"}, Body: []byte("digraph flow {\n\"a\" [label=\"Start\"];\n\"b\" [label=\"Done\"];\n\"a\" -> \"b\";\n}\n")}}
	graph := goGraph{}
	indexDiagramFiles(files, &graph)
	counts := map[string]int{}
	for _, n := range graph.nodes {
		counts[n.Label]++
	}
	for _, e := range graph.edges {
		counts[e.kind]++
	}
	if counts["Diagram"] != 1 || counts["DiagramElement"] != 2 || counts["DIAGRAM_ARROW"] != 1 {
		t.Fatalf("diagram counts: %+v", counts)
	}
}

func TestDOTUnquotedArrowDefinesBothElements(t *testing.T) {
	files := []ParsedSyntaxFile{{File: FileRecord{Path: "docs/flow.dot", Language: "diagram"}, Body: []byte("digraph {\n  BAR -> BAZ\n}\n")}}
	graph := goGraph{}
	indexDiagramFiles(files, &graph)
	counts := map[string]int{}
	for _, node := range graph.nodes {
		counts[node.Label]++
	}
	for _, edge := range graph.edges {
		counts[edge.kind]++
	}
	if counts["DiagramElement"] != 2 || counts["DIAGRAM_ARROW"] != 1 {
		t.Fatalf("unquoted DOT arrow: %+v", counts)
	}
}

func TestDiagramCompositionAndInheritanceKeepDirection(t *testing.T) {
	files := []ParsedSyntaxFile{
		{File: FileRecord{Path: "doc/flow.mmd", Language: "mermaid"}, Body: []byte("classDiagram\nclass Ancestor\nclass Child\nclass Member\nChild --|> Ancestor\nMember --* Ancestor : field\n")},
		{File: FileRecord{Path: "doc/flow.dot", Language: "diagram"}, Body: []byte("digraph {\n\"Ancestor\" [label=\"Ancestor\"];\n\"Member\" [label=\"Member\"];\n\"Member\" -> \"Ancestor\" [arrowhead=\"diamond\", label=\"field\"];\n}\n")},
	}
	graph := goGraph{}
	indexDiagramFiles(files, &graph)
	want := map[string]bool{
		"doc.flow#diagram-1.Child>doc.flow#diagram-1.Ancestor:inheritance":  false,
		"doc.flow#diagram-1.Ancestor>doc.flow#diagram-1.Member:composition": false,
	}
	for _, edge := range graph.edges {
		if edge.kind != "DIAGRAM_ARROW" {
			continue
		}
		key := edge.source + ">" + edge.target + ":" + edge.properties["style"].(string)
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for key, found := range want {
		if !found {
			t.Errorf("missing %s: %+v", key, graph.edges)
		}
	}
}
