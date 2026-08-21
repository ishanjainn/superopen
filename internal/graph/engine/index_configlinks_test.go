package engine

import (
	"strings"
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

func TestIndexConfigLinksCapsCanonicalCodeSet(t *testing.T) {
	graph := goGraph{nodes: []api.Node{
		{Label: "Variable", Name: "asset_revision", QualifiedName: "z.config.asset_revision", Location: api.Location{File: "z.json"}},
		{Label: "Variable", Name: "asset_revision", QualifiedName: "a.config.asset_revision", Location: api.Location{File: "a.json"}},
	}}
	for i := 0; i < configLinkCodeCap+10; i++ {
		name := "fn"
		qn := "zzz.fn" + strings.Repeat("x", i%3)
		if i == 0 {
			qn = "aaa.assetRevision"
			name = "assetRevision"
		}
		graph.nodes = append(graph.nodes, api.Node{Label: "Function", Name: name, QualifiedName: qn, Location: api.Location{File: "main.go"}})
	}
	configs := collectConfigEntries(graph.nodes, configLinkConfigCap)
	if len(configs) != 2 || configs[0].qn != "a.config.asset_revision" {
		t.Fatalf("configs=%#v", configs)
	}
	code := collectCodeEntries(graph.nodes, 2)
	if len(code) != 2 || code[0].qn != "aaa.assetRevision" {
		t.Fatalf("code=%#v", code)
	}
}

func TestIndexConfigDepImportsLinksManifestToImport(t *testing.T) {
	graph := goGraph{
		nodes: []api.Node{
			{Label: "Variable", Name: "lodash", QualifiedName: "pkg.dependencies.lodash", Location: api.Location{File: "package.json"}},
			{Label: "Module", Name: "lodash", QualifiedName: "node.lodash", Location: api.Location{File: "web/app.ts"}},
			{Label: "File", Name: "app.ts", QualifiedName: "web.app.ts.__file__", Location: api.Location{File: "web/app.ts"}},
		},
		edges: []pendingEdge{{source: "web.app.ts.__file__", target: "node.lodash", kind: "IMPORTS"}},
	}
	indexConfigLinks(&graph)
	for _, edge := range graph.edges {
		if edge.kind == "CONFIGURES" && edge.source == "web.app.ts.__file__" && edge.target == "pkg.dependencies.lodash" {
			return
		}
	}
	t.Fatalf("missing dep import CONFIGURES: %#v", graph.edges)
}
