package steer

import (
	"strings"
	"testing"
)

func TestExploreAugmentRendersHits(t *testing.T) {
	text := ExploreAugment("HandleRequest", 12, []GraphHit{
		{QualifiedName: "internal/api.HandleRequest", Label: "Function", File: "internal/api/handler.go", Lines: "42-88"},
		{QualifiedName: "internal/api.handleRequestV2", Label: "Function", File: "internal/api/v2.go", Lines: "10-31"},
	})
	if !strings.Contains(text, "12 indexed match") {
		t.Fatalf("missing total: %s", text)
	}
	for _, want := range []string{"internal/api.HandleRequest", "Function", "internal/api/handler.go", "42-88"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	if !strings.Contains(text, "graph_snippet") {
		t.Fatalf("augment should tell the agent what to call next: %s", text)
	}
}

// The augment is injected into the agent's context, so an empty result must
// cost nothing at all rather than rendering a header with no rows.
func TestExploreAugmentEmptyWithoutHits(t *testing.T) {
	if got := ExploreAugment("HandleRequest", 0, nil); got != "" {
		t.Fatalf("expected empty augment, got %q", got)
	}
	if got := ExploreAugment("", 3, []GraphHit{{QualifiedName: "x"}}); got != "" {
		t.Fatalf("expected empty augment for a blank term, got %q", got)
	}
}

func TestExploreAugmentStaysSmall(t *testing.T) {
	hits := make([]GraphHit, 0, 5)
	for range 5 {
		hits = append(hits, GraphHit{
			QualifiedName: "internal/some/deep/package.SomeReasonablyLongSymbolName",
			Label:         "Function",
			File:          "internal/some/deep/package/file.go",
			Lines:         "100-200",
		})
	}
	text := ExploreAugment("SomeReasonablyLongSymbolName", 250, hits)
	// This lands in context on every augmented tool call; a bloated view
	// would cost more than the grep it replaces.
	if len(text) > 800 {
		t.Fatalf("augment too large (%d bytes):\n%s", len(text), text)
	}
}
