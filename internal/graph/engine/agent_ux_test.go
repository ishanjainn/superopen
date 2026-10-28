package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/ishanjainn/superopen/internal/graph/format"
)

// queryRowQNs is the qualified names of a page of rows, in order.
func queryRowQNs(rows []api.Node) []string {
	out := make([]string, len(rows))
	for i, n := range rows {
		out[i] = n.QualifiedName
	}
	return out
}

func queryHasRow(rows []api.Node, qn string) bool {
	for _, n := range rows {
		if n.QualifiedName == qn {
			return true
		}
	}
	return false
}

func queryBodyFor(bodies []api.QueryBody, qn string) (api.QueryBody, bool) {
	for _, b := range bodies {
		if b.QualifiedName == qn {
			return b, true
		}
	}
	return api.QueryBody{}, false
}

func TestQuerySeedingPrefersDottedSymbol(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		foo, err := builder.PutNode(api.Node{Project: "fixture", Label: "Class", Name: "Foo", QualifiedName: "pkg.Foo", Location: api.Location{File: "pkg/foo.go", StartLine: 1}})
		if err != nil {
			return err
		}
		bar, err := builder.PutNode(api.Node{Project: "fixture", Label: "Method", Name: "bar", QualifiedName: "pkg.Foo.bar", Location: api.Location{File: "pkg/foo.go", StartLine: 10, EndLine: 20}, Properties: api.Properties{"is_exported": true}})
		if err != nil {
			return err
		}
		other, err := builder.PutNode(api.Node{Project: "fixture", Label: "Method", Name: "enable", QualifiedName: "pkg.Other.enable", Location: api.Location{File: "pkg/other.go", StartLine: 5}})
		if err != nil {
			return err
		}
		helper, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "setup", QualifiedName: "pkg.setup", Location: api.Location{File: "pkg/setup.go", StartLine: 1}})
		if err != nil {
			return err
		}
		_, _ = foo, other
		_, err = builder.PutEdge(api.Edge{Project: "fixture", SourceID: bar, TargetID: helper, Type: "CALLS", Evidence: &api.Evidence{Strategy: "import_map", Confidence: 0.9, Location: &api.Location{File: "pkg/foo.go", StartLine: 12}}})
		return err
	})
	defer store.Close()

	result, err := store.Query(ctx, api.QueryRequest{
		Project:  "fixture",
		Question: "How does Foo.bar work and what does it configure?",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) == 0 || result.Rows[0].QualifiedName != "pkg.Foo.bar" {
		t.Fatalf("the dotted symbol must lead the page, got %v", queryRowQNs(result.Rows))
	}
	if queryHasRow(result.Rows, "pkg.Other.enable") {
		t.Fatalf("an unrelated method must not be listed: %v", queryRowQNs(result.Rows))
	}
}

func TestQueryAttachesBodyOfTheListedCallable(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	src := "package answer\nfunc answer() {\n\tenable()\n}\nfunc enable() {}\n"
	if err := os.WriteFile(filepath.Join(root, "answer.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/graph.db"
	store, err := OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{
			Name: "fixture", RootPath: root, Generation: "one",
			EngineVersion: "test", IndexedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		answer, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Function", Name: "answer", QualifiedName: "answer.answer",
			Location: api.Location{File: "answer.go", StartLine: 2, EndLine: 4},
		})
		if err != nil {
			return err
		}
		enable, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Function", Name: "enable", QualifiedName: "answer.enable",
			Location: api.Location{File: "answer.go", StartLine: 5, EndLine: 5},
		})
		if err != nil {
			return err
		}
		_, err = builder.PutEdge(api.Edge{Project: "fixture", SourceID: answer, TargetID: enable, Type: "CALLS"})
		return err
	})
	if closeErr := store.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	store, err = OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	got, err := store.Query(ctx, api.QueryRequest{Project: "fixture", Question: "answer"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rows) == 0 || got.Rows[0].QualifiedName != "answer.answer" {
		t.Fatalf("answer must lead: %v", queryRowQNs(got.Rows))
	}
	body, ok := queryBodyFor(got.Bodies, "answer.answer")
	if !ok || !strings.Contains(body.Code, "func answer") {
		t.Fatalf("the top row must carry its source: %+v", got.Bodies)
	}
	if len(got.Edges) != 1 || got.Edges[0].Type != "CALLS" {
		t.Fatalf("the call between two listed rows must be an edge: %+v", got.Edges)
	}

	empty := fixtureGraph(t, func(builder *Builder) error {
		_, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Function", Name: "answer", QualifiedName: "answer.answer",
			Location: api.Location{File: "answer.go", StartLine: 2, EndLine: 4},
		})
		return err
	})
	defer empty.Close()
	plain, err := empty.Query(ctx, api.QueryRequest{Project: "fixture", Question: "answer"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plain.Bodies) != 0 || len(plain.Rows) == 0 {
		t.Fatalf("missing source lists the row without a body: rows=%d bodies=%d", len(plain.Rows), len(plain.Bodies))
	}
}

func TestQuerySeedsCallableNotJSONFile(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		fn, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "registerPlugin", QualifiedName: "src.module.plugin.registerPlugin", Location: api.Location{File: "src/module.ts", StartLine: 10}})
		if err != nil {
			return err
		}
		dash, err := builder.PutNode(api.Node{Project: "fixture", Label: "File", Name: "dashboard.json", QualifiedName: "dashboard.json.__file__", Location: api.Location{File: "dashboard.json", StartLine: 1}})
		if err != nil {
			return err
		}
		for i := 0; i < 30; i++ {
			v, err := builder.PutNode(api.Node{
				Project: "fixture", Label: "Variable", Name: "title",
				QualifiedName: "dashboard.json.panel" + strconv.Itoa(i),
				Location:      api.Location{File: "dashboard.json", StartLine: i + 1},
			})
			if err != nil {
				return err
			}
			if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: dash, TargetID: v, Type: "DEFINES"}); err != nil {
				return err
			}
		}
		_ = fn
		return nil
	})
	defer store.Close()

	result, err := store.Query(ctx, api.QueryRequest{
		Project:  "fixture",
		Question: "How does this plugin register and load the dashboard?",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) == 0 || result.Rows[0].Name != "registerPlugin" {
		t.Fatalf("expected registerPlugin first, got %v", queryRowQNs(result.Rows))
	}
	for _, row := range result.Rows {
		if row.Label == "File" || (row.Label == "Variable" && row.Location.File == "dashboard.json") {
			t.Fatalf("JSON keys and data files are not answers: %+v", row)
		}
	}
}

func TestQueryPagesRanksAndReportsTheTrueTotal(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		for i := 0; i < 40; i++ {
			if _, err := builder.PutNode(api.Node{
				Project: "fixture", Label: "Function", Name: fmt.Sprintf("hub_leaf%d", i),
				QualifiedName: fmt.Sprintf("pkg.hub_leaf%d", i),
				Location:      api.Location{File: fmt.Sprintf("pkg/leaf%d.go", i), StartLine: 1},
			}); err != nil {
				return err
			}
		}
		return nil
	})
	defer store.Close()
	first, err := store.Query(ctx, api.QueryRequest{Project: "fixture", Question: "hub leaf"})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Rows) != queryDefaultLimit || first.RowsTotal != 40 || first.Offset != 0 {
		t.Fatalf("default page: rows=%d total=%d offset=%d", len(first.Rows), first.RowsTotal, first.Offset)
	}
	second, err := store.Query(ctx, api.QueryRequest{Project: "fixture", Question: "hub leaf", Offset: queryDefaultLimit})
	if err != nil {
		t.Fatal(err)
	}
	if second.Offset != queryDefaultLimit || len(second.Rows) != queryDefaultLimit || second.Rows[0].QualifiedName == first.Rows[0].QualifiedName {
		t.Fatalf("the next page must continue the ranking: %v", queryRowQNs(second.Rows))
	}
	// These fixture nodes have no source files, so neither page has bodies.
	seen := map[string]bool{}
	for _, n := range append(first.Rows, second.Rows...) {
		if seen[n.QualifiedName] {
			t.Fatalf("a row repeats across pages: %s", n.QualifiedName)
		}
		seen[n.QualifiedName] = true
	}
}

