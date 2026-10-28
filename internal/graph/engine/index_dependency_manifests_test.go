package engine

import "testing"

func TestRequirementsManifestProducesPackages(t *testing.T) {
	files := []ParsedSyntaxFile{{File: FileRecord{Path: "doc/requirements.txt"}, Body: []byte("Sphinx>=7\nfuro==1.2\n-r more.txt\n# comment\n")}}
	graph := goGraph{}
	indexDependencyManifests(files, "test", &graph)
	if len(graph.nodes) != 2 || len(graph.edges) != 2 || graph.nodes[0].Name != "Sphinx" || graph.nodes[1].Name != "furo" {
		t.Fatalf("requirements packages: %+v %+v", graph.nodes, graph.edges)
	}
}
