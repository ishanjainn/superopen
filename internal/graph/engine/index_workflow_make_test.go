package engine

import (
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestWorkflowMakeStepLinksExistingTarget(t *testing.T) {
	files := []ParsedSyntaxFile{{File: FileRecord{Path: ".github/workflows/checks.yaml", Language: "yaml"}, Body: []byte("jobs:\n  documentation:\n    steps:\n      - name: build\n        run: |\n          cd doc\n          make html\n")}}
	graph := goGraph{nodes: []api.Node{{Label: "Function", Name: "html", QualifiedName: "doc.Makefile.html", Location: api.Location{File: "doc/Makefile", StartLine: 41}}}}
	indexWorkflowMakeSpawns(files, &graph)
	if len(graph.edges) != 1 || graph.edges[0].target != "doc.Makefile.html" || graph.edges[0].properties["job"] != "documentation" {
		t.Fatalf("workflow target: %+v", graph.edges)
	}
}