func TestQueryPromotesDirectCalleeIntoTheFirstPage(t *testing.T) {
	ctx := context.Background()
	var ranked []api.Node
	store := fixtureGraph(t, func(builder *Builder) error {
		for i := 0; i < 24; i++ {
			n := api.Node{Project: "fixture", Label: "Method", Name: fmt.Sprintf("choice%d", i),
				QualifiedName: fmt.Sprintf("pkg.Filter.choice%d", i),
				Location:      api.Location{File: fmt.Sprintf("pkg/choice%d.go", i), StartLine: 1}}
			if i == 3 {
				n.Name, n.QualifiedName = "field_choices", "pkg.Filter.field_choices"
			}
			if i == 20 {
				n.Name, n.QualifiedName = "get_choices", "pkg.Field.get_choices"
			}
			id, err := builder.PutNode(n)
			if err != nil {
				return err
			}
			n.ID = id
			ranked = append(ranked, n)
		}
		_, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: ranked[3].ID, TargetID: ranked[20].ID,
			Type: "CALLS", Evidence: &api.Evidence{Confidence: 1}})
		return err
	})
	defer store.Close()
	rows, err := store.prioritizeQueryRelations(ctx, "fixture", ranked,
		"Filter field choices ordering", []string{"filter", "choice", "ordering"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(ranked) || rows[3].ID != ranked[3].ID || rows[4].ID != ranked[20].ID {
		t.Fatalf("direct callee must follow its caller without duplicating rows: %v", queryRowQNs(rows[:6]))
	}
	var result api.QueryResult
	if err := store.fillQueryRows(ctx, &result, rows, api.QueryRequest{Project: "fixture", Limit: 5}); err != nil {
		t.Fatal(err)
	}
	if result.RowsTotal != len(ranked) || len(result.Edges) != 1 || result.Edges[0].Type != "CALLS" {
		t.Fatalf("first page must expose the call edge: rows=%d edges=%+v", result.RowsTotal, result.Edges)
	}
	linked, err := store.queryLinkedNodes(ctx, ranked[:4], nil, "filter choices")
	if err != nil {
		t.Fatal(err)
	}
	if len(linked) != 1 || linked[0].To != ranked[20].QualifiedName || linked[0].Rel != "CALLS" {
		t.Fatalf("off-page call target should keep its full qualified name: %+v", linked)
	}
}

func TestQueryDiversifiesTheFirstPageWithoutLosingRows(t *testing.T) {
	var rows []api.Node
	for i := 0; i < 25; i++ {
		file := "pkg/crowded.go"
		if i >= 12 {
			file = fmt.Sprintf("pkg/other%d.go", i)
		}
		rows = append(rows, api.Node{ID: int64(i + 1), QualifiedName: fmt.Sprintf("pkg.symbol%d", i),
			Location: api.Location{File: file}})
	}
	got := diversifyQueryRows(rows)
	if len(got) != len(rows) {
		t.Fatalf("row count changed: %d", len(got))
	}
	for i := 0; i < queryDiverseLockedPrefix; i++ {
		if got[i].ID != rows[i].ID {
			t.Fatalf("top evidence moved: %v", queryRowQNs(got[:5]))
		}
	}
	seen := map[int64]bool{}
	for _, n := range got {
		if seen[n.ID] {
			t.Fatalf("duplicated row %d", n.ID)
		}
		seen[n.ID] = true
	}
	if got[4].ID != rows[12].ID || got[14].ID != rows[22].ID {
		t.Fatalf("other source files should fill the remaining first-page slots: %v", queryRowQNs(got[:15]))
	}
}

func TestQueryDiversificationDefersDocumentationExamples(t *testing.T) {
	rows := []api.Node{
		{ID: 1, Location: api.Location{File: "sphinx/ext/autodoc/__init__.py"}},
		{ID: 2, Location: api.Location{File: "sphinx/util/inspect.py"}},
		{ID: 3, Location: api.Location{File: "sphinx/ext/autodoc/__init__.py"}},
		{ID: 4, Location: api.Location{File: "sphinx/ext/autodoc/__init__.py"}},
		{ID: 5, Location: api.Location{File: "doc/development/examples/property.py"}},
		{ID: 6, Location: api.Location{File: "sphinx/ext/autodoc/importer.py"}},
		{ID: 7, Location: api.Location{File: "sphinx/domains/python.py"}},
	}
	got := diversifyQueryRows(rows)
	if got[4].ID != 6 || got[5].ID != 7 || got[6].ID != 5 {
		t.Fatalf("implementation should precede documentation examples: %v", queryRowQNs(got))
	}
}

func TestQueryBodiesDeferDocumentationExamples(t *testing.T) {
	property := api.Node{ID: 1, Label: "Method", Name: "can_document_member", Location: api.Location{File: "sphinx/ext/autodoc/__init__.py"}}
	isProperty := api.Node{ID: 2, Label: "Function", Name: "isproperty", Location: api.Location{File: "sphinx/util/inspect.py"}}
	example := api.Node{ID: 3, Label: "Method", Name: "can_document_member", Location: api.Location{File: "doc/development/examples/property.py"}}
	importer := api.Node{ID: 4, Label: "Function", Name: "get_class_members", Location: api.Location{File: "sphinx/ext/autodoc/importer.py"}}
	got := queryBodyCandidates([]api.Node{property, isProperty, example, importer}, []api.Edge{{SourceID: 1, TargetID: 2, Type: "CALLS"}}, []string{"property", "document"})
	if len(got) != 4 || got[0].ID != 1 || got[1].ID != 2 || got[2].ID != 4 || got[3].ID != 3 {
		t.Fatalf("documentation example should not consume an implementation body slot: %v", queryRowQNs(got))
	}
}

func TestQueryBodiesFollowRankBeforeBroadNameMatch(t *testing.T) {
	property := api.Node{ID: 1, Label: "Method", Name: "can_document_member", Location: api.Location{File: "sphinx/ext/autodoc/__init__.py"}}
	isProperty := api.Node{ID: 2, Label: "Function", Name: "isproperty", Location: api.Location{File: "sphinx/util/inspect.py"}}
	importer := api.Node{ID: 3, Label: "Function", Name: "get_class_members", Location: api.Location{File: "sphinx/ext/autodoc/importer.py"}}
	broad := api.Node{ID: 4, Label: "Method", Name: "add_autodocumenter", Location: api.Location{File: "sphinx/application.py"}}
	got := queryBodyCandidates([]api.Node{property, isProperty, importer, broad}, []api.Edge{{SourceID: 1, TargetID: 2, Type: "CALLS"}}, []string{"autodoc", "document", "property", "descriptor"})
	if len(got) != 4 || got[2].ID != importer.ID {
		t.Fatalf("earlier implementation should beat later broad name match: %v", queryRowQNs(got))
	}
}

func TestQueryBodiesPreferConnectedImplementationOverTests(t *testing.T) {
	test := api.Node{ID: 1, Label: "Method", Name: "test_choices", Location: api.Location{File: "tests/test_filters.py"}}
	caller := api.Node{ID: 2, Label: "Method", Name: "field_choices", Location: api.Location{File: "pkg/filters.py"}}
	other := api.Node{ID: 3, Label: "Method", Name: "unrelated", Location: api.Location{File: "pkg/other.py"}}
	callee := api.Node{ID: 4, Label: "Method", Name: "get_choices", Location: api.Location{File: "pkg/fields.py"}}
	rows := queryBodyCandidates([]api.Node{test, caller, other, callee}, []api.Edge{{SourceID: caller.ID, TargetID: callee.ID, Type: "CALLS"}}, nil)
	if len(rows) != 4 || rows[0].ID != caller.ID || rows[1].ID != callee.ID || rows[2].ID != other.ID || rows[3].ID != test.ID {
		t.Fatalf("source call chain must get body slots before unrelated rows: %v", queryRowQNs(rows))
	}
	if !queryAttachableNode(api.Node{Label: "Variable", Location: api.Location{File: "pkg/options.py"}}) ||
		queryAttachableNode(api.Node{Label: "Variable", Location: api.Location{File: "config.json"}}) {
		t.Fatal("source variables are attachable; data-file variables are not")
	}
}

func TestQueryBodiesIncludeSiblingImplementationInAnotherFile(t *testing.T) {
	root := api.Node{ID: 1, Label: "Method", Name: "field_choices", Location: api.Location{File: "admin/filters.py"}}
	callee := api.Node{ID: 2, Label: "Method", Name: "get_choices", Location: api.Location{File: "models/fields.py"}}
	unrelated := api.Node{ID: 3, Label: "Method", Name: "get_queryset", Location: api.Location{File: "admin/views.py"}}
	sibling := api.Node{ID: 4, Label: "Method", Name: "get_choices", Location: api.Location{File: "models/reverse.py"}}
	got := queryBodyCandidates([]api.Node{root, callee, unrelated, sibling},
		[]api.Edge{{SourceID: root.ID, TargetID: callee.ID, Type: "CALLS"}}, nil)
	if len(got) != 4 || got[0].ID != root.ID || got[1].ID != callee.ID || got[2].ID != sibling.ID {
		t.Fatalf("sibling implementation should use the third body slot: %v", queryRowQNs(got))
	}
}

func TestQueryBodiesUseQuestionNameWhenNoEdgeSurvives(t *testing.T) {
	parser := api.Node{ID: 1, Label: "Method", Name: "__init__", Location: api.Location{File: "config/parser.py"}}
	entry := api.Node{ID: 2, Label: "Method", Name: "run", Location: api.Location{File: "lint/run.py"}}
	verbose := api.Node{ID: 3, Label: "Function", Name: "_set_verbose_mode", Location: api.Location{File: "config/utils.py"}}
	got := queryBodyCandidates([]api.Node{parser, entry, verbose}, nil, []string{"verbose", "option"})
	if len(got) != 3 || got[0].ID != parser.ID || got[1].ID != verbose.ID {
		t.Fatalf("named behavior should use the next body slot: %v", queryRowQNs(got))
	}
}

func TestQueryBodiesKeepGraphUserAheadOfUnrelatedNameMatch(t *testing.T) {
	message := api.Node{ID: 1, Label: "Method", Name: "message", Location: api.Location{File: "core/mail/message.py"}}
	dns := api.Node{ID: 2, Label: "Variable", Name: "DNS_NAME", Location: api.Location{File: "core/mail/utils.py", StartLine: 20, EndLine: 20}}
	unrelated := api.Node{ID: 3, Label: "Method", Name: "normalize_email", Location: api.Location{File: "auth/user.py"}}
	smtp := api.Node{ID: 4, Label: "Method", Name: "open", Location: api.Location{File: "core/mail/smtp.py"}}
	got := queryBodyCandidates([]api.Node{message, dns, unrelated, smtp}, []api.Edge{
		{SourceID: message.ID, TargetID: dns.ID, Type: "USAGE"},
		{SourceID: smtp.ID, TargetID: dns.ID, Type: "USAGE"},
	}, []string{"email", "message", "dns", "name"})
	if len(got) < 3 || got[0].ID != message.ID || got[1].ID != smtp.ID {
		t.Fatalf("body slot should follow the graph user before lexical bystander: %v", queryRowQNs(got))
	}
}

func TestQueryBodiesSpendSourceSlotsOnImplementations(t *testing.T) {
	wrapper := api.Node{ID: 1, Label: "Method", Name: "__str__", Location: api.Location{File: "mail/utils.py", StartLine: 10, EndLine: 11}}
	fqdn := api.Node{ID: 2, Label: "Method", Name: "get_fqdn", Location: api.Location{File: "mail/utils.py", StartLine: 13, EndLine: 17}}
	message := api.Node{ID: 3, Label: "Method", Name: "message", Location: api.Location{File: "mail/message.py", StartLine: 239, EndLine: 264}}
	dns := api.Node{ID: 4, Label: "Variable", Name: "DNS_NAME", Location: api.Location{File: "mail/utils.py", StartLine: 20, EndLine: 20}}
	smtp := api.Node{ID: 5, Label: "Method", Name: "open", Location: api.Location{File: "mail/smtp.py", StartLine: 41, EndLine: 73}}
	unrelated := api.Node{ID: 6, Label: "Method", Name: "send_mail", Location: api.Location{File: "auth/forms.py", StartLine: 100, EndLine: 120}}
	got := queryBodyCandidates([]api.Node{wrapper, fqdn, message, dns, smtp, unrelated}, []api.Edge{
		{SourceID: wrapper.ID, TargetID: fqdn.ID, Type: "CALLS"},
		{SourceID: message.ID, TargetID: dns.ID, Type: "USAGE"},
		{SourceID: smtp.ID, TargetID: dns.ID, Type: "USAGE"},
	}, []string{"dns", "name", "mail"})
	if len(got) < 3 || got[0].ID != fqdn.ID || got[1].ID != message.ID || got[2].ID != smtp.ID {
		t.Fatalf("wrapper and one-line alias should not displace the call paths: %v", queryRowQNs(got))
	}
}

func TestQueryBodiesKeepSingleShortDefinition(t *testing.T) {
	variable := api.Node{ID: 1, Label: "Variable", Name: "FLAG", Location: api.Location{File: "settings.py", StartLine: 8, EndLine: 8}}
	got := queryBodyCandidates([]api.Node{variable}, nil, nil)
	if len(got) != 1 || got[0].ID != variable.ID {
		t.Fatalf("single-line definition must remain readable: %+v", got)
	}
}

func TestQueryPromotesShortWrapperCalleeInSameFile(t *testing.T) {
	ctx := context.Background()
	var wrapper, callee api.Node
	store := fixtureGraph(t, func(builder *Builder) error {
		wrapper = api.Node{Project: "fixture", Label: "Method", Name: "__str__", QualifiedName: "mail.CachedDnsName.__str__", Location: api.Location{File: "mail/utils.py", StartLine: 10, EndLine: 11}}
		callee = api.Node{Project: "fixture", Label: "Method", Name: "get_fqdn", QualifiedName: "mail.CachedDnsName.get_fqdn", Location: api.Location{File: "mail/utils.py", StartLine: 13, EndLine: 17}}
		id, err := builder.PutNode(wrapper)
		if err != nil {
			return err
		}
		wrapper.ID = id
		id, err = builder.PutNode(callee)
		if err != nil {
			return err
		}
		callee.ID = id
		_, err = builder.PutEdge(api.Edge{Project: "fixture", SourceID: wrapper.ID, TargetID: callee.ID, Type: "CALLS", Evidence: &api.Evidence{Strategy: "import_map", Confidence: 0.9}})
		return err
	})
	defer store.Close()
	other := api.Node{ID: 999, Label: "Method", Name: "other", QualifiedName: "mail.other", Location: api.Location{File: "mail/message.py", StartLine: 1, EndLine: 20}}
	got, err := store.prioritizeQueryRelations(ctx, "fixture", []api.Node{wrapper, other}, "hostname encoding", []string{"hostname", "encoding"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[1].QualifiedName != callee.QualifiedName {
		t.Fatalf("short wrapper's callee should be beside it: %v", queryRowQNs(got))
	}
}

func TestQueryPromotesBothRelevantCalleesOfShortEntryPoint(t *testing.T) {
	ctx := context.Background()
	var ranked []api.Node
	store := fixtureGraph(t, func(builder *Builder) error {
		for i := 0; i < 24; i++ {
			n := api.Node{Project: "fixture", Label: "Method", Name: fmt.Sprintf("other%d", i),
				QualifiedName: fmt.Sprintf("pkg.Query.other%d", i),
				Location:      api.Location{File: fmt.Sprintf("pkg/other%d.py", i), StartLine: 1, EndLine: 10}}
			switch i {
			case 0:
				n.Name, n.QualifiedName, n.Location = "get_count", "pkg.Query.get_count", api.Location{File: "pkg/query.py", StartLine: 548, EndLine: 554}
			case 19:
				n.Name, n.QualifiedName, n.Location = "clone", "pkg.Query.clone", api.Location{File: "pkg/query.py", StartLine: 314, EndLine: 357}
			case 20:
				n.Name, n.QualifiedName, n.Location = "add_annotation", "pkg.Query.add_annotation", api.Location{File: "pkg/query.py", StartLine: 1101, EndLine: 1111}
			case 21:
				n.Name, n.QualifiedName, n.Location = "get_aggregation", "pkg.Query.get_aggregation", api.Location{File: "pkg/query.py", StartLine: 438, EndLine: 546}
			}
			id, err := builder.PutNode(n)
			if err != nil {
				return err
			}
			n.ID = id
			ranked = append(ranked, n)
		}
		for i, target := range ranked[19:22] {
			if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: ranked[0].ID, TargetID: target.ID,
				Type: "CALLS", Evidence: &api.Evidence{Confidence: 1,
					Location: &api.Location{File: "pkg/query.py", StartLine: 552 + i}}}); err != nil {
				return err
			}
		}
		return nil
	})
	defer store.Close()
	got, err := store.prioritizeQueryRelations(ctx, "fixture", ranked, "count unused annotations", []string{"count", "unused", "annotations"})
	if err != nil {
		t.Fatal(err)
	}
	if got[1].QualifiedName != "pkg.Query.get_aggregation" || got[2].QualifiedName != "pkg.Query.add_annotation" {
		t.Fatalf("the terminal call and annotation path should outrank clone: %v", queryRowQNs(got[:4]))
	}
}

func TestCodeSearchPrefersLiteralDefinitionsAcrossFiles(t *testing.T) {
	matches := []api.CodeSearchMatch{
		{Location: api.Location{File: "config/init.py", StartLine: 25}, Source: "verbose_mode = False"},
		{Location: api.Location{File: "config/init.py", StartLine: 37}, Source: "configure(verbose_mode)"},
		{Location: api.Location{File: "lint/base_options.py", StartLine: 539}, Source: `"verbose",`},
		{Location: api.Location{File: "config/utils.py", StartLine: 210}, Source: `"--verbose": handler`},
		{Location: api.Location{File: "script/log.py", StartLine: 3}, Source: `"--verbose", action="store_true"`},
		{Location: api.Location{File: "ChangeLog", StartLine: 7}, Source: `"verbose" feature`},
		{Location: api.Location{File: "tests/test_cli.py", StartLine: 10}, Source: `"verbose"`},
	}
	result := codeSearchResult(matches, nil, "verbose", 3)
	if result.TotalGrepMatches != 7 || len(result.Matches) != 3 {
		t.Fatalf("unexpected search page: %+v", result)
	}
	got := []string{result.Matches[0].Location.File, result.Matches[1].Location.File, result.Matches[2].Location.File}
	if got[0] != "lint/base_options.py" || got[1] != "config/utils.py" || got[2] != "config/init.py" {
		t.Fatalf("want literal definition and diverse source files before incidental uses: %v", got)
	}
}

func TestCodeSearchRanksCodeAheadOfWorkflowConfig(t *testing.T) {
	matches := []api.CodeSearchMatch{
		{Location: api.Location{File: ".github/workflows/release.yml", StartLine: 37}, Source: "twine upload --verbose dist/*"},
		{Location: api.Location{File: "pylint/config/utils.py", StartLine: 209}, Source: `"--verbose": (False, _set_verbose_mode),`},
		{Location: api.Location{File: "tests/test_self.py", StartLine: 1425}, Source: `run = Run(["--verbose"])`},
	}
	result := codeSearchResult(matches, nil, "--verbose", 3)
	if result.Matches[0].Location.File != "pylint/config/utils.py" || result.Matches[1].Location.File != "tests/test_self.py" {
		t.Fatalf("code and tests should rank before workflow config: %+v", result.Matches)
	}
}

func TestCodeSearchRanksCodeAheadOfChangelogs(t *testing.T) {
	matches := []api.CodeSearchMatch{
		{Location: api.Location{File: "CHANGES", StartLine: 1493}, Source: "* autodoc classmethod coroutines are not detected"},
		{Location: api.Location{File: "CHANGES.old", StartLine: 346}, Source: "- Added a classmethod directive"},
		{Location: api.Location{File: "sphinx/domains/python.py", StartLine: 717}, Source: "'classmethod': directives.flag,"},
	}
	result := codeSearchResult(matches, nil, "classmethod", 3)
	if result.Matches[0].Location.File != "sphinx/domains/python.py" {
		t.Fatalf("implementation should lead changelog mentions: %+v", result.Matches)
	}
}

func TestCodeSearchRanksDecoratorConsumersAheadOfBareDecorators(t *testing.T) {
	matches := []api.CodeSearchMatch{
		{Location: api.Location{File: "sphinx/util/inventory.py", StartLine: 83}, Source: "@classmethod"},
		{Location: api.Location{File: "sphinx/ext/autodoc/importer.py", StartLine: 280}, Source: "if isinstance(descriptor, classmethod):"},
		{Location: api.Location{File: "sphinx/domains/python.py", StartLine: 717}, Source: "'classmethod': directives.flag,"},
	}
	result := codeSearchResult(matches, nil, "classmethod", 3)
	if result.Matches[0].Location.File != "sphinx/domains/python.py" || result.Matches[1].Location.File != "sphinx/ext/autodoc/importer.py" {
		t.Fatalf("generic decorator lines should follow consumer logic: %+v", result.Matches)
	}
}

func TestCodeSearchSharedValuesRequireMatchedUse(t *testing.T) {
	links := []api.QueryLink{
		{From: "sphinx.util.inventory.InventoryFile", Rel: "used_by", To: "sphinx.ext.intersphinx.fetch_inventory"},
		{From: "mail.message", Rel: "uses", To: "mail.DNS_NAME"},
		{From: "mail.DNS_NAME", Rel: "used_by", To: "mail.smtp.open"},
	}
	shared := codeSearchSharedValues(links, nil)
	if len(shared) != 1 || shared[0].Value != "mail.DNS_NAME" {
		t.Fatalf("only a source value at the matched line should be shared: %+v", shared)
	}
	direct := codeSearchSharedValues(links[2:], []api.CodeSearchMatch{{Node: &api.Node{Label: "Variable", QualifiedName: "mail.DNS_NAME"}}})
	if len(direct) != 0 {
		t.Fatalf("one consumer is not a shared-value summary: %+v", direct)
	}
	direct = codeSearchSharedValues([]api.QueryLink{
		{From: "mail.DNS_NAME", Rel: "used_by", To: "mail.message"},
		{From: "mail.DNS_NAME", Rel: "used_by", To: "mail.smtp.open"},
	}, []api.CodeSearchMatch{{Node: &api.Node{Label: "Variable", QualifiedName: "mail.DNS_NAME"}}})
	if len(direct) != 1 || len(direct[0].Consumers) != 2 {
		t.Fatalf("direct variable matches should summarize their users: %+v", direct)
	}
}

func TestCodeSearchRanksBehaviorAheadOfImportsAndExports(t *testing.T) {
	matches := []api.CodeSearchMatch{
		{Location: api.Location{File: "django/core/mail/__init__.py", StartLine: 18}, Source: "'CachedDnsName', 'DNS_NAME', 'EmailMessage',"},
		{Location: api.Location{File: "django/core/mail/utils.py", StartLine: 20}, Source: "DNS_NAME = CachedDnsName()"},
		{Location: api.Location{File: "django/core/mail/message.py", StartLine: 18}, Source: "from django.core.mail.utils import DNS_NAME"},
		{Location: api.Location{File: "django/core/mail/message.py", StartLine: 259}, Source: "# Use cached DNS_NAME for performance"},
		{Location: api.Location{File: "django/core/mail/message.py", StartLine: 260}, Source: "msg['Message-ID'] = make_msgid(domain=DNS_NAME)"},
	}
	result := codeSearchResult(matches, nil, "DNS_NAME", 5)
	if result.Matches[0].Location.File != "django/core/mail/utils.py" || result.Matches[0].Location.StartLine != 20 {
		t.Fatalf("a shared value's definition should lead, got %+v", result.Matches)
	}
	if result.Matches[1].Location.File != "django/core/mail/message.py" {
		t.Fatalf("consumer should precede import/export, got %+v", result.Matches)
	}
	makeMsgID := codeSearchResult([]api.CodeSearchMatch{
		{Location: api.Location{File: "django/core/mail/__init__.py", StartLine: 12}, Source: "    forbid_multi_line_headers, make_msgid,"},
		{Location: api.Location{File: "django/core/mail/message.py", StartLine: 260}, Source: "msg['Message-ID'] = make_msgid(domain=DNS_NAME)"},
	}, nil, "make_msgid", 2)
	if makeMsgID.Matches[0].Location.File != "django/core/mail/message.py" {
		t.Fatalf("a call must rank ahead of a multiline re-export: %+v", makeMsgID.Matches)
	}
}

func TestCodeSearchHitLinksToNarrowestIndexedSymbol(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		for _, n := range []api.Node{
			{Project: "fixture", Label: "Class", Name: "Options", QualifiedName: "pkg.Options", Location: api.Location{File: "pkg/options.py", StartLine: 1, EndLine: 50}},
			{Project: "fixture", Label: "Method", Name: "run", QualifiedName: "pkg.Options.run", Location: api.Location{File: "pkg/options.py", StartLine: 20, EndLine: 30}},
		} {
			if _, err := builder.PutNode(n); err != nil {
				return err
			}
		}
		return nil
	})
	defer store.Close()
	result := api.CodeSearchResult{Matches: []api.CodeSearchMatch{
		{Location: api.Location{File: "pkg/options.py", StartLine: 25}},
		{Location: api.Location{File: "pkg/options.py", StartLine: 60}},
	}}
	if err := store.attachCodeSearchSymbols(ctx, "fixture", &result); err != nil {
		t.Fatal(err)
	}
	if result.Matches[0].Node == nil || result.Matches[0].Node.QualifiedName != "pkg.Options.run" || result.Matches[1].Node != nil {
		t.Fatalf("line hits must identify their narrowest indexed symbol: %+v", result.Matches)
	}
}

