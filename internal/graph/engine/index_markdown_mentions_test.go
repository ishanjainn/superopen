package engine

import (
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestMarkdownCodePathsLinkToRepositoryFilesAndFolders(t *testing.T) {
	graph := goGraph{nodes: []api.Node{
		{Label: "File", QualifiedName: fileQualifiedName("doc/release.md"), Location: api.Location{File: "doc/release.md"}},
		{Label: "File", QualifiedName: fileQualifiedName("doc/index.rst"), Location: api.Location{File: "doc/index.rst"}},
		{Label: "Folder", QualifiedName: "doc.whatsnew", Location: api.Location{File: "doc/whatsnew"}},
		{Label: "Section", QualifiedName: "doc.release.Releasing", Location: api.Location{File: "doc/release.md", StartLine: 5, EndLine: 6}},
	}}
	files := []ParsedSyntaxFile{{File: FileRecord{Path: "doc/release.md", Language: "markdown"}, Body: []byte("# Release\n\n\n\n## Releasing\nUse `doc/whatsnew`.\nAlso `doc/index.rst`.\n")}}
	indexMarkdownMentions(files, &graph)
	if len(graph.edges) != 2 {
		t.Fatalf("markdown links: %+v", graph.edges)
	}
	for _, edge := range graph.edges {
		if edge.source != "doc.release.Releasing" {
			t.Fatalf("markdown heading source: %+v", edge)
		}
	}
}
