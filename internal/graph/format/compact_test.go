package format_test

import (
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/ishanjainn/superopen/internal/graph/format"
)

func TestSearchCompactOmitsProperties(t *testing.T) {
	text := format.SearchCompact(api.SearchResult{
		Matches: []api.RankedNode{{
			Node: api.Node{
				Label: "Method", Name: "bar", QualifiedName: "pkg.Foo.bar",
				Location:   api.Location{File: "foo.go", StartLine: 1, EndLine: 10},
				Properties: api.Properties{"bt": "secret", "fp": "abc"},
			},
			Score: 12.5,
		}},
		Page: api.Page{Total: 1},
	})
	if strings.Contains(text, "secret") || strings.Contains(text, "fp") {
		t.Fatalf("leaked properties: %q", text)
	}
	if !strings.Contains(text, "pkg.Foo.bar") {
		t.Fatalf("missing qn: %q", text)
	}
}

func TestTraceCompactAmbiguous(t *testing.T) {
	text := format.TraceCompact(api.TraceResult{
		Status:  "ambiguous",
		Message: "retry with qualified_name",
		Suggestions: []api.Node{
			{QualifiedName: "pkg.A.init", Label: "Method", Location: api.Location{File: "a.go", StartLine: 1}},
			{QualifiedName: "pkg.B.init", Label: "Method", Location: api.Location{File: "b.go", StartLine: 2}},
		},
	})
	if !strings.Contains(text, "status: ambiguous") || !strings.Contains(text, "pkg.A.init") {
		t.Fatalf("%q", text)
	}
}

func TestHelpForQuery(t *testing.T) {
	hints := format.HelpForQuery(api.QueryResult{
		Seeds: []api.RankedNode{{
			Node: api.Node{Name: "bar", QualifiedName: "pkg.Foo.bar"},
		}},
	})
	if len(hints) != 1 {
		t.Fatalf("len=%d hints=%v", len(hints), hints)
	}
	if hints[0] != "so graph snippet pkg.Foo.bar" {
		t.Fatalf("snippet hint: %q", hints[0])
	}
}

func TestTraceCompactIncomingCallers(t *testing.T) {
	caller := api.Node{Name: "useLeaf", QualifiedName: "b.useLeaf", Location: api.Location{File: "b.ts"}}
	leaf := api.Node{Name: "leaf", QualifiedName: "a.leaf", Location: api.Location{File: "a.ts"}}
	text := format.TraceCompact(api.TraceResult{
		Direction: "incoming",
		Paths: [][]api.TraceStep{{
			{Node: leaf, Hop: 0},
			{Node: caller, Via: &api.Edge{Type: "CALLS"}, Hop: 1},
		}},
	})
	if !strings.Contains(text, "direction: incoming") || !strings.Contains(text, "callers:") {
		t.Fatalf("%q", text)
	}
	if strings.Contains(text, "callees:") {
		t.Fatalf("incoming labeled callees: %q", text)
	}
	if !strings.Contains(text, "useLeaf") {
		t.Fatalf("missing caller: %q", text)
	}
}

func TestHelpForSnippetDirections(t *testing.T) {
	hints := format.HelpForSnippet(api.SnippetResult{QualifiedName: "a.leaf"})
	joined := strings.Join(hints, "\n")
	if strings.Contains(joined, "both") {
		t.Fatalf("must not suggest both: %v", hints)
	}
	if len(hints) != 2 || !strings.Contains(hints[0], "incoming") || !strings.Contains(hints[1], "outgoing") {
		t.Fatalf("hints=%v", hints)
	}
}