func TestCodeSearchShowsMatchContextAndGraphUser(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	file := filepath.Join(root, "utils.py")
	source := "PREPROCESSABLE_OPTIONS = {\n    '--verbose': set_verbose,\n}\n\ndef preprocess(args):\n    if args[0] in PREPROCESSABLE_OPTIONS:\n        value = PREPROCESSABLE_OPTIONS[args[0]]\n"
	for i := 0; i < 35; i++ {
		source += fmt.Sprintf("        # parser step %d\n", i)
	}
	source += "        return value\n"
	if err := os.WriteFile(file, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(t.TempDir(), "graph.db")
	store, err := OpenWritable(db)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{Name: "fixture", RootPath: root, Generation: "one", EngineVersion: "test", IndexedAt: time.Now().UTC()}); err != nil {
			return err
		}
		variable, err := builder.PutNode(api.Node{Project: "fixture", Label: "Variable", Name: "PREPROCESSABLE_OPTIONS", QualifiedName: "utils.PREPROCESSABLE_OPTIONS", Location: api.Location{File: "utils.py", StartLine: 1, EndLine: 3}})
		if err != nil {
			return err
		}
		function, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "preprocess", QualifiedName: "utils.preprocess", Location: api.Location{File: "utils.py", StartLine: 5, EndLine: 43}})
		if err != nil {
			return err
		}
		_, err = builder.PutEdge(api.Edge{Project: "fixture", SourceID: function, TargetID: variable, Type: "USAGE"})
		return err
	})
	if closeErr := store.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	store, err = OpenReadOnly(db)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	result := api.CodeSearchResult{Matches: []api.CodeSearchMatch{{Location: api.Location{File: "utils.py", StartLine: 2}, Source: "    '--verbose': set_verbose,"}}}
	if err := store.attachCodeSearchSymbols(ctx, "fixture", &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Related) != 1 || result.Related[0].To != "utils.preprocess" || result.Related[0].Rel != "used_by" {
		t.Fatalf("missing graph user of literal definition: %+v", result.Related)
	}
	if len(result.Bodies) != 2 || result.Bodies[1].QualifiedName != "utils.preprocess" ||
		!strings.Contains(result.Bodies[0].Code, "if args[0] in PREPROCESSABLE_OPTIONS") ||
		!strings.Contains(result.Bodies[0].Code, "return value") {
		t.Fatalf("missing match-centered source context: %+v", result.Bodies)
	}
	formatted := format.CodeSearchCompact(result, "--verbose", "")
	if !strings.Contains(formatted, "related[1]") || !strings.Contains(formatted, "contexts[2]") {
		t.Fatalf("agent output missing relation/context: %s", formatted)
	}
}

