package engine

import (
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestScoreQuerySeedsPrefersCamelCaseClassOverLowerMethod(t *testing.T) {
	class := seedCandidate{node: api.Node{
		Label: "Class", Name: "QuerySet", QualifiedName: "django.db.models.query.QuerySet",
		Location: api.Location{File: "django/db/models/query.py"},
	}}
	method := seedCandidate{node: api.Node{
		Label: "Method", Name: "queryset", QualifiedName: "django.contrib.admin.filters.FieldListFilter.queryset",
		Location: api.Location{File: "django/contrib/admin/filters.py"},
	}, degree: 80}
	q := "How does the QuerySet class build and evaluate its query"
	got := scoreQuerySeeds([]seedCandidate{class, method}, queryTerms(q, nil), q)
	if len(got.ranked) == 0 || !strings.EqualFold(got.ranked[0].Name, "QuerySet") || got.ranked[0].Label != "Class" {
		t.Fatalf("CamelCase class should rank first, got %q %q", got.ranked[0].Label, got.ranked[0].Name)
	}
	for _, seed := range got.seeds {
		if seed.Name == "queryset" && seed.Label == "Method" {
			t.Fatalf("lowercased method must not seed over QuerySet class, seeds=%v", seedNames(got.seeds))
		}
	}
}

func TestScoreQuerySeedsPrefersNamedClassOverTestSubstring(t *testing.T) {
	class := seedCandidate{node: api.Node{
		Label: "Class", Name: "QuerySet", QualifiedName: "django.db.models.query.QuerySet",
		Location: api.Location{File: "django/db/models/query.py"},
	}}
	testFn := seedCandidate{node: api.Node{
		Label: "Function", Name: "test_doesnotexist_class",
		QualifiedName: "tests.queryset_pickle.tests.PickleabilityTestCase.test_doesnotexist_class",
		Location:      api.Location{File: "tests/queryset_pickle/tests.py"},
	}, degree: 40}
	q := "How does the QuerySet class build and evaluate its query"
	got := scoreQuerySeeds([]seedCandidate{class, testFn}, queryTerms(q, nil), q)
	if len(got.ranked) == 0 {
		t.Fatal("expected ranked seeds")
	}
	if !strings.EqualFold(got.ranked[0].Name, "QuerySet") {
		t.Fatalf("named class should rank first, got %q", got.ranked[0].Name)
	}
	for _, seed := range got.seeds {
		if strings.Contains(strings.ToLower(seed.Name), "test_doesnotexist") {
			t.Fatalf("substring 'class' must not seed a test function, seeds=%v", seedNames(got.seeds))
		}
	}
}

func TestQueryProperNamesExtractsCamelCase(t *testing.T) {
	got := queryProperNames("How does the QuerySet class build and evaluate its query, including the SQL compiler?")
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "QuerySet") {
		t.Fatalf("QuerySet missing: %v", got)
	}
	if !strings.Contains(joined, "SQL") {
		t.Fatalf("SQL missing: %v", got)
	}
}

func TestIsSyntheticQueryFile(t *testing.T) {
	if !isSyntheticQueryFile("<python-builtins>") {
		t.Fatal("python builtins sentinel")
	}
	if isSyntheticQueryFile("django/db/models/query.py") {
		t.Fatal("real source must not look synthetic")
	}
}

func TestScoreQuerySeedsIgnoresEnglishVerbPrefix(t *testing.T) {
	class := seedCandidate{node: api.Node{
		Label: "Class", Name: "QuerySet", QualifiedName: "app.QuerySet",
		Location: api.Location{File: "app/query.py"},
	}}
	building := seedCandidate{node: api.Node{
		Label: "Class", Name: "Building", QualifiedName: "tests.models.Building",
		Location: api.Location{File: "tests/models.py"},
	}}
	evalClass := seedCandidate{node: api.Node{
		Label: "Class", Name: "Evaluation", QualifiedName: "tests.models.Evaluation",
		Location: api.Location{File: "tests/models.py"},
	}}
	q := "How does the QuerySet class build and evaluate its query"
	got := scoreQuerySeeds([]seedCandidate{class, building, evalClass}, queryTerms(q, nil), q)
	for _, seed := range got.seeds {
		if seed.Name == "Building" || seed.Name == "Evaluation" {
			t.Fatalf("English verbs must not prefix-seed classes, seeds=%v", seedNames(got.seeds))
		}
	}
	if len(got.seeds) == 0 || got.seeds[0].Name != "QuerySet" {
		t.Fatalf("QuerySet should seed, got %v", seedNames(got.seeds))
	}
}

