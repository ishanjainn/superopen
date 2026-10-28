package engine

import (
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestAppendTestRelationshipsMatchesPinnedNamingAndCallRules(t *testing.T) {
	graph := goGraph{nodes: []api.Node{
		{Label: "File", Name: "store.go", QualifiedName: "store.go.__file__", Location: api.Location{File: "store.go"}},
		{Label: "File", Name: "store_test.go", QualifiedName: "store_test.go.__file__", Location: api.Location{File: "store_test.go"}},
		{Label: "Function", Name: "Run", QualifiedName: "store.Run", Location: api.Location{File: "store.go"}},
		{Label: "Function", Name: "TestRun", QualifiedName: "store_test.TestRun", Location: api.Location{File: "store_test.go"}},
		{Label: "Function", Name: "helper", QualifiedName: "store_test.helper", Location: api.Location{File: "store_test.go"}},
	}, edges: []pendingEdge{
		{source: "store_test.TestRun", target: "store.Run", kind: "CALLS"},
		{source: "store_test.helper", target: "store.Run", kind: "CALLS"},
	}}
	appendTestRelationships(&graph)
	want := map[string]bool{
		"store_test.TestRun\x00TESTS\x00store.Run":                  false,
		"store_test.go.__file__\x00TESTS_FILE\x00store.go.__file__": false,
	}
	for _, edge := range graph.edges {
		key := edge.source + "\x00" + edge.kind + "\x00" + edge.target
		if _, ok := want[key]; ok {
			want[key] = true
		}
		if edge.kind == "TESTS" && edge.source == "store_test.helper" {
			t.Fatalf("helper was incorrectly classified as a test: %#v", edge)
		}
	}
	for key, found := range want {
		if !found {
			t.Errorf("missing %q", key)
		}
	}
}

func TestAppendTestRelationshipsAttributesNestedPythonHelper(t *testing.T) {
	graph := goGraph{nodes: []api.Node{
		{Label: "Function", Name: "run", QualifiedName: "app.run", Location: api.Location{File: "app.py"}},
		{Label: "Function", Name: "test_run", QualifiedName: "tests.test_app.test_run", Location: api.Location{File: "tests/test_app.py"}},
		{Label: "Function", Name: "invoke", QualifiedName: "tests.test_app.test_run.invoke", Location: api.Location{File: "tests/test_app.py"}},
		{Label: "Function", Name: "helper", QualifiedName: "tests.test_app.helper", Location: api.Location{File: "tests/test_app.py"}},
	}, edges: []pendingEdge{
		{source: "tests.test_app.test_run.invoke", target: "app.run", kind: "CALLS"},
		{source: "tests.test_app.helper", target: "app.run", kind: "CALLS"},
	}}
	appendTestRelationships(&graph)
	if !hasPendingEdge(graph.edges, "tests.test_app.test_run", "TESTS", "app.run") ||
		hasPendingEdge(graph.edges, "tests.test_app.helper", "TESTS", "app.run") {
		t.Fatalf("test-local helper attribution is wrong: %#v", graph.edges)
	}
}