func TestCodeSearchShowsOtherUsersOfMatchedValue(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tests", "mail"), 0o700); err != nil {
		t.Fatal(err)
	}
	utilsSource := strings.Repeat("# package header\n", 6) +
		"class CachedDnsName:\n    def get_fqdn(self):\n        return socket.getfqdn()\n" +
		strings.Repeat("\n", 10) + "DNS_NAME = CachedDnsName()\n"
	for name, source := range map[string]string{
		"__init__.py":         "from .message import make_msgid\n__all__ = ['make_msgid']\n",
		"message.py":          "def message():\n    return make_msgid(domain=DNS_NAME)\n",
		"utils.py":            utilsSource,
		"smtp.py":             "def open():\n    connection_params = {'local_hostname': DNS_NAME.get_fqdn()}\n",
		"tests/mail/tests.py": "class MailTests:\n    def test_unicode_dns(self):\n        assert message()\n\nclass SMTPBackendTests:\n    def test_server_open(self):\n        assert open()\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	store, err := OpenWritable(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{Name: "fixture", RootPath: root, Generation: "one", EngineVersion: "test", IndexedAt: time.Now().UTC()}); err != nil {
			return err
		}
		message, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "message", QualifiedName: "mail.message", Location: api.Location{File: "message.py", StartLine: 1, EndLine: 2}})
		if err != nil {
			return err
		}
		dns, err := builder.PutNode(api.Node{Project: "fixture", Label: "Variable", Name: "DNS_NAME", QualifiedName: "mail.DNS_NAME", Location: api.Location{File: "utils.py", StartLine: 20, EndLine: 20}})
		if err != nil {
			return err
		}
		smtp, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "open", QualifiedName: "mail.smtp.open", Location: api.Location{File: "smtp.py", StartLine: 1, EndLine: 2}})
		if err != nil {
			return err
		}
		guessed, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "unrelated", QualifiedName: "mail.unrelated", Location: api.Location{File: "smtp.py", StartLine: 1, EndLine: 2}})
		if err != nil {
			return err
		}
		if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: guessed, TargetID: dns, Type: "USAGE", Evidence: &api.Evidence{Confidence: 0.1}}); err != nil {
			return err
		}
		for _, test := range []struct {
			name, qn string
			start    int
			target   int64
		}{
			{"test_unicode_dns", "tests.mail.tests.MailTests.test_unicode_dns", 2, message},
			{"test_server_open", "tests.mail.tests.SMTPBackendTests.test_server_open", 6, smtp},
		} {
			node, err := builder.PutNode(api.Node{Project: "fixture", Label: "Method", Name: test.name, QualifiedName: test.qn,
				Location: api.Location{File: "tests/mail/tests.py", StartLine: test.start, EndLine: test.start + 1}})
			if err != nil {
				return err
			}
			if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: node, TargetID: test.target, Type: "TESTS"}); err != nil {
				return err
			}
		}
		for _, from := range []int64{message, smtp} {
			if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: from, TargetID: dns, Type: "USAGE"}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	result := api.CodeSearchResult{TotalGrepMatches: 3, Matches: []api.CodeSearchMatch{
		{Location: api.Location{File: "__init__.py", StartLine: 1}, Source: "from .message import make_msgid"},
		{Location: api.Location{File: "message.py", StartLine: 2}, Source: "    return make_msgid(domain=DNS_NAME)"},
		{Location: api.Location{File: "__init__.py", StartLine: 2}, Source: "__all__ = ['make_msgid']"},
	}}
	if err := store.attachCodeSearchSymbols(ctx, "fixture", &result, "DNS_NAME"); err != nil {
		t.Fatal(err)
	}
	if len(result.Related) < 2 || result.Related[0].To != "mail.DNS_NAME" || result.Related[1].To != "mail.smtp.open" {
		t.Fatalf("shared value and its other user should be linked: %+v", result.Related)
	}
	if len(result.Shared) != 1 || result.Shared[0].Value != "mail.DNS_NAME" ||
		len(result.Shared[0].Consumers) != 2 || result.Shared[0].Consumers[0] != "mail.message" ||
		result.Shared[0].Consumers[1] != "mail.smtp.open" {
		t.Fatalf("both consumers should be explicit in the shared-value summary: %+v", result.Shared)
	}
	if len(result.Shared[0].Uses) != 2 ||
		!strings.Contains(result.Shared[0].Uses[0].Expr, "make_msgid(domain=DNS_NAME)") ||
		!strings.Contains(result.Shared[0].Uses[1].Expr, "local_hostname") {
		t.Fatalf("shared uses should include the actual boundary expressions: %+v", result.Shared[0].Uses)
	}
	for _, link := range result.Related {
		if link.To == "mail.unrelated" {
			t.Fatalf("guessed edge must not displace source context: %+v", result.Related)
		}
	}
	if len(result.Bodies) != 5 || result.Bodies[1].QualifiedName != "mail.DNS_NAME" ||
		!strings.Contains(result.Bodies[1].Code, "class CachedDnsName:") ||
		result.Bodies[2].QualifiedName != "mail.smtp.open" ||
		result.Bodies[3].QualifiedName != "tests.mail.tests.MailTests.test_unicode_dns" ||
		result.Bodies[4].QualifiedName != "tests.mail.tests.SMTPBackendTests.test_server_open" {
		t.Fatalf("narrow literal should expose linked source windows: %+v", result.Bodies)
	}
	if len(result.Tests) < 2 || result.Tests[0].Target != "mail.message" || result.Tests[1].Target != "mail.smtp.open" {
		t.Fatalf("shared consumers should each have a test: %+v", result.Tests)
	}
	direct := api.CodeSearchResult{Matches: []api.CodeSearchMatch{{
		Location: api.Location{File: "utils.py", StartLine: 20}, Source: "DNS_NAME = CachedDnsName()",
	}}}
	if err := store.attachCodeSearchSymbols(ctx, "fixture", &direct); err != nil {
		t.Fatal(err)
	}
	if len(direct.Bodies) == 0 || !strings.Contains(direct.Bodies[0].Code, "class CachedDnsName:") {
		t.Fatalf("a direct value hit should show its nearby producer: %+v", direct.Bodies)
	}
}

func TestCodeSearchFindsRegisteredDirectiveHandler(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tests"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]string{
		"python.py":            "class PyProperty:\n    option_spec = {'type': str}\n    def run(self):\n        return self.options\n\nclass PythonDomain:\n    directives = {\n        'property': PyProperty,\n    }\n",
		"flags.py":             "option_spec = {'property': directives.flag}\nobject_types = {'property': ObjType('property')}\n",
		"tests/test_domain.py": "def test_property():\n    assert '.. py:property:: example'\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	store, err := OpenWritable(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{Name: "fixture", RootPath: root, Generation: "one", EngineVersion: "test", IndexedAt: time.Now().UTC()}); err != nil {
			return err
		}
		handler, err := builder.PutNode(api.Node{Project: "fixture", Label: "Class", Name: "PyProperty", QualifiedName: "python.PyProperty", Location: api.Location{File: "python.py", StartLine: 1, EndLine: 4}})
		if err != nil {
			return err
		}
		if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "Class", Name: "PythonDomain", QualifiedName: "python.PythonDomain", Location: api.Location{File: "python.py", StartLine: 6, EndLine: 9}}); err != nil {
			return err
		}
		test, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "test_property", QualifiedName: "tests.test_domain.test_property", Location: api.Location{File: "tests/test_domain.py", StartLine: 1, EndLine: 2}})
		if err != nil {
			return err
		}
		_, err = builder.PutEdge(api.Edge{Project: "fixture", SourceID: test, TargetID: handler, Type: "TESTS"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	result, err := CodeSearch(ctx, root, api.CodeSearchRequest{Pattern: "py:property", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.NameMatches) != 1 || !strings.Contains(result.NameMatches[0].Source, "'property': PyProperty") {
		t.Fatalf("namespaced directive should find its bare registry key: %+v", result.NameMatches)
	}
	if err := store.attachCodeSearchSymbols(ctx, "fixture", &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Related) == 0 || result.Related[0].Rel != "registers" || result.Related[0].To != "python.PyProperty" {
		t.Fatalf("directive key should route to the registered class: %+v", result.Related)
	}
	if len(result.Bodies) < 2 || result.Bodies[0].QualifiedName != "python.PyProperty" ||
		!strings.Contains(result.Bodies[0].Code, "option_spec") {
		t.Fatalf("parser body should be edit-ready: %+v", result.Bodies)
	}
	if len(result.Tests) != 1 || result.Tests[0].QualifiedName != "tests.test_domain.test_property" ||
		len(result.TestRuns) != 1 || result.TestRuns[0] != "python3 -m pytest 'tests/test_domain.py'" {
		t.Fatalf("matching test and runnable module should be attached: %+v / %+v", result.Tests, result.TestRuns)
	}
}

func TestCodeSearchShowsConsumerTestsAndSuiteCommand(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "tests", "mail"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]string{
		"message.py":          "def message():\n    return make_msgid(domain=DNS_NAME)\n",
		"tests/runtests.py":   "# test runner\n",
		"tests/mail/tests.py": "class MailTests:\n    def test_encoding(self):\n        assert message()\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	store, err := OpenWritable(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{Name: "fixture", RootPath: root, Generation: "one", EngineVersion: "test", IndexedAt: time.Now().UTC()}); err != nil {
			return err
		}
		method, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "message", QualifiedName: "mail.message", Location: api.Location{File: "message.py", StartLine: 1, EndLine: 2}})
		if err != nil {
			return err
		}
		test, err := builder.PutNode(api.Node{Project: "fixture", Label: "Method", Name: "test_encoding", QualifiedName: "tests.mail.tests.MailTests.test_encoding", Location: api.Location{File: "tests/mail/tests.py", StartLine: 2, EndLine: 3}})
		if err != nil {
			return err
		}
		_, err = builder.PutEdge(api.Edge{Project: "fixture", SourceID: test, TargetID: method, Type: "TESTS"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	result, err := CodeSearch(ctx, root, api.CodeSearchRequest{Pattern: "make_msgid(domain=DNS_NAME)", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.attachCodeSearchSymbols(ctx, "fixture", &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Tests) != 1 || result.Tests[0].Target != "mail.message" ||
		len(result.TestRuns) != 1 || result.TestRuns[0] != "PYTHONPATH=. python3 tests/runtests.py mail.tests" {
		t.Fatalf("graph test edge should provide a runnable module suite: %+v / %+v", result.Tests, result.TestRuns)
	}
	if len(result.Bodies) != 2 || result.Bodies[0].QualifiedName != "mail.message" ||
		result.Bodies[1].QualifiedName != "tests.mail.tests.MailTests.test_encoding" {
		t.Fatalf("implementation and test source should be paired: %+v", result.Bodies)
	}
}

func TestCodeSearchShowsDistinctClassesInOneFile(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	lines := []string{"class RelatedFieldListFilter:"}
	for i := 0; i < 34; i++ {
		lines = append(lines, fmt.Sprintf("    pad_%d = None", i))
	}
	lines = append(lines, "    def field_choices(self):", "        return self.field.get_choices()", "",
		"class RelatedOnlyFieldListFilter(RelatedFieldListFilter):", "    def field_choices(self):", "        return self.field.get_choices(ordering=())")
	if err := os.WriteFile(filepath.Join(root, "filters.py"), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "__init__.py"), []byte("from .filters import RelatedFieldListFilter\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := OpenWritable(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{Name: "fixture", RootPath: root, Generation: "one", EngineVersion: "test", IndexedAt: time.Now().UTC()}); err != nil {
			return err
		}
		for _, n := range []api.Node{
			{Project: "fixture", Label: "Class", Name: "RelatedFieldListFilter", QualifiedName: "filters.RelatedFieldListFilter", Location: api.Location{File: "filters.py", StartLine: 1, EndLine: 37}},
			{Project: "fixture", Label: "Class", Name: "RelatedOnlyFieldListFilter", QualifiedName: "filters.RelatedOnlyFieldListFilter", Location: api.Location{File: "filters.py", StartLine: 39, EndLine: 41}},
		} {
			if _, err := builder.PutNode(n); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	result := api.CodeSearchResult{Matches: []api.CodeSearchMatch{
		{Location: api.Location{File: "__init__.py", StartLine: 1}, Source: "from .filters import RelatedFieldListFilter"},
		{Location: api.Location{File: "filters.py", StartLine: 1}, Source: lines[0]},
		{Location: api.Location{File: "filters.py", StartLine: 39}, Source: lines[38]},
	}}
	if err := store.attachCodeSearchSymbols(ctx, "fixture", &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Bodies) != 2 || result.Bodies[0].QualifiedName != "filters.RelatedFieldListFilter" ||
		result.Bodies[1].QualifiedName != "filters.RelatedOnlyFieldListFilter" ||
		!strings.Contains(result.Bodies[0].Code, "def field_choices") ||
		!strings.Contains(result.Bodies[1].Code, "ordering=()") {
		t.Fatalf("both class behaviors should be visible without package re-exports: %+v", result.Bodies)
	}
}

func TestQueryNameOverlapUsesSymbolNameNotPackagePath(t *testing.T) {
	n := api.Node{Name: "QuerySet", QualifiedName: "django.db.models.query.QuerySet"}
	if queryNameOverlap(n, []string{"models"}) != 0 {
		t.Fatal("package-path token models must not score QuerySet")
	}
	if queryNameOverlap(n, []string{"queryset"}) < 10 {
		t.Fatal("exact name queryset should score")
	}
	annotation := api.Node{Name: "add_annotation", QualifiedName: "django.db.models.sql.query.Query.add_annotation"}
	if queryNameOverlap(annotation, []string{"annotations"}) == 0 {
		t.Fatal("plural question should find singular method")
	}
	reg := api.Node{Name: "register", QualifiedName: "pkg.Site.register"}
	if queryNameOverlap(reg, []string{"register", "display"}) < 10 {
		t.Fatal("exact method name register should score")
	}
	overlap := queryOverlapTerms("How does the admin register and display models?", nil)
	joined := strings.Join(overlap, " ")
	if !strings.Contains(joined, "register") || !strings.Contains(joined, "display") {
		t.Fatalf("overlap terms must keep register/display, got %v", overlap)
	}
	if strings.Contains(joined, "how") || strings.Contains(joined, "does") {
		t.Fatalf("overlap terms must skip grammar, got %v", overlap)
	}
}

func TestOrderQueryNodesPrefersQuestionMatchingMethods(t *testing.T) {
	seed := queryNodeHit{node: api.Node{ID: 1, Label: "Class", Name: "Site", QualifiedName: "pkg.Site"}, seed: true, hop: 0}
	method := queryNodeHit{node: api.Node{ID: 2, Label: "Method", Name: "register", QualifiedName: "pkg.Site.register"}, hop: 1, deg: 1}
	sib := queryNodeHit{node: api.Node{ID: 3, Label: "Class", Name: "Sibling", QualifiedName: "pkg.Sibling"}, hop: 1, deg: 80}
	byID := map[int64]queryNodeHit{1: seed, 2: method, 3: sib}
	terms := queryOverlapTerms("How does Site register and display models?", nil)
	got := orderQueryNodes([]int64{1}, byID, terms)
	if len(got) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(got))
	}
	if got[0].node.Name != "Site" {
		t.Fatalf("seed must stay first, got %q", got[0].node.Name)
	}
	if got[1].node.Name != "register" {
		t.Fatalf("question-matching method should outrank high-degree sibling, got %q then %q", got[1].node.Name, got[2].node.Name)
	}
}

func TestFileRoundRobinLocksTopSeedFile(t *testing.T) {
	a1 := queryNodeHit{node: api.Node{Name: "a1", Location: api.Location{File: "a.go"}}}
	a2 := queryNodeHit{node: api.Node{Name: "a2", Location: api.Location{File: "a.go"}}}
	b1 := queryNodeHit{node: api.Node{Name: "b1", Location: api.Location{File: "b.go"}}}
	b2 := queryNodeHit{node: api.Node{Name: "b2", Location: api.Location{File: "b.go"}}}
	got := fileRoundRobinNodes([]queryNodeHit{a1, a2, b1, b2})
	if len(got) != 4 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].node.Name != "a1" || got[1].node.Name != "a2" {
		t.Fatalf("lock file group must stay a contiguous prefix, got %q %q", got[0].node.Name, got[1].node.Name)
	}
	c1 := queryNodeHit{node: api.Node{Name: "c1", Location: api.Location{File: "c.go"}}}
	got = fileRoundRobinNodes([]queryNodeHit{a1, b1, b2, c1})
	if got[0].node.Name != "a1" {
		t.Fatalf("top seed file must stay first, got %q", got[0].node.Name)
	}
	if got[1].node.Name != "b1" || got[2].node.Name != "c1" || got[3].node.Name != "b2" {
		t.Fatalf("remaining files must round-robin, got %#v", []string{got[1].node.Name, got[2].node.Name, got[3].node.Name})
	}
}

func TestQueryAttachesMatchingCallableBodies(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	srcDir := filepath.Join(root, "pkg")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := strings.Join([]string{
		"class Site:",
		"    def ping(self):",
		"        return 1",
		"    def register(self, model):",
		"        self._registry[model] = True",
		"        return model",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(srcDir, "site.py"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/graph.db"
	store, err := OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{
			Name: "fixture", RootPath: root, Generation: "one",
			EngineVersion: "test", IndexedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		site, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Class", Name: "Site", QualifiedName: "pkg.Site",
			Location: api.Location{File: "pkg/site.py", StartLine: 1, EndLine: 6},
		})
		if err != nil {
			return err
		}
		ping, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Method", Name: "ping", QualifiedName: "pkg.Site.ping",
			Location: api.Location{File: "pkg/site.py", StartLine: 2, EndLine: 3},
		})
		if err != nil {
			return err
		}
		reg, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Method", Name: "register", QualifiedName: "pkg.Site.register",
			Location: api.Location{File: "pkg/site.py", StartLine: 4, EndLine: 6},
		})
		if err != nil {
			return err
		}
		if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: site, TargetID: ping, Type: "DEFINES_METHOD"}); err != nil {
			return err
		}
		if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: site, TargetID: reg, Type: "DEFINES_METHOD"}); err != nil {
			return err
		}
		return nil
	})
	if closeErr := store.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	store, err = OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	result, err := store.Query(ctx, api.QueryRequest{
		Project:  "fixture",
		Question: "How does Site.ping work?",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) == 0 || result.Rows[0].QualifiedName != "pkg.Site.ping" {
		t.Fatalf("the named method must lead: %v", queryRowQNs(result.Rows))
	}
	ping, ok := queryBodyFor(result.Bodies, "pkg.Site.ping")
	if !ok || !strings.Contains(ping.Code, "return 1") {
		t.Fatalf("ping body missing: %+v", result.Bodies)
	}
	if len(result.Bodies) > queryMaxBodies {
		t.Fatalf("bodies exceed the cap: %d", len(result.Bodies))
	}
}

