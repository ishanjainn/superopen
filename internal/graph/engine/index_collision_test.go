package engine

import (
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestSortGraphKeepsDefinitionOverSameNameBinding(t *testing.T) {
	graph := goGraph{nodes: []api.Node{
		{Label: "Variable", Name: "Task", QualifiedName: "pkg.Task", Location: api.Location{File: "pkg.py", StartLine: 2}},
		{Label: "Class", Name: "Task", QualifiedName: "pkg.Task", Location: api.Location{File: "pkg.py", StartLine: 8}},
	}}
	sortGraph(&graph)
	if len(graph.nodes) != 1 || graph.nodes[0].Label != "Class" {
		t.Fatalf("same-name definition: %+v", graph.nodes)
	}
}