func TestScoreQuerySeedsDemotesPropertyNameCollision(t *testing.T) {
	prop := seedCandidate{node: api.Node{
		Label: "Property", Name: "django", QualifiedName: "gis.OGRGeomType.django",
		Location: api.Location{File: "gis/ogr.py"},
	}}
	mod := seedCandidate{node: api.Node{
		Label: "Module", Name: "django", QualifiedName: "django",
		Location: api.Location{File: "django/__init__.py"},
	}}
	q := "How does django middleware work"
	got := scoreQuerySeeds([]seedCandidate{prop, mod}, queryTerms(q, nil), q)
	for _, seed := range got.seeds {
		if seed.Label == "Property" && seed.Name == "django" {
			t.Fatalf("bare 'django' must not seed a Property, seeds=%v", seedNames(got.seeds))
		}
	}
}

func TestScoreQuerySeedsSkipsDecoratorAndPrefersProjectPrefix(t *testing.T) {
	deco := seedCandidate{node: api.Node{
		Label: "Decorator", Name: "classmethod", QualifiedName: "<decorator:classmethod>",
	}}
	js := seedCandidate{node: api.Node{
		Label: "Function", Name: "property",
		QualifiedName: "sphinx.themes.basic.static.underscore-1.13.1.property",
		Location:      api.Location{File: "sphinx/themes/basic/static/underscore-1.13.1.js"},
	}, degree: 40}
	doc := seedCandidate{node: api.Node{
		Label: "Class", Name: "PropertyDocumenter",
		QualifiedName: "sphinx.ext.autodoc.PropertyDocumenter",
		Location:      api.Location{File: "sphinx/ext/autodoc/__init__.py"},
	}}
	q := "classmethod property documentation autodoc"
	got := scoreQuerySeeds([]seedCandidate{deco, js, doc}, queryTerms(q, nil), q)
	if len(got.seeds) == 0 || got.seeds[0].Name != "PropertyDocumenter" {
		t.Fatalf("project prefix should lead, seeds=%v", seedNames(got.seeds))
	}
	for _, seed := range got.seeds {
		if strings.HasPrefix(seed.QualifiedName, "<") || strings.Contains(seed.Location.File, "/static/") {
			t.Fatalf("synthetic decorator and vendored property must not seed, seeds=%v", seedNames(got.seeds))
		}
	}
}

func TestScoreQuerySeedsClaimsDirectorySegment(t *testing.T) {
	exact := seedCandidate{node: api.Node{
		Label: "Function", Name: "classmethod", QualifiedName: "pkg.classmethod",
		Location: api.Location{File: "pkg/other.py"},
	}}
	helper := seedCandidate{node: api.Node{
		Label: "Function", Name: "setup", QualifiedName: "sphinx.ext.autodoc.setup",
		Location: api.Location{File: "sphinx/ext/autodoc/__init__.py"},
	}}
	q := "classmethod property documentation autodoc"
	got := scoreQuerySeeds([]seedCandidate{exact, helper}, queryTerms(q, nil), q)
	found := false
	for _, seed := range got.seeds {
		if strings.Contains(seed.Location.File, "/autodoc/") {
			found = true
		}
		if seed.Name == "test_doesnotexist_class" {
			t.Fatal("path claim must be a whole segment")
		}
	}
	if !found {
		t.Fatalf("directory autodoc should claim a seed, seeds=%v", seedNames(got.seeds))
	}
	if queryTermEqualsPathSegment("tests/test_class.py", "class") || queryTermEqualsPathSegment("classic/foo.py", "class") {
		t.Fatal("class must not match a longer path segment")
	}
	if !queryTermEqualsPathSegment("sphinx/ext/autodoc/__init__.py", "autodoc") {
		t.Fatal("autodoc should match the directory segment")
	}
}