func TestQueryMentionsPytestAndConftest(t *testing.T) {
	if !queryMentionsTests("where is the pytest fixture", nil) {
		t.Fatal("pytest in the question must skip the test-path penalty")
	}
	if !queryMentionsTests("open conftest hooks", nil) {
		t.Fatal("conftest in the question must skip the test-path penalty")
	}
	if queryMentionsTests("how does Site register models", nil) {
		t.Fatal("production question must keep the test-path penalty")
	}
	// A traceback frame's path is where code ran, not a question about tests.
	q := `TypeError File "/home/u/django/tests/expressions/tests.py", line 1218, in test_duration File "/home/u/django/django/db/models/query.py"`
	if queryMentionsTests(q, queryTerms(q, nil)) {
		t.Fatal("a test path inside pasted text must not switch the page to tests")
	}
	if !queryMentionsTests("which tests cover tests/expressions/tests.py", queryTerms("which tests cover tests/expressions/tests.py", nil)) {
		t.Fatal("tests asked in prose still count")
	}
}

func TestIsTestPathIncludesConftest(t *testing.T) {
	if !isTestPath("conftest.py") || !isTestPath("tests/conftest.py") || !isTestPath("tests/test_admin.py") {
		t.Fatal("conftest.py and test_*.py must be test paths")
	}
}

func TestQueryWideClassListsSameFileMethods(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		qs, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Class", Name: "QuerySet", QualifiedName: "pkg.QuerySet",
			Location: api.Location{File: "pkg/query.py", StartLine: 277, EndLine: 2040},
		})
		if err != nil {
			return err
		}
		for _, name := range []string{"filter", "_fetch_all", "iterator"} {
			method, err := builder.PutNode(api.Node{
				Project: "fixture", Label: "Method", Name: name, QualifiedName: "pkg.QuerySet." + name,
				Location:   api.Location{File: "pkg/query.py", StartLine: 400, EndLine: 430},
				Properties: api.Properties{"signature": "def " + name + "(self)"},
			})
			if err != nil {
				return err
			}
			if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: qs, TargetID: method, Type: "DEFINES_METHOD"}); err != nil {
				return err
			}
		}
		dummy, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Function", Name: "dummy", QualifiedName: "pkg.dummy",
			Location: api.Location{File: "pkg/dummy.py", StartLine: 1},
		})
		if err != nil {
			return err
		}
		for i := 0; i < 20; i++ {
			sib, err := builder.PutNode(api.Node{
				Project: "fixture", Label: "Class", Name: fmt.Sprintf("Sibling%02d", i),
				QualifiedName: fmt.Sprintf("pkg.Sibling%02d", i),
				Location:      api.Location{File: fmt.Sprintf("pkg/sib%02d.py", i), StartLine: 1, EndLine: 10},
			})
			if err != nil {
				return err
			}
			if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: qs, TargetID: sib, Type: "CALLS"}); err != nil {
				return err
			}
			if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: dummy, TargetID: sib, Type: "CALLS"}); err != nil {
				return err
			}
		}
		return nil
	})
	defer store.Close()
	result, err := store.Query(ctx, api.QueryRequest{
		Project:  "fixture",
		Question: "How does QuerySet.filter build its query",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !queryHasRow(result.Rows, "pkg.QuerySet") || !queryHasRow(result.Rows, "pkg.QuerySet.filter") {
		t.Fatalf("the class and the named method must be listed: %v", queryRowQNs(result.Rows))
	}
	rank := func(qn string) int {
		for i, n := range result.Rows {
			if n.QualifiedName == qn {
				return i
			}
		}
		return len(result.Rows)
	}
	if rank("pkg.QuerySet.filter") > rank("pkg.QuerySet._fetch_all") || rank("pkg.QuerySet.filter") > rank("pkg.QuerySet.iterator") {
		t.Fatalf("the named method must outrank unnamed siblings: %v", queryRowQNs(result.Rows))
	}
}

func TestQueryRoundRobinKeepsSiblingFileAfterSplice(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		qs, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Class", Name: "QuerySet", QualifiedName: "pkg.QuerySet",
			Location: api.Location{File: "pkg/query.py", StartLine: 1, EndLine: 200},
		})
		if err != nil {
			return err
		}
		for _, name := range []string{"filter", "exclude"} {
			method, err := builder.PutNode(api.Node{
				Project: "fixture", Label: "Method", Name: name, QualifiedName: "pkg.QuerySet." + name,
				Location: api.Location{File: "pkg/query.py", StartLine: 40, EndLine: 50},
			})
			if err != nil {
				return err
			}
			if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: qs, TargetID: method, Type: "DEFINES_METHOD"}); err != nil {
				return err
			}
		}
		helper, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Function", Name: "helper", QualifiedName: "pkg.helper",
			Location: api.Location{File: "pkg/manager.py", StartLine: 1, EndLine: 8},
		})
		if err != nil {
			return err
		}
		_, err = builder.PutEdge(api.Edge{Project: "fixture", SourceID: qs, TargetID: helper, Type: "CALLS"})
		return err
	})
	defer store.Close()
	result, err := store.Query(ctx, api.QueryRequest{
		Project:  "fixture",
		Question: "How does QuerySet filter helper work",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !queryHasRow(result.Rows, "pkg.helper") {
		t.Fatalf("a named symbol in a sibling file must be listed: %v", queryRowQNs(result.Rows))
	}
}

func TestTraceAmbiguousShortName(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "Method", Name: "init", QualifiedName: "pkg.A.init", Location: api.Location{File: "a.go", StartLine: 1}}); err != nil {
			return err
		}
		if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "Method", Name: "init", QualifiedName: "pkg.B.init", Location: api.Location{File: "b.go", StartLine: 1}}); err != nil {
			return err
		}
		return nil
	})
	defer store.Close()

	result, err := store.Trace(ctx, api.TraceRequest{Project: "fixture", Start: "init", Direction: "outgoing", Depth: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "ambiguous" {
		t.Fatalf("status=%q suggestions=%d", result.Status, len(result.Suggestions))
	}
	if len(result.Suggestions) < 2 {
		t.Fatalf("expected suggestions, got %d", len(result.Suggestions))
	}
	text := format.TraceCompact(result)
	if !strings.Contains(text, "status: ambiguous") {
		t.Fatalf("compact=%q", text)
	}
}

func TestCompactTraceCalleesSubsetOfJSON(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		caller, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "Caller", QualifiedName: "pkg.Caller", Location: api.Location{File: "caller.go", StartLine: 1}})
		if err != nil {
			return err
		}
		called, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "Called", QualifiedName: "pkg.Called", Location: api.Location{File: "called.go", StartLine: 1}, Properties: api.Properties{"bt": "noise", "fp": "deadbeef", "sp": "1,2,3"}})
		if err != nil {
			return err
		}
		_, err = builder.PutEdge(api.Edge{Project: "fixture", SourceID: caller, TargetID: called, Type: "CALLS"})
		return err
	})
	defer store.Close()

	full, err := store.Trace(ctx, api.TraceRequest{Project: "fixture", Start: "pkg.Caller", Direction: "outgoing", Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Paths) == 0 {
		t.Fatal("expected paths")
	}
	// Full JSON path still carries properties.
	foundProps := false
	for _, path := range full.Paths {
		for _, step := range path {
			if step.Node.QualifiedName == "pkg.Called" && step.Node.Properties["bt"] == "noise" {
				foundProps = true
			}
		}
	}
	if !foundProps {
		t.Fatal("expected full trace to retain properties")
	}
	compact := format.TraceCompact(full)
	if strings.Contains(compact, "deadbeef") || strings.Contains(compact, `"bt"`) {
		t.Fatalf("compact leaked properties: %q", compact)
	}
	if !strings.Contains(compact, "Called") {
		t.Fatalf("compact missing callee: %q", compact)
	}
	if len(compact) > 2000 {
		t.Fatalf("compact too large: %d", len(compact))
	}
}

