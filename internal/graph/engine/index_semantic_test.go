package engine

import (
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestExplicitOverridesUseNearestDeclaringAncestors(t *testing.T) {
	graph := goGraph{}
	for _, name := range []string{"Root", "Gap", "Tail", "Left", "Right", "Leaf"} {
		graph.nodes = append(graph.nodes, api.Node{Label: "Class", Name: name, QualifiedName: "pkg." + name,
			Location: api.Location{File: "pkg.py"}})
	}
	for _, name := range []string{"Root", "Tail", "Left", "Right", "Leaf"} {
		method := "pkg." + name + ".clean"
		graph.nodes = append(graph.nodes, api.Node{Label: "Method", Name: "clean", QualifiedName: method,
			Location: api.Location{File: "pkg.py"}})
		graph.edges = append(graph.edges, pendingEdge{source: "pkg." + name, target: method, kind: "DEFINES_METHOD"})
	}
	for _, pair := range [][2]string{{"Gap", "Root"}, {"Tail", "Gap"}, {"Left", "Root"}, {"Right", "Root"}, {"Leaf", "Left"}, {"Leaf", "Right"}} {
		graph.edges = append(graph.edges, pendingEdge{source: "pkg." + pair[0], target: "pkg." + pair[1], kind: "INHERITS"})
	}
	indexSyntaxExplicitOverrides(&graph)
	for _, base := range []string{"Root"} {
		if !hasPendingEdge(graph.edges, "pkg.Tail.clean", "OVERRIDE", "pkg."+base+".clean") {
			t.Fatalf("transitive nearest override missing: %#v", graph.edges)
		}
	}
	for _, base := range []string{"Left", "Right"} {
		if !hasPendingEdge(graph.edges, "pkg.Leaf.clean", "OVERRIDE", "pkg."+base+".clean") {
			t.Fatalf("equally near override missing: %#v", graph.edges)
		}
	}
	if hasPendingEdge(graph.edges, "pkg.Leaf.clean", "OVERRIDE", "pkg.Root.clean") {
		t.Fatalf("override bypassed a nearer declaration: %#v", graph.edges)
	}
}
