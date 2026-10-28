package format_test

import (
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/ishanjainn/superopen/internal/graph/format"
)

func TestCoverageFullShowsEveryRecordedFile(t *testing.T) {
	coverage := api.Coverage{Status: "partial", Total: 20}
	for i := 0; i < 20; i++ {
		coverage.Rows = append(coverage.Rows, api.CoverageRow{Path: "file" + string(rune('a'+i)) + ".go", Kind: "parse"})
	}
	brief := format.CoverageCompact(coverage, false)
	full := format.CoverageCompact(coverage, true)
	if !strings.Contains(brief, "count: 15 of 20") || strings.Contains(brief, "filet.go") {
		t.Fatalf("brief coverage not clipped: %q", brief)
	}
	if !strings.Contains(full, "count: 20 of 20") || !strings.Contains(full, "filet.go") {
		t.Fatalf("full coverage still clipped: %q", full)
	}
}

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
	if len(hints) != 1 || !strings.Contains(hints[0], `so graph code-search -- "make_msgid"`) {
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
	broad := queryFixture()
	broad.RowsTotal = 2000
	hints = format.HelpForQuery(broad, "verbose option parser", 0)
	if len(hints) != 2 || !strings.Contains(hints[0], "graph code-search") {
		t.Fatalf("broad queries should offer indexed literal search before paging: %v", hints)
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

func TestQueryCompactShowsOffPageLinkedSymbol(t *testing.T) {
	q := queryFixture()
	q.Linked = []api.QueryLink{{From: "pkg.mail.EmailMessage.message", Rel: "CALLS",
		To: "pkg.mail.utils.DNS_NAME", File: "pkg/mail/utils.py", Start: 20, End: 20}}
	got := format.QueryCompact(q)
	if !strings.Contains(got, "linked[1]{from,rel,qn,location}:") ||
		!strings.Contains(got, "EmailMessage.message,CALLS,pkg.mail.utils.DNS_NAME,\"pkg/mail/utils.py:20\"") {
		t.Fatalf("linked target must be usable with graph snippet: %s", got)
	}
	json := format.QueryAgentJSON(q)
	if len(json["linked"].([]api.QueryLink)) != 1 {
		t.Fatalf("agent JSON lost linked targets: %+v", json)
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

func TestTraceCompactShowsRouteRelationships(t *testing.T) {
	start := api.Node{QualifiedName: "__route__ANY__/pet/{}"}
	caller := api.Node{QualifiedName: "app.update"}
	for _, rel := range []string{"HANDLES", "HTTP_CALLS", "REDIRECTS_TO"} {
		got := format.TraceCompact(api.TraceResult{Direction: "incoming", Paths: [][]api.TraceStep{{
			{Node: start, Hop: 0}, {Node: caller, Via: &api.Edge{Type: rel}, Hop: 1},
		}}})
		if !strings.Contains(got, "app.update,"+strings.ToLower(rel)+",1") {
			t.Fatalf("%s relationship missing from compact trace: %s", rel, got)
		}
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
	withSymbol := api.CodeSearchResult{Matches: []api.CodeSearchMatch{{Location: api.Location{File: "a.py", StartLine: 7},
		Node: &api.Node{QualifiedName: "pkg.Options.run"}, Source: `verbose = True`}}}
	text = format.CodeSearchCompact(withSymbol, "verbose", "")
	if !strings.Contains(text, "matches[1]{file,line,qn,text}:") || !strings.Contains(text, "a.py,7,pkg.Options.run") {
		t.Fatalf("symbol qualified name missing from literal hit: %q", text)
	}
	if hints := format.HelpForCodeSearch(withSymbol, "verbose", 20); len(hints) != 1 || !strings.Contains(hints[0], `graph snippet "a.py" --from 1 --lines 30`) {
		t.Fatalf("literal hit should point to a bounded source window: %v", hints)
	}
	withSymbol.Related = []api.QueryLink{{From: "pkg.Options.run", Rel: "used_by", To: "pkg.preprocess", File: "a.py", Start: 9, End: 20}}
	withSymbol.Shared = []api.CodeSearchSharedValue{{Value: "pkg.Options", Consumers: []string{"pkg.Options.run", "pkg.preprocess"},
		Uses: []api.CodeSearchUseSite{{Consumer: "pkg.Options.run", File: "a.py", Line: 6, Expr: "verbose = True"}}}}
	withSymbol.Tests = []api.CodeSearchTest{{QualifiedName: "tests.test_options.test_verbose", Target: "pkg.Options.run", File: "tests/test_options.py", Line: 7}}
	withSymbol.TestRuns = []string{"python3 -m pytest 'tests/test_options.py'"}
	withSymbol.NameMatches = []api.CodeSearchMatch{{Location: api.Location{File: "register.py", StartLine: 12}, Node: &api.Node{QualifiedName: "pkg.register"}, Source: `"verbose", {}`}}
	withSymbol.Bodies = []api.QueryBody{{QualifiedName: "pkg.Options.run", File: "a.py", StartLine: 5, EndLine: 7, TotalLines: 3, Code: "def run():\n    verbose = True\n    return verbose"}}
	text = format.CodeSearchCompact(withSymbol, "verbose", "")
	if !strings.Contains(text, "name_matches[1]{file,line,qn,text}:") || !strings.Contains(text, "shared[1]{value,consumers}:") || !strings.Contains(text, "uses[1]{value,consumer,location,expr}:") || !strings.Contains(text, "test_scope: inject the issue input at the shared producer") || !strings.Contains(text, "related[1]{from,rel,qn,location}:") || !strings.Contains(text, "tests[1]{qn,target,file,line}:") || !strings.Contains(text, "test_run[1]{command}:") || !strings.Contains(text, "contexts[1]:") || !strings.Contains(text, "6|     verbose = True") {
		t.Fatalf("literal result should show graph user and numbered source: %q", text)
	}
	if strings.Index(text, "shared[1]") > strings.Index(text, "matches[1]") {
		t.Fatalf("shared source risk should lead literal rows: %q", text)
	}
	if hints := format.HelpForCodeSearch(withSymbol, "verbose", 20); len(hints) != 0 {
		t.Fatalf("source already shown; help must not repeat it: %v", hints)
	}
	if hints := format.HelpForCodeSearch(api.CodeSearchResult{Matches: make([]api.CodeSearchMatch, 1), TotalGrepMatches: 3}, "p", 20); len(hints) != 3 || !strings.Contains(hints[0], "graph snippet") {
		t.Fatalf("more matches should offer --limit and a path: %v", hints)
	}
	empty := format.CodeSearchCompact(api.CodeSearchResult{}, "zzz", "src")
	if !strings.Contains(empty, "count: 0") || !strings.Contains(empty, `no line in src contains "zzz"`) {
		t.Fatalf("%q", empty)
	}
}

func TestGraphFullEscapesPresentationTruncation(t *testing.T) {
	long := strings.Repeat("x", 220)
	search := api.CodeSearchResult{Matches: []api.CodeSearchMatch{{Location: api.Location{File: "src/a.py", StartLine: 3}, Source: long}}}
	brief := format.CodeSearchCompact(search, "x", "")
	full := format.CodeSearchCompact(search, "x", "", true)
	if !strings.Contains(brief, "truncated, 220 chars; use --full") || strings.Contains(brief, long) {
		t.Fatalf("brief match lacks a usable truncation hint: %q", brief)
	}
	if !strings.Contains(full, long) || strings.Contains(full, "truncated") {
		t.Fatalf("full match is still clipped: %q", full)
	}
	architecture := api.ArchitectureResult{Packages: make([]api.PackageSummary, 25)}
	if got := format.ArchitectureCompact(architecture); !strings.Contains(got, "count: 20 of 25") {
		t.Fatalf("default architecture page is not bounded: %q", got)
	}
	if got := format.ArchitectureCompact(architecture); !strings.Contains(got, "packages[20]{name,nodes}:") || strings.Contains(got, "fan_in") {
		t.Fatalf("unknown package flow should not be shown as zero: %q", got)
	}
	if got := format.ArchitectureCompact(architecture, true); !strings.Contains(got, "count: 25 of 25") {
		t.Fatalf("full architecture is still clipped: %q", got)
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