func TestQueryHubSkipDoesNotExpandTransitHubs(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		leaf, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "Leaf", QualifiedName: "pkg.Leaf", Location: api.Location{File: "leaf.go", StartLine: 1}})
		if err != nil {
			return err
		}
		hub, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "Hub", QualifiedName: "pkg.Hub", Location: api.Location{File: "hub.go", StartLine: 1}})
		if err != nil {
			return err
		}
		if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: leaf, TargetID: hub, Type: "CALLS"}); err != nil {
			return err
		}
		for i := 0; i < 80; i++ {
			other, err := builder.PutNode(api.Node{
				Project: "fixture", Label: "Function",
				Name:          "Other",
				QualifiedName: "pkg.Other" + string(rune('A'+i%26)) + string(rune('0'+i/26)),
				Location:      api.Location{File: "other.go", StartLine: i + 1},
			})
			if err != nil {
				return err
			}
			if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: hub, TargetID: other, Type: "CALLS"}); err != nil {
				return err
			}
		}
		return nil
	})
	defer store.Close()

	result, err := store.Query(ctx, api.QueryRequest{
		Project:  "fixture",
		Question: "How does Leaf work?",
		Depth:    2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) == 0 || result.Rows[0].QualifiedName != "pkg.Leaf" {
		t.Fatalf("expected Leaf first, got %v", queryRowQNs(result.Rows))
	}
	others := 0
	for _, node := range result.Rows {
		if strings.HasPrefix(node.QualifiedName, "pkg.Other") {
			others++
		}
	}
	if others > 5 {
		t.Fatalf("hub skip should not pull the Other* neighborhood, got %d of %d rows", others, len(result.Rows))
	}
}

func TestQueryCountDoesNotLeadWithDunder(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		qs, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Class", Name: "QuerySet", QualifiedName: "django.db.models.query.QuerySet",
			Location: api.Location{File: "django/db/models/query.py", StartLine: 291, EndLine: 1993},
		})
		if err != nil {
			return err
		}
		for _, name := range []string{"__bool__", "__getitem__", "count"} {
			method, err := builder.PutNode(api.Node{
				Project: "fixture", Label: "Method", Name: name, QualifiedName: "django.db.models.query.QuerySet." + name,
				Location: api.Location{File: "django/db/models/query.py", StartLine: 400, EndLine: 420},
			})
			if err != nil {
				return err
			}
			if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: qs, TargetID: method, Type: "DEFINES_METHOD"}); err != nil {
				return err
			}
		}
		return nil
	})
	defer store.Close()
	result, err := store.Query(ctx, api.QueryRequest{
		Project:  "fixture",
		Question: "where is queryset.count() implemented and how does it build the SQL query, especially regarding annotations",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) == 0 || !strings.HasSuffix(result.Rows[0].QualifiedName, "QuerySet.count") {
		t.Fatalf("count must lead the page: %v", queryRowQNs(result.Rows))
	}
	for i, n := range result.Rows {
		if strings.Contains(n.Name, "__") && i < 1 {
			t.Fatalf("a dunder method must not lead: %v", queryRowQNs(result.Rows))
		}
	}
	if result.Rows[0].Name != "count" {
		t.Fatalf("the named method must come before its class and dunder siblings: %v", queryRowQNs(result.Rows))
	}
}

func TestSnippetMissListsSameSegment(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		_, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Method", Name: "get_choices",
			QualifiedName: "django.db.models.fields.related.ForeignKey.get_choices",
			Location:      api.Location{File: "django/db/models/fields/related.py", StartLine: 10, EndLine: 20},
		})
		return err
	})
	defer store.Close()
	_, err := store.Snippet(ctx, api.SnippetRequest{
		Project: "fixture", QualifiedName: "django.db.models.fields.Field.get_choices",
	})
	if err == nil || !strings.Contains(err.Error(), "symbol not found") {
		t.Fatalf("expected symbol not found, got %v", err)
	}
	if !strings.Contains(err.Error(), "ForeignKey.get_choices") || !strings.Contains(err.Error(), "related.py") {
		t.Fatalf("miss should name the indexed symbol: %v", err)
	}
}

func TestClippedSnippetListsCallees(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	var src strings.Builder
	src.WriteString("package pkg\nfunc helper() {}\nfunc big() {\n")
	for i := 0; i < 300; i++ {
		src.WriteString("\thelper()\n")
	}
	src.WriteString("}\n")
	if err := os.WriteFile(filepath.Join(root, "pkg", "big.go"), []byte(src.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/graph.db"
	store, err := OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{
			Name: "fixture", RootPath: root, Generation: "one",
			EngineVersion: "test", IndexedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		helper, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Function", Name: "helper", QualifiedName: "pkg.helper",
			Location: api.Location{File: "pkg/big.go", StartLine: 2, EndLine: 2},
		})
		if err != nil {
			return err
		}
		big, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Function", Name: "big", QualifiedName: "pkg.big",
			Location: api.Location{File: "pkg/big.go", StartLine: 3, EndLine: 304},
		})
		if err != nil {
			return err
		}
		_, err = builder.PutEdge(api.Edge{Project: "fixture", SourceID: big, TargetID: helper, Type: "CALLS"})
		return err
	})
	if closeErr := store.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	store, err = OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	got, err := store.Snippet(ctx, api.SnippetRequest{Project: "fixture", QualifiedName: "pkg.big"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Clipped {
		t.Fatalf("expected clipped snippet, lines %d-%d", got.Location.StartLine, got.Location.EndLine)
	}
	compact := format.SnippetCompact(got)
	if !strings.Contains(compact, "pkg.helper") || !strings.Contains(compact, "pkg/big.go") {
		t.Fatalf("clipped snippet missing callee: %q", compact)
	}
}

func TestLongBodyIsClippedNotSkipped(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	var src strings.Builder
	src.WriteString("package pkg\nfunc big() {\n")
	for i := 0; i < 80; i++ {
		src.WriteString("\tx := 1\n")
	}
	src.WriteString("}\nfunc small() { return }\n")
	if err := os.WriteFile(filepath.Join(root, "pkg", "bodies.go"), []byte(src.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/graph.db"
	store, err := OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{
			Name: "fixture", RootPath: root, Generation: "one",
			EngineVersion: "test", IndexedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		if _, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Function", Name: "big", QualifiedName: "pkg.big",
			Location: api.Location{File: "pkg/bodies.go", StartLine: 2, EndLine: 83},
		}); err != nil {
			return err
		}
		_, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Function", Name: "small", QualifiedName: "pkg.small",
			Location: api.Location{File: "pkg/bodies.go", StartLine: 84, EndLine: 84},
		})
		return err
	})
	if closeErr := store.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	store, err = OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	bodies := store.collectQueryBodies(ctx, "fixture", []api.Node{
		{Label: "Function", Name: "big", QualifiedName: "pkg.big", Location: api.Location{File: "pkg/bodies.go", StartLine: 2, EndLine: 83}},
		{Label: "Function", Name: "small", QualifiedName: "pkg.small", Location: api.Location{File: "pkg/bodies.go", StartLine: 84, EndLine: 84}},
	})
	big, ok := queryBodyFor(bodies, "pkg.big")
	if !ok || !strings.Contains(big.Code, "x := 1") {
		t.Fatalf("the top-ranked body must be attached, clipped if long: %+v", bodies)
	}
	if _, ok := queryBodyFor(bodies, "pkg.small"); !ok {
		t.Fatalf("the short body fits the remaining budget: %+v", bodies)
	}
}

func TestClippedCalleesPreferSameFile(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	var src strings.Builder
	src.WriteString("package pkg\nfunc helper() {}\nfunc big() {\n")
	for i := 0; i < 300; i++ {
		src.WriteString("\thelper()\n")
	}
	src.WriteString("}\n")
	if err := os.WriteFile(filepath.Join(root, "pkg", "big.go"), []byte(src.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/graph.db"
	store, err := OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{
			Name: "fixture", RootPath: root, Generation: "one",
			EngineVersion: "test", IndexedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		helper, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Function", Name: "helper", QualifiedName: "pkg.helper",
			Location: api.Location{File: "pkg/big.go", StartLine: 2, EndLine: 2},
		})
		if err != nil {
			return err
		}
		builtin, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Function", Name: "dict", QualifiedName: "builtins.dict",
			Location: api.Location{File: "<python-builtins>", StartLine: 1, EndLine: 1},
		})
		if err != nil {
			return err
		}
		other, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Method", Name: "items", QualifiedName: "django.contrib.flatpages.sitemaps.FlatPageSitemap.items",
			Location: api.Location{File: "django/contrib/flatpages/sitemaps.py", StartLine: 1, EndLine: 2},
		})
		if err != nil {
			return err
		}
		big, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Function", Name: "big", QualifiedName: "pkg.big",
			Location: api.Location{File: "pkg/big.go", StartLine: 3, EndLine: 304},
		})
		if err != nil {
			return err
		}
		for _, target := range []int64{builtin, other, helper} {
			if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: big, TargetID: target, Type: "CALLS"}); err != nil {
				return err
			}
		}
		return nil
	})
	if closeErr := store.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	store, err = OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	got, err := store.Snippet(ctx, api.SnippetRequest{Project: "fixture", QualifiedName: "pkg.big"})
	if err != nil {
		t.Fatal(err)
	}
	compact := format.SnippetCompact(got)
	if !strings.Contains(compact, "pkg.helper") {
		t.Fatalf("same-file callee missing: %q", compact)
	}
	if strings.Contains(compact, "builtins.dict") {
		t.Fatalf("builtin callee must be dropped: %q", compact)
	}
	helperAt := strings.Index(compact, "pkg.helper")
	otherAt := strings.Index(compact, "FlatPageSitemap.items")
	if otherAt >= 0 && helperAt > otherAt {
		t.Fatalf("same-file callee must come first: %q", compact)
	}
}

func TestStaleHeaderNamesDirtyFiles(t *testing.T) {
	got := StaleHeader(api.ChangeSet{Modified: []api.FileChange{{Path: "django/db/models/sql/query.py"}}})
	if !strings.HasPrefix(got, GraphStaleHeader) {
		t.Fatalf("header changed: %q", got)
	}
	if !strings.Contains(got, "query.py") || !strings.Contains(got, "absent from this answer") {
		t.Fatalf("dirty path missing: %q", got)
	}
	if StaleHeader(api.ChangeSet{}) != GraphStaleHeader {
		t.Fatal("empty changeset should keep the stable header")
	}
	var many []api.FileChange
	for i := 0; i < 20; i++ {
		many = append(many, api.FileChange{Path: fmt.Sprintf("file%d.go", i)})
	}
	if StaleHeader(api.ChangeSet{Modified: many}) != GraphStaleHeader {
		t.Fatal("a whole-tree dirty set must stay the single stale line")
	}
}

func TestQueryAgentJSONIsTheTextPage(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		root, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "Root", QualifiedName: "pkg.Root", Location: api.Location{File: "root.go", StartLine: 1}})
		if err != nil {
			return err
		}
		for i := 0; i < 40; i++ {
			child, err := builder.PutNode(api.Node{
				Project: "fixture", Label: "Function", Name: "Child",
				QualifiedName: "pkg.Child" + string(rune('A'+i%26)) + string(rune('0'+i/26)),
				Location:      api.Location{File: "child.go", StartLine: i + 1},
			})
			if err != nil {
				return err
			}
			if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: root, TargetID: child, Type: "CALLS"}); err != nil {
				return err
			}
		}
		return nil
	})
	defer store.Close()

	result, err := store.Query(ctx, api.QueryRequest{Project: "fixture", Question: "How does Root work?"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) < 2 {
		t.Fatalf("the page should list the root and its callees, got %d rows", len(result.Rows))
	}
	raw, err := json.Marshal(format.QueryAgentJSON(result))
	if err != nil {
		t.Fatal(err)
	}
	// Agent JSON is the page the text view shows: slim rows, no full
	// Node/Edge structs (ids, properties, evidence).
	if bytes.Contains(raw, []byte(`"properties"`)) || bytes.Contains(raw, []byte(`"source_id"`)) || bytes.Contains(raw, []byte(`"text"`)) {
		t.Fatalf("agent JSON must not include full node/edge structs: %s", raw)
	}
	if len(raw) > 64*1024 {
		t.Fatalf("agent JSON too large: %d", len(raw))
	}
	if !bytes.Contains(raw, []byte(`"total":`)) || !bytes.Contains(raw, []byte(`"qn":"pkg.Root"`)) {
		t.Fatalf("agent JSON missing the ranked page: %s", raw[:200])
	}
}

