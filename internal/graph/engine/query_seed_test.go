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

func seedNames(seeds []api.RankedNode) []string {
	out := make([]string, 0, len(seeds))
	for _, s := range seeds {
		out = append(out, s.Name)
	}
	return out
}
