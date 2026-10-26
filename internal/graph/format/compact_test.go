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
		Page: api.Page{Total: 4},
	}, "bar")
	if strings.Contains(text, "secret") || strings.Contains(text, "fp") {
		t.Fatalf("leaked properties: %q", text)
	}
	if !strings.Contains(text, "count: 1 of 4") || !strings.Contains(text, "results[1]{qn,kind,file,lines}:") ||
		!strings.Contains(text, "  pkg.Foo.bar,Method,foo.go,1-10") {
		t.Fatalf("search rows: %q", text)
	}
}

func TestSearchCompactEmptyIsDefinitive(t *testing.T) {
	text := format.SearchCompact(api.SearchResult{}, "make_msgid")
	if !strings.Contains(text, "count: 0") || !strings.Contains(text, `"make_msgid"`) {
		t.Fatalf("empty search must state the zero and the query: %q", text)
	}
	hints := format.HelpForSearch(api.SearchResult{}, "make_msgid", 10)
	if len(hints) != 1 || !strings.Contains(hints[0], `so graph code-search "make_msgid"`) {
		t.Fatalf("empty search should point at code-search: %v", hints)
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
	if !strings.Contains(text, "status: ambiguous") || !strings.Contains(text, "candidates[2]{qn,kind,file,lines}:") || !strings.Contains(text, "pkg.A.init") {
		t.Fatalf("%q", text)
	}
}

func queryFixture() api.QueryResult {
	msg := api.Node{ID: 1, Label: "Method", Name: "message", QualifiedName: "django.core.mail.message.EmailMessage.message",
		Location: api.Location{File: "django/core/mail/message.py", StartLine: 239, EndLine: 264}}
	fqdn := api.Node{ID: 2, Label: "Method", Name: "get_fqdn", QualifiedName: "django.core.mail.utils.CachedDnsName.get_fqdn",
		Location: api.Location{File: "django/core/mail/utils.py", StartLine: 14, EndLine: 17}}
	return api.QueryResult{
		Rows:      []api.Node{msg, fqdn},
		RowsTotal: 40,
		Edges: []api.Edge{
			{SourceID: 1, TargetID: 2, Type: "CALLS"},
			{SourceID: 1, TargetID: 2, Type: "DEFINES"},
			{SourceID: 1, TargetID: 99, Type: "CALLS"},
		},
		Bodies: []api.QueryBody{{
			QualifiedName: fqdn.QualifiedName, File: fqdn.Location.File, StartLine: 14, EndLine: 17, TotalLines: 4,
			Code: "    def get_fqdn(self):\n        if not hasattr(self, '_fqdn'):\n            self._fqdn = socket.getfqdn()\n        return self._fqdn",
		}},
	}
}

func TestQueryCompactListsRankedPageEdgesAndNumberedBodies(t *testing.T) {
	text := format.QueryCompact(queryFixture())
	for _, want := range []string{
		"count: 2 of 40\n",
		"nodes[2]{qn,kind,file,lines}:\n",
		"  django.core.mail.utils.CachedDnsName.get_fqdn,Method,django/core/mail/utils.py,14-17\n",
		"edges[1]{from,rel,to}:\n  EmailMessage.message,CALLS,CachedDnsName.get_fqdn\n",
		"bodies[1]:\n  - qn: django.core.mail.utils.CachedDnsName.get_fqdn\n",
		"    code[4 lines]:\n",
		"      16|             self._fqdn = socket.getfqdn()\n",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("query output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "NODE ") || strings.Contains(text, "BODIES") || strings.Contains(text, "DEFINES") {
		t.Fatalf("legacy or structural lines leaked:\n%s", text)
	}
}

func TestHelpForQueryPagesWithOffset(t *testing.T) {
	hints := format.HelpForQuery(queryFixture(), "where is the Message-ID domain built", 0)
	if len(hints) != 1 || hints[0] != `38 more ranked: so graph query "where is the Message-ID domain built" --offset 2` {
		t.Fatalf("hints=%v", hints)
	}
}

func TestHelpForQueryClippedBodyAndNoBodies(t *testing.T) {
	q := queryFixture()
	q.RowsTotal = 2
	q.Bodies[0].TotalLines = 30
	hints := format.HelpForQuery(q, "q", 0)
	if len(hints) != 1 || !strings.Contains(hints[0], "--from 18") {
		t.Fatalf("clipped body must name --from: %v", hints)
	}
	q.Bodies = nil
	hints = format.HelpForQuery(q, "q", 0)
	if len(hints) != 1 || hints[0] != "so graph snippet <qn>" {
		t.Fatalf("no bodies should suggest snippet: %v", hints)
	}
}

func TestQueryCompactEmptyIsDefinitive(t *testing.T) {
	text := format.QueryCompact(api.QueryResult{})
	if !strings.Contains(text, "count: 0\n") || !strings.Contains(text, "no indexed symbol matches") {
		t.Fatalf("%q", text)
	}
	hints := format.HelpForQuery(api.QueryResult{}, "x", 0)
	if len(hints) != 2 {
		t.Fatalf("empty query should offer search and code-search: %v", hints)
	}
}

func TestHelpForQueryImpactShaped(t *testing.T) {
	hints := format.HelpForQuery(queryFixture(), "DNS_NAME usages across codebase", 0)
	joined := strings.Join(hints, "\n")
	if !strings.Contains(joined, "graph impact") {
		t.Fatalf("usage-shaped query should hint impact: %v", hints)
	}
}

func TestImpactCompactFiles(t *testing.T) {
	text := format.ImpactCompact(api.ImpactResult{
		Base:  "main",
		Total: 2,
		ImpactedFiles: []api.ImpactedFile{
			{Path: "pkg/dispatch.py", Symbols: 1, Reasons: []string{"caller"}},
			{Path: "pkg/http.py", Symbols: 1, Reasons: []string{"impl", "caller"}},
		},
	})
	if !strings.Contains(text, "impacted_files[2]{path,symbols,reasons}:") || !strings.Contains(text, "  pkg/http.py,1,impl;caller") {
		t.Fatalf("%q", text)
	}
	if empty := format.ImpactCompact(api.ImpactResult{}); !strings.Contains(empty, "impacted_files: none") {
		t.Fatalf("empty impact must be definitive: %q", empty)
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
	if !strings.Contains(text, "direction: incoming") || !strings.Contains(text, "callers[1]{qn,rel,hop}:\n  b.useLeaf,calls,1\n") {
		t.Fatalf("%q", text)
	}
	if strings.Contains(text, "callees") {
		t.Fatalf("incoming labeled callees: %q", text)
	}
}

func TestHelpForSnippetOnlyWhenClipped(t *testing.T) {
	if hints := format.HelpForSnippet(api.SnippetResult{QualifiedName: "a.leaf"}); len(hints) != 0 {
		t.Fatalf("a complete body is self-contained: %v", hints)
	}
	hints := format.HelpForSnippet(api.SnippetResult{QualifiedName: "a.leaf", Clipped: true, OmittedStart: 252, OmittedEnd: 400})
	if len(hints) != 1 || !strings.Contains(hints[0], "so graph snippet a.leaf --from 252") {
		t.Fatalf("hints=%v", hints)
	}
}

func TestSnippetCompactFieldsAndNumberedCode(t *testing.T) {
	text := format.SnippetCompact(api.SnippetResult{
		QualifiedName: "pkg.Foo.bar",
		Name:          "bar",
		Label:         "Method",
		Location:      api.Location{File: "foo.go", StartLine: 1674, EndLine: 1675},
		Code:          "func bar() {\n}\n",
		Clipped:       true, OmittedStart: 1676, OmittedEnd: 1700,
	})
	for _, want := range []string{"symbol:\n", "  qn: pkg.Foo.bar\n", "  file: foo.go\n", "  lines: 1674-1675\n", "code[2 of 27 lines]:\n", "  1674| func bar() {\n"} {
		if !strings.Contains(text, want) {
			t.Fatalf("snippet missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "src=") {
		t.Fatalf("file is printed once: %q", text)
	}
}

func TestCodeSearchCompactRowsAndEmpty(t *testing.T) {
	text := format.CodeSearchCompact(api.CodeSearchResult{
		Matches:          []api.CodeSearchMatch{{Location: api.Location{File: "a.py", StartLine: 7}, Source: `    x = "a, b"`}},
		TotalGrepMatches: 3,
	}, "a, b", "")
	if !strings.Contains(text, "count: 1 of 3") || !strings.Contains(text, `  a.py,7,"x = \"a, b\""`) {
		t.Fatalf("%q", text)
	}
	if hints := format.HelpForCodeSearch(api.CodeSearchResult{Matches: make([]api.CodeSearchMatch, 1), TotalGrepMatches: 3}, "p", 20); len(hints) != 2 {
		t.Fatalf("more matches should offer --limit and a path: %v", hints)
	}
	empty := format.CodeSearchCompact(api.CodeSearchResult{}, "zzz", "src")
	if !strings.Contains(empty, "count: 0") || !strings.Contains(empty, `no line in src contains "zzz"`) {
		t.Fatalf("%q", empty)
	}
}

func TestCellQuoting(t *testing.T) {
	cases := map[string]string{
		"django/core/mail/utils.py": "django/core/mail/utils.py",
		"14-17":                     "14-17",
		"42":                        "42",
		"":                          `""`,
		"a,b":                       `"a,b"`,
		`say "hi"`:                  `"say \"hi\""`,
		"-x":                        `"-x"`,
		" pad":                      `" pad"`,
	}
	for in, want := range cases {
		if got := format.Cell(in); got != want {
			t.Fatalf("Cell(%q) = %q, want %q", in, got, want)
		}
	}
}