func TestCommunityLabelConsistentWithArchitecture(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		a, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "A", QualifiedName: "pkg.A", Location: api.Location{File: "a.go", StartLine: 1}})
		if err != nil {
			return err
		}
		b, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "B", QualifiedName: "pkg.B", Location: api.Location{File: "b.go", StartLine: 1}})
		if err != nil {
			return err
		}
		c, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "C", QualifiedName: "pkg.C", Location: api.Location{File: "c.go", StartLine: 1}})
		if err != nil {
			return err
		}
		for _, edge := range []api.Edge{
			{Project: "fixture", SourceID: a, TargetID: b, Type: "CALLS"},
			{Project: "fixture", SourceID: b, TargetID: c, Type: "CALLS"},
			{Project: "fixture", SourceID: c, TargetID: a, Type: "CALLS"},
		} {
			if _, err := builder.PutEdge(edge); err != nil {
				return err
			}
		}
		return nil
	})
	defer store.Close()

	labels, err := store.communityLabelByNodeID(ctx, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	arch, err := store.Architecture(ctx, api.ArchitectureRequest{Project: "fixture", Aspects: []string{"clusters"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(arch.Communities) == 0 {
		t.Fatal("expected communities")
	}
	// Every labeled node should map to one of the architecture community names.
	names := map[string]struct{}{}
	for _, community := range arch.Communities {
		names[community.Name] = struct{}{}
	}
	for _, name := range labels {
		if _, ok := names[name]; !ok && name != "" {
			t.Fatalf("label %q not in architecture communities %#v", name, names)
		}
	}
}

func fixtureGraph(t *testing.T, build func(*Builder) error) *Store {
	t.Helper()
	path := t.TempDir() + "/graph.db"
	store, err := OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(context.Background(), func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{
			Name: "fixture", RootPath: t.TempDir(), Generation: "one",
			EngineVersion: "test", IndexedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		return build(builder)
	})
	if closeErr := store.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	store, err = OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestSnippetAmbiguousQualifiedAndFilePath(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "export function App() { return null }\n"
	if err := os.WriteFile(filepath.Join(root, "src", "module.tsx"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/graph.db"
	store, err := OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{
			Name: "fixture", RootPath: root, Generation: "one",
			EngineVersion: "test", IndexedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "App", QualifiedName: "src.module.App", Location: api.Location{File: "src/module.tsx", StartLine: 1, EndLine: 1}}); err != nil {
			return err
		}
		if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "App", QualifiedName: "src.pages.App", Location: api.Location{File: "src/pages.tsx", StartLine: 1, EndLine: 1}}); err != nil {
			return err
		}
		if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "File", Name: "module.tsx", QualifiedName: "src.module.tsx.__file__", Location: api.Location{File: "src/module.tsx", StartLine: 1, EndLine: 1}}); err != nil {
			return err
		}
		return nil
	})
	if closeErr := store.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	store, err = OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ambiguous, err := store.Snippet(ctx, api.SnippetRequest{Project: "fixture", QualifiedName: "App"})
	if err != nil {
		t.Fatal(err)
	}
	if ambiguous.Status != "ambiguous" || len(ambiguous.Suggestions) < 2 {
		t.Fatalf("snippet App status=%q suggestions=%d", ambiguous.Status, len(ambiguous.Suggestions))
	}
	text := format.SnippetCompact(ambiguous)
	if !strings.Contains(text, "status: ambiguous") {
		t.Fatalf("compact=%q", text)
	}
	help := format.HelpForSnippet(ambiguous)
	if len(help) == 0 || !strings.Contains(help[0], "so graph snippet") {
		t.Fatalf("help=%v", help)
	}

	qualified, err := store.Snippet(ctx, api.SnippetRequest{Project: "fixture", QualifiedName: "src.module.App"})
	if err != nil {
		t.Fatal(err)
	}
	if qualified.Status == "ambiguous" || !strings.Contains(qualified.Code, "export function App") {
		t.Fatalf("qualified snippet: status=%q code=%q", qualified.Status, qualified.Code)
	}

	byPath, err := store.Snippet(ctx, api.SnippetRequest{Project: "fixture", QualifiedName: "src/module.tsx"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(byPath.Message), "symbol not found") || !strings.Contains(byPath.Code, "export function App") {
		t.Fatalf("path snippet failed: %#v", byPath)
	}
}

func TestTraceInboundUnresolvedCalls(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "statPanel", QualifiedName: "src.panels.statPanel", Location: api.Location{File: "src/panels.ts", StartLine: 1}}); err != nil {
			return err
		}
		return builder.PutUnresolved(api.UnresolvedRelationship{
			Project: "fixture", Source: "src.panels.statPanel", TargetText: "re.test", Type: "CALL_REFERENCE",
		})
	})
	defer store.Close()

	result, err := store.Trace(ctx, api.TraceRequest{Project: "fixture", Start: "src.panels.statPanel", Direction: "incoming", Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Paths) != 0 {
		t.Fatalf("expected empty inbound paths, got %d", len(result.Paths))
	}
	if result.UnresolvedCalls < 1 {
		t.Fatalf("expected unresolved_calls, got %d compact=%q", result.UnresolvedCalls, format.TraceCompact(result))
	}
	text := format.TraceCompact(result)
	if !strings.Contains(text, "unresolved_calls:") {
		t.Fatalf("compact missing unresolved_calls: %q", text)
	}
}

func TestQueryCallersListsIncomingNames(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		leaf, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "leaf", QualifiedName: "a.leaf", Location: api.Location{File: "a.ts", StartLine: 1, EndLine: 3}})
		if err != nil {
			return err
		}
		caller, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "useLeaf", QualifiedName: "b.useLeaf", Location: api.Location{File: "b.ts", StartLine: 1, EndLine: 5}})
		if err != nil {
			return err
		}
		if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "alone", QualifiedName: "a.alone", Location: api.Location{File: "a.ts", StartLine: 8, EndLine: 9}}); err != nil {
			return err
		}
		_, err = builder.PutEdge(api.Edge{Project: "fixture", SourceID: caller, TargetID: leaf, Type: "CALLS"})
		return err
	})
	defer store.Close()

	got, err := store.Query(ctx, api.QueryRequest{Project: "fixture", Question: "callers of leaf"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.CallersOf) != 1 || got.CallersOf[0] != "a.leaf" || len(got.Rows) != 1 || got.Rows[0].QualifiedName != "b.useLeaf" {
		t.Fatalf("a caller question lists the incoming callers, got callers_of=%v rows=%v", got.CallersOf, queryRowQNs(got.Rows))
	}
	if len(got.Bodies) != 0 {
		t.Fatalf("a caller question does not attach bodies: %+v", got.Bodies)
	}

	plain, err := store.Query(ctx, api.QueryRequest{Project: "fixture", Question: "how does leaf work"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plain.CallersOf) != 0 {
		t.Fatalf("a normal question is not a caller question: %v", plain.CallersOf)
	}

	none, err := store.Query(ctx, api.QueryRequest{Project: "fixture", Question: "who calls alone"})
	if err != nil {
		t.Fatal(err)
	}
	if len(none.CallersOf) != 1 || none.CallersOf[0] != "a.alone" || len(none.Rows) != 0 {
		t.Fatalf("zero callers is an empty page for that symbol, got callers_of=%v rows=%v", none.CallersOf, queryRowQNs(none.Rows))
	}
	if queryAsksForCallers("where is the caller id checked") {
		t.Fatal("caller id is not a caller question")
	}
}

func TestTraceIncomingLabelsCallers(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		leaf, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "leaf", QualifiedName: "a.leaf", Location: api.Location{File: "a.ts", StartLine: 1, EndLine: 3}})
		if err != nil {
			return err
		}
		caller, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "useLeaf", QualifiedName: "b.useLeaf", Location: api.Location{File: "b.ts", StartLine: 1, EndLine: 5}})
		if err != nil {
			return err
		}
		_, err = builder.PutEdge(api.Edge{Project: "fixture", SourceID: caller, TargetID: leaf, Type: "CALLS"})
		return err
	})
	defer store.Close()

	result, err := store.Trace(ctx, api.TraceRequest{Project: "fixture", Start: "leaf", Direction: "incoming", Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if result.Direction != "incoming" {
		t.Fatalf("direction=%q", result.Direction)
	}
	text := format.TraceCompact(result)
	if !strings.Contains(text, "direction: incoming") {
		t.Fatalf("compact missing incoming: %q", text)
	}
	if !strings.Contains(text, "callers[") {
		t.Fatalf("incoming compact must say callers: %q", text)
	}
	if strings.Contains(text, "callees") {
		t.Fatalf("incoming compact must not say callees: %q", text)
	}
	if !strings.Contains(text, "useLeaf") {
		t.Fatalf("expected caller useLeaf: %q", text)
	}
}

func TestSnippetPrefersFunctionOverModuleAndPathPicksFile(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	lines := make([]string, 80)
	for i := range lines {
		lines[i] = fmt.Sprintf("// line %d", i+1)
	}
	lines[19] = "export function foo() { return 1 }"
	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(root, "src", "foo.ts"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/graph.db"
	store, err := OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{
			Name: "fixture", RootPath: root, Generation: "one",
			EngineVersion: "test", IndexedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "Module", Name: "src/foo.ts", QualifiedName: "src.foo", Location: api.Location{File: "src/foo.ts", StartLine: 1, EndLine: 80}}); err != nil {
			return err
		}
		if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "File", Name: "foo.ts", QualifiedName: "src.foo.ts.__file__", Location: api.Location{File: "src/foo.ts", StartLine: 1, EndLine: 80}}); err != nil {
			return err
		}
		if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "foo", QualifiedName: "src.foo.foo", Location: api.Location{File: "src/foo.ts", StartLine: 20, EndLine: 20}}); err != nil {
			return err
		}
		return nil
	})
	if closeErr := store.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	store, err = OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	symbol, err := store.Snippet(ctx, api.SnippetRequest{Project: "fixture", QualifiedName: "foo"})
	if err != nil {
		t.Fatal(err)
	}
	if symbol.Status == "ambiguous" || symbol.Label != "Function" {
		t.Fatalf("symbol foo: status=%q label=%q qn=%q", symbol.Status, symbol.Label, symbol.QualifiedName)
	}
	if !strings.Contains(symbol.Code, "export function foo") {
		t.Fatalf("expected function body, got %q", symbol.Code)
	}

	byPath, err := store.Snippet(ctx, api.SnippetRequest{Project: "fixture", QualifiedName: "src/foo.ts"})
	if err != nil {
		t.Fatal(err)
	}
	if byPath.Label != "File" {
		t.Fatalf("path snippet label=%q qn=%q", byPath.Label, byPath.QualifiedName)
	}
	if byPath.Location.EndLine <= 1 {
		t.Fatalf("file snippet must not be L1-only: %#v", byPath.Location)
	}

	// A complete body is self-contained (AXI §9): no next-step hints.
	if help := format.HelpForSnippet(symbol); len(help) != 0 {
		t.Fatalf("complete snippet must not add hints: %v", help)
	}
}

