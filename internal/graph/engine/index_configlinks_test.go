package engine

import (
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestIndexConfigLinksMatchesNormalizedMultiTokenKeys(t *testing.T) {
	graph := goGraph{nodes: []api.Node{
		{Label: "Variable", Name: "asset_revision", QualifiedName: "config.asset_revision", Location: api.Location{File: "manifest.json"}},
		{Label: "Function", Name: "assetRevision", QualifiedName: "tool.assetRevision", Location: api.Location{File: "main.go"}},
		{Label: "Function", Name: "commit", QualifiedName: "tool.commit", Location: api.Location{File: "main.go"}},
	}}
	indexConfigLinks(&graph)
	for _, edge := range graph.edges {
		if edge.source == "tool.assetRevision" && edge.kind == "CONFIGURES" && edge.target == "config.asset_revision" {
			return
		}
	}
	t.Fatalf("missing config key link: %#v", graph.edges)
}

func TestNormalizeConfigKeyUsesPinnedCamelBoundary(t *testing.T) {
	got, tokens := normalizeConfigKey("treeSitter.asset_revision")
	if got != "tree_sitter_asset_revision" || len(tokens) != 4 {
		t.Fatalf("normalized=%q tokens=%#v", got, tokens)
	}
}
