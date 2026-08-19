package engine

import (
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestQueryEdgePreferred(t *testing.T) {
	if !queryEdgePreferred("CALLS") || !queryEdgePreferred("CONFIGURES") {
		t.Fatal("expected CALLS/CONFIGURES preferred")
	}
	if queryEdgePreferred("IMPORTS") {
		t.Fatal("IMPORTS should not be preferred")
	}
}

func TestQuerySnippetSeedsPrefersCallables(t *testing.T) {
	seeds := querySnippetSeeds([]api.RankedNode{
		{Node: api.Node{Label: "Class", QualifiedName: "A"}},
		{Node: api.Node{Label: "Method", QualifiedName: "A.m"}},
		{Node: api.Node{Label: "Function", QualifiedName: "f"}},
		{Node: api.Node{Label: "File", QualifiedName: "x"}},
	}, 3)
	if len(seeds) != 3 {
		t.Fatalf("got %d seeds", len(seeds))
	}
	if seeds[0].QualifiedName != "A.m" || seeds[1].QualifiedName != "f" {
		t.Fatalf("unexpected order: %#v", seeds)
	}
}