func TestScoreQuerySeedsKeepsVendoredWhenQuestionNamesIt(t *testing.T) {
	js := seedCandidate{node: api.Node{
		Label: "Function", Name: "property",
		QualifiedName: "themes.property",
		Location:      api.Location{File: "themes/static/underscore.js"},
	}}
	q := "How does the property function in static underscore work"
	got := scoreQuerySeeds([]seedCandidate{js}, queryTerms(q, nil), q)
	if len(got.seeds) == 0 || got.seeds[0].Name != "property" {
		t.Fatalf("naming the static directory should keep that file, seeds=%v", seedNames(got.seeds))
	}
}

func TestScoreQuerySeedsSkipsTestDirectoryClaim(t *testing.T) {
	count := seedCandidate{node: api.Node{
		Label: "Class", Name: "Count", QualifiedName: "django.db.models.aggregates.Count",
		Location: api.Location{File: "django/db/models/aggregates.py"},
	}}
	testFn := seedCandidate{node: api.Node{
		Label: "Method", Name: "test_q_annotation",
		QualifiedName: "tests.queries.test_query.TestQueryNoModel.test_q_annotation",
		Location:      api.Location{File: "tests/queries/test_query.py"},
	}}
	q := "where is Count annotation stripped for count queries"
	got := scoreQuerySeeds([]seedCandidate{count, testFn}, queryTerms(q, nil), q)
	if len(got.seeds) == 0 || got.seeds[0].Name != "Count" {
		t.Fatalf("Count should lead, seeds=%v", seedNames(got.seeds))
	}
	for _, seed := range got.seeds {
		if seed.Name == "test_q_annotation" {
			t.Fatalf("a queries directory must not seed a test, seeds=%v", seedNames(got.seeds))
		}
	}
}

func TestScoreQuerySeedsKeepsTestWhenQuestionSaysPytest(t *testing.T) {
	testFn := seedCandidate{node: api.Node{
		Label: "Function", Name: "test_q_annotation",
		QualifiedName: "tests.queries.test_query.TestQueryNoModel.test_q_annotation",
		Location:      api.Location{File: "tests/queries/test_query.py"},
	}}
	q := "how does pytest run test_q_annotation"
	got := scoreQuerySeeds([]seedCandidate{testFn}, queryTerms(q, nil), q)
	for _, seed := range got.seeds {
		if seed.Name == "test_q_annotation" {
			return
		}
	}
	t.Fatalf("a pytest question should seed the test, seeds=%v", seedNames(got.seeds))
}

func TestOmitUnmentionedTestsDropsTestFromScreen(t *testing.T) {
	count := queryNodeHit{node: api.Node{Name: "Count", Location: api.Location{File: "django/db/models/aggregates.py"}}, seed: true}
	testFn := queryNodeHit{node: api.Node{Name: "test_q_annotation", Location: api.Location{File: "tests/queries/test_query.py"}}, seed: true}
	q := "where is Count annotation stripped for count queries"
	got := omitUnmentionedTests([]queryNodeHit{count, testFn}, q, queryTerms(q, nil))
	if len(got) != 1 || got[0].node.Name != "Count" {
		t.Fatalf("test must leave the first screen, got %d", len(got))
	}
	pytest := "how does pytest run test_q_annotation"
	kept := omitUnmentionedTests([]queryNodeHit{count, testFn}, pytest, queryTerms(pytest, nil))
	if len(kept) != 2 {
		t.Fatalf("pytest question should keep the test on screen, got %d", len(kept))
	}
}

func seedNames(seeds []api.RankedNode) []string {
	out := make([]string, 0, len(seeds))
	for _, s := range seeds {
		out = append(out, s.Name)
	}
	return out
}