func TestSnippetFileNodeWithMissingEndLineReadsDisk(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	lines := make([]string, 75)
	for i := range lines {
		lines[i] = fmt.Sprintf("// line %d", i+1)
	}
	lines[22] = "export function main() { return null }"
	body := strings.Join(lines, "\n") + "\n"
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "app.ts"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/graph.db"
	store, err := OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{
			Name: "fixture", RootPath: root, Generation: "one",
			EngineVersion: "test", IndexedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		_, err := builder.PutNode(api.Node{Project: "fixture", Label: "File", Name: "app.ts", QualifiedName: "src/app.ts.__file__", Location: api.Location{File: "src/app.ts", StartLine: 1, EndLine: 1}})
		return err
	})
	if closeErr := store.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	store, err = OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	got, err := store.Snippet(ctx, api.SnippetRequest{Project: "fixture", QualifiedName: "src/app.ts"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Location.EndLine <= 1 {
		t.Fatalf("stale File end_line=1 must still snippet from disk, got %#v code=%q", got.Location, got.Code)
	}
	if !strings.Contains(got.Code, "export function main") {
		t.Fatalf("expected file body, got %q", got.Code)
	}
}

func TestSnippetClipsWideFileRange(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	var b strings.Builder
	for i := 1; i <= 600; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	if err := os.WriteFile(filepath.Join(root, "big.ts"), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/graph.db"
	store, err := OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{
			Name: "fixture", RootPath: root, Generation: "one",
			EngineVersion: "test", IndexedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		_, err := builder.PutNode(api.Node{Project: "fixture", Label: "File", Name: "big.ts", QualifiedName: "big.ts.__file__", Location: api.Location{File: "big.ts", StartLine: 1, EndLine: 600}})
		return err
	})
	if closeErr := store.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	store, err = OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	got, err := store.Snippet(ctx, api.SnippetRequest{Project: "fixture", QualifiedName: "big.ts"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Clipped {
		t.Fatalf("expected clipped file snippet, lines=%d-%d code lines=%d", got.Location.StartLine, got.Location.EndLine, strings.Count(got.Code, "\n")+1)
	}
	if strings.Count(got.Code, "\n") > 500 {
		t.Fatalf("clipped snippet still too large: %d newlines", strings.Count(got.Code, "\n"))
	}
	compact := format.SnippetCompact(got)
	if !strings.Contains(compact, "code[500 of 600 lines]:") {
		t.Fatalf("compact must show shown and total lines: %q", compact[:200])
	}
	short, err := store.Snippet(ctx, api.SnippetRequest{Project: "fixture", QualifiedName: "big.ts", StartLine: 535, MaxLines: 18})
	if err != nil {
		t.Fatal(err)
	}
	if short.Location.StartLine != 535 || short.Location.EndLine != 552 || len(strings.Split(short.Code, "\n")) != 18 {
		t.Fatalf("explicit file snippet span should be 18 lines: %+v", short.Location)
	}
}

func TestSnippetClipsSymbolToMaxLines(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	var b strings.Builder
	for i := 1; i <= 400; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	if err := os.WriteFile(filepath.Join(root, "fn.go"), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/graph.db"
	store, err := OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{
			Name: "fixture", RootPath: root, Generation: "one",
			EngineVersion: "test", IndexedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		_, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Function", Name: "wide", QualifiedName: "fn.wide",
			Location: api.Location{File: "fn.go", StartLine: 1, EndLine: 400},
		})
		return err
	})
	if closeErr := store.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	store, err = OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	got, err := store.Snippet(ctx, api.SnippetRequest{Project: "fixture", QualifiedName: "fn.wide"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Clipped {
		t.Fatalf("expected clipped function snippet, lines=%d-%d", got.Location.StartLine, got.Location.EndLine)
	}
	if got.Location.EndLine-got.Location.StartLine > snippetMaxLines {
		t.Fatalf("function snippet span %d-%d wider than %d", got.Location.StartLine, got.Location.EndLine, snippetMaxLines)
	}
	compact := format.SnippetCompact(got)
	hints := strings.Join(format.HelpForSnippet(got), "\n")
	if !strings.Contains(compact, "code[250 of 400 lines]:") || !strings.Contains(hints, "so graph snippet fn.wide --from 251") {
		t.Fatalf("clipped snippet must name the omitted tail: %q / %q", compact[:120], hints)
	}
	body, err := store.Snippet(ctx, api.SnippetRequest{Project: "fixture", QualifiedName: "fn.wide", MaxLines: queryBodyMaxLines})
	if err != nil {
		t.Fatal(err)
	}
	if !body.Clipped || body.Location.EndLine-body.Location.StartLine > queryBodyMaxLines {
		t.Fatalf("query body should keep the %d-line cap, lines %d-%d", queryBodyMaxLines, body.Location.StartLine, body.Location.EndLine)
	}
	tail, err := store.Snippet(ctx, api.SnippetRequest{Project: "fixture", QualifiedName: "fn.wide", StartLine: 390})
	if err != nil {
		t.Fatal(err)
	}
	if tail.Clipped || tail.Location.StartLine != 390 || !strings.Contains(tail.Code, "line 400") {
		t.Fatalf("from-line should return the tail, lines %d-%d clipped=%v\n%s", tail.Location.StartLine, tail.Location.EndLine, tail.Clipped, tail.Code)
	}
}

func TestSnippetOneLineSymbolNotClipped(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	var b strings.Builder
	for i := 1; i <= 20; i++ {
		if i == 3 {
			fmt.Fprintf(&b, "export const FOO = 1\n")
			continue
		}
		fmt.Fprintf(&b, "// line %d\n", i)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "app.ts"), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/graph.db"
	store, err := OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{
			Name: "fixture", RootPath: root, Generation: "one",
			EngineVersion: "test", IndexedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		_, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Variable", Name: "FOO", QualifiedName: "src.app.FOO",
			Location: api.Location{File: "src/app.ts", StartLine: 3, EndLine: 3},
		})
		return err
	})
	if closeErr := store.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	store, err = OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	got, err := store.Snippet(ctx, api.SnippetRequest{Project: "fixture", QualifiedName: "src.app.FOO"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Clipped {
		t.Fatalf("one-line symbol must not be clipped: %#v code=%q", got.Location, got.Code)
	}
	if !strings.Contains(got.Code, "export const FOO") {
		t.Fatalf("expected symbol body, got %q", got.Code)
	}
}

func TestQuerySkipsDataLanguageVariablesAndTraceBothSkipsJSONKeys(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		fn, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "loadConfig", QualifiedName: "src.app.loadConfig", Location: api.Location{File: "src/app.ts", StartLine: 10, EndLine: 40}})
		if err != nil {
			return err
		}
		file, err := builder.PutNode(api.Node{Project: "fixture", Label: "File", Name: "config.json", QualifiedName: "config.json.__file__", Location: api.Location{File: "config.json", StartLine: 1, EndLine: 12}})
		if err != nil {
			return err
		}
		title, err := builder.PutNode(api.Node{Project: "fixture", Label: "Variable", Name: "title", QualifiedName: "config.json.title", Location: api.Location{File: "config.json", StartLine: 2}})
		if err != nil {
			return err
		}
		foo, err := builder.PutNode(api.Node{Project: "fixture", Label: "Variable", Name: "FOO", QualifiedName: "src.app.FOO", Location: api.Location{File: "src/app.ts", StartLine: 3, EndLine: 8}})
		if err != nil {
			return err
		}
		if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: file, TargetID: title, Type: "DEFINES"}); err != nil {
			return err
		}
		if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: fn, TargetID: title, Type: "DEFINES"}); err != nil {
			return err
		}
		_, err = builder.PutEdge(api.Edge{Project: "fixture", SourceID: fn, TargetID: foo, Type: "DEFINES"})
		return err
	})
	defer store.Close()

	result, err := store.Query(ctx, api.QueryRequest{Project: "fixture", Question: "how does config load", Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range result.Rows {
		if row.Label == "Variable" && row.Location.File == "config.json" {
			t.Fatalf("query listed a JSON key: %v", queryRowQNs(result.Rows))
		}
	}

	traced, err := store.Trace(ctx, api.TraceRequest{Project: "fixture", Start: "loadConfig", Direction: "both", Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	compact := format.TraceCompact(traced)
	if strings.Contains(compact, "title") {
		t.Fatalf("trace both listed JSON key as neighbor: %q", compact)
	}
}

func TestSearchFindsExportedSourceConstNotJSONKey(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "Variable", Name: "FOO", QualifiedName: "src.app.FOO", Location: api.Location{File: "src/app.ts", StartLine: 3, EndLine: 8}}); err != nil {
			return err
		}
		if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "Variable", Name: "title", QualifiedName: "config.json.title", Location: api.Location{File: "config.json", StartLine: 2}}); err != nil {
			return err
		}
		return nil
	})
	defer store.Close()

	foo, err := store.Search(ctx, api.SearchRequest{Project: "fixture", Query: "FOO", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range foo.Matches {
		if m.QualifiedName == "src.app.FOO" {
			found = true
		}
		if m.Location.File == "config.json" {
			t.Fatalf("JSON variable leaked into FTS: %#v", m)
		}
	}
	if !found {
		t.Fatalf("exported source const FOO not searchable: %#v", foo.Matches)
	}
	title, err := store.Search(ctx, api.SearchRequest{Project: "fixture", Query: "title", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range title.Matches {
		if m.QualifiedName == "config.json.title" || (m.Label == "Variable" && m.Location.File == "config.json") {
			t.Fatalf("JSON key title was an FTS hit: %#v", m)
		}
	}
}

func TestQueryAnswerRowsDropsRowsWithoutSourceLocation(t *testing.T) {
	rows := queryAnswerRows(nil, []seedCandidate{
		{node: api.Node{ID: 1, Label: "Route", Name: "/parse_encoded_text/", QualifiedName: "__route__ANY__/parse_encoded_text/"}},
		{node: api.Node{ID: 2, Label: "Function", Name: "f", QualifiedName: "pkg.f", Location: api.Location{File: "pkg/f.py", StartLine: 3}}},
	}, nil, "how does f work", []string{"work"})
	if len(rows) != 1 || rows[0].QualifiedName != "pkg.f" {
		t.Fatalf("rows=%v", rows)
	}
}

func TestQueryAnswerRowsPutsVendoredFilesLast(t *testing.T) {
	js := api.Node{ID: 1, Label: "Function", Name: "property", QualifiedName: "themes.static.underscore.property", Location: api.Location{File: "sphinx/themes/basic/static/underscore.js", StartLine: 1}}
	py := api.Node{ID: 2, Label: "Function", Name: "isproperty", QualifiedName: "sphinx.util.inspect.isproperty", Location: api.Location{File: "sphinx/util/inspect.py", StartLine: 386}}
	rows := queryAnswerRows(nil, []seedCandidate{{node: js}, {node: py}}, nil, "where is property detected", []string{"property"})
	if len(rows) != 2 || rows[0].QualifiedName != py.QualifiedName {
		t.Fatalf("vendored row must sort last: %v", rows)
	}
	rows = queryAnswerRows(nil, []seedCandidate{{node: js}, {node: py}}, nil, "how does the static underscore helper work", []string{"static", "underscore"})
	if rows[0].QualifiedName != js.QualifiedName {
		t.Fatalf("a question that names static keeps it in place: %v", rows)
	}
}

func TestQueryAnswerRowsSkipsExpansionOnlyModules(t *testing.T) {
	mod := api.Node{ID: 1, Label: "Module", Name: "cpp", QualifiedName: "sphinx.domains.cpp", Location: api.Location{File: "sphinx/domains/cpp.py", StartLine: 1, EndLine: 7901}}
	fn := api.Node{ID: 2, Label: "Function", Name: "f", QualifiedName: "pkg.f", Location: api.Location{File: "pkg/f.py", StartLine: 3}}
	seedMod := api.Node{ID: 3, Label: "Module", Name: "inspect", QualifiedName: "sphinx.util.inspect", Location: api.Location{File: "sphinx/util/inspect.py", StartLine: 1, EndLine: 865}}
	ordered := []queryNodeHit{{node: mod}, {node: fn}, {node: seedMod, seed: true}}
	rows := queryAnswerRows(nil, nil, ordered, "how does f work", []string{"work"})
	got := []string{}
	for _, r := range rows {
		got = append(got, r.QualifiedName)
	}
	if len(got) != 2 || got[0] != "pkg.f" || got[1] != "sphinx.util.inspect" {
		t.Fatalf("expansion-only Module must drop, seeded Module must stay: %v", got)
	}
}

func TestQuerySeedPoolSetsTestsAsideBeforeTheCut(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/graph.db"
	store, err := OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{Name: "fixture", RootPath: t.TempDir(), Generation: "one", EngineVersion: "test", IndexedAt: time.Now().UTC()}); err != nil {
			return err
		}
		// 60 tests whose long names match every generic word in the question,
		// and one real function that matches only the rare one.
		for i := 0; i < 60; i++ {
			name := fmt.Sprintf("test_ordering_not_other_filter_annotation_%d", i)
			if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: name, QualifiedName: "tests.q." + name,
				Location: api.Location{File: "tests/queries/tests.py", StartLine: i + 1}}); err != nil {
				return err
			}
		}
		for i := 0; i < 12; i++ {
			name := "get_aggregation"
			if i > 0 {
				name = fmt.Sprintf("aggregation_helper_%d", i)
			}
			if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "Method", Name: name, QualifiedName: "pkg.query.Query." + name,
				Location: api.Location{File: "pkg/query.py", StartLine: 10 + i*20, EndLine: 28 + i*20}}); err != nil {
				return err
			}
		}
		return nil
	})
	if closeErr := store.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	store, err = OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	q := "drop aggregation that is not referenced by other ordering filter annotation"
	cands, _, err := store.querySeedCandidates(ctx, "fixture", q, queryTerms(q, nil))
	if err != nil {
		t.Fatal(err)
	}
	var real bool
	for _, c := range cands {
		if c.node.Name == "get_aggregation" {
			real = true
		}
		if queryNodeLooksLikeTest(c.node) {
			t.Fatalf("tests must be set aside when the question does not ask for them: %s", c.node.Name)
		}
	}
	if !real {
		t.Fatalf("the real function must survive the cut, got %d candidates", len(cands))
	}
	asked := "test ordering not other filter annotation"
	cands, _, err = store.querySeedCandidates(ctx, "fixture", asked, queryTerms(asked, nil))
	if err != nil {
		t.Fatal(err)
	}
	tests := 0
	for _, c := range cands {
		if queryNodeLooksLikeTest(c.node) {
			tests++
		}
	}
	if tests == 0 {
		t.Fatal("a question that mentions tests must keep them")
	}
}
