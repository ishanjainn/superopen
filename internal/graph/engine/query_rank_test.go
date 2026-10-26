package engine

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestRankTokensSplitsIdentifiersAndStems(t *testing.T) {
	got := rankTokens("HTTPServer get_choices existing_annotations Base64Encoder decorated")
	want := []string{"http", "server", "get", "choice", "exist", "annotation", "base", "64", "encoder", "decorat"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestQueryRankTermsDropQuestionWords(t *testing.T) {
	got := queryRankTerms("How does the count() use annotations when they are not used?")
	want := []string{"count", "annotation"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestQueryRankIdentsReadCodeSpelling(t *testing.T) {
	got := queryRankIdents("Ordering in admin.RelatedFieldListFilter; queryset.count() and get_choices break")
	for _, want := range []string{"relatedfieldlistfilter", "count", "get_choices"} {
		if !got[want] {
			t.Fatalf("missing %q in %v", want, got)
		}
	}
	for _, prose := range []string{"ordering", "admin", "break"} {
		if got[prose] {
			t.Fatalf("prose word %q read as an identifier: %v", prose, got)
		}
	}
}

func rankFixtureDoc(name, qn, file, doc, body string) rankDoc {
	return newRankDoc(api.Node{Label: "Method", Name: name, QualifiedName: qn, Location: api.Location{File: file},
		Properties: api.Properties{"docstring": doc, "bt": body}})
}

// One exact common word in a name must not beat a body that does what the
// question describes.
func TestScoreRankDocsPrefersCoverageOverOneExactName(t *testing.T) {
	pool := []rankDoc{
		rankFixtureDoc("count", "pkg.paginator.Paginator.count", "pkg/paginator.py", "Return the total number of objects.", "len object_list"),
		rankFixtureDoc("count", "pkg.geo.ListMixin.count", "pkg/geo/list.py", "", "val items"),
		rankFixtureDoc("get_count", "pkg.sql.query.Query.get_count", "pkg/sql/query.py", "Perform a COUNT() query.",
			"obj clone add_annotation Count get_aggregation existing_annotations"),
	}
	q := "Strip unused annotations from count queries"
	got := scoreRankDocs(pool, queryRankTerms(q), map[string]int{"strip": 5, "unus": 2, "annotation": 40, "count": 300, "query": 400}, 10000, queryRankIdents(q), q)
	if len(got) == 0 || got[0].Name != "get_count" {
		t.Fatalf("want get_count first, got %v", rankNames(got))
	}
}

func TestScoreRankDocsBoostsNamedIdentifier(t *testing.T) {
	pool := []rankDoc{
		rankFixtureDoc("field_choices", "pkg.filters.RelatedFieldListFilter.field_choices", "pkg/filters.py", "", "field ordering get_choices"),
		rankFixtureDoc("get_choices", "pkg.fields.Field.get_choices", "pkg/fields.py", "Return choices with a default blank choice.", "ordering limit_choices_to"),
	}
	q := "How does get_choices use ordering?"
	got := scoreRankDocs(pool, queryRankTerms(q), map[string]int{"choices": 30, "ordering": 30}, 1000, queryRankIdents(q), q)
	if len(got) != 2 || got[0].Name != "get_choices" {
		t.Fatalf("the named identifier must rank first, got %v", rankNames(got))
	}
}

func TestScoreRankDocsSinksVendoredTrees(t *testing.T) {
	pool := []rankDoc{
		rankFixtureDoc("highlight", "static.jquery.highlight", "pkg/static/jquery.js", "", "highlight text"),
		rankFixtureDoc("highlight_block", "pkg.highlighting.highlight_block", "pkg/highlighting.py", "", "lexer"),
	}
	q := "highlight text"
	got := scoreRankDocs(pool, queryRankTerms(q), map[string]int{"highlight": 5, "text": 50}, 1000, queryRankIdents(q), q)
	if got[len(got)-1].Name != "highlight" {
		t.Fatalf("vendored code must sort last, got %v", rankNames(got))
	}
}

func rankNames(nodes []api.Node) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.QualifiedName
	}
	return out
}

// A question in behavior words must reach a symbol whose name has none of
// them, through its docstring and body, in the stored index.
func TestQueryRowsReachSymbolsByBodyAndDocstring(t *testing.T) {
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
		nodes := []api.Node{
			{Label: "Function", Name: "sanitize_address", QualifiedName: "mail.message.sanitize_address",
				Location:   api.Location{File: "mail/message.py", StartLine: 74, EndLine: 111},
				Properties: api.Properties{"docstring": "Format a pair of (name, address).", "bt": "domain encoding encode UnicodeEncodeError idna"}},
			{Label: "Method", Name: "encoding", QualifiedName: "http.request.HttpRequest.encoding",
				Location: api.Location{File: "http/request.py", StartLine: 250, EndLine: 260}},
			{Label: "Method", Name: "encoding", QualifiedName: "gis.feature.Feature.encoding",
				Location: api.Location{File: "gis/feature.py", StartLine: 58, EndLine: 59}},
		}
		for _, n := range nodes {
			n.Project = "fixture"
			if _, err := builder.PutNode(n); err != nil {
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
	result, err := store.Query(ctx, api.QueryRequest{Project: "fixture", Question: "Email crashes on non-ASCII domain when encoding is non-unicode"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) == 0 || result.Rows[0].Name != "sanitize_address" {
		t.Fatalf("want sanitize_address first, got %v", rankNames(result.Rows))
	}
	// Name search stays a name search: body words do not list it.
	search, err := store.Search(ctx, api.SearchRequest{Project: "fixture", Query: "idna"})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range search.Matches {
		if strings.Contains(m.QualifiedName, "sanitize_address") {
			t.Fatalf("search matched a body word: %v", m.QualifiedName)
		}
	}
}
