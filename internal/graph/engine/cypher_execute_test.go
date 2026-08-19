package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestCypherExecutorPatternsFiltersProjectionAndAggregate(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "graph.db")
	buildFixture(t, ctx, path, "cypher-generation")
	store, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	result, err := store.executeCypher(ctx, api.CypherRequest{Project: "fixture", Query: `MATCH (f:Function)-[:CALLS]->(g:Function) WHERE f.name STARTS WITH "Refresh" RETURN f.name, g.name`})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 || result.Rows[0]["f.name"] != "RefreshAtomicWithOptions" || result.Rows[0]["g.name"] != "BuildWithOptions" {
		t.Fatalf("rows = %#v", result.Rows)
	}

	result, err = store.executeCypher(ctx, api.CypherRequest{Project: "fixture", Query: `MATCH (f:Function) RETURN f.label, COUNT(f) AS cnt ORDER BY cnt DESC`})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 || result.Rows[0]["cnt"] != int64(2) {
		t.Fatalf("aggregate rows = %#v", result.Rows)
	}
}

func TestCypherExecutorOptionalUnionUnwindAndParameters(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "graph.db")
	buildFixture(t, ctx, path, "cypher-generation")
	store, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	result, err := store.executeCypher(ctx, api.CypherRequest{
		Project: "fixture",
		Query:   `MATCH (f:Function {name: $name}) OPTIONAL MATCH (f)-[:MISSING]->(g) RETURN f.name, g.name`,
		Params:  map[string]any{"name": "BuildWithOptions"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 || result.Rows[0]["g.name"] != nil {
		t.Fatalf("optional rows = %#v", result.Rows)
	}
	result, err = store.executeCypher(ctx, api.CypherRequest{Project: "fixture", Query: `MATCH (f:Function) WHERE EXISTS { (f)-[:CALLS]->() } RETURN f.name`})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 || result.Rows[0]["f.name"] != "RefreshAtomicWithOptions" {
		t.Fatalf("EXISTS rows = %#v", result.Rows)
	}

	result, err = store.executeCypher(ctx, api.CypherRequest{Project: "fixture", Query: `UNWIND ["RefreshAtomicWithOptions"] AS wanted MATCH (f:Function) WHERE f.name = wanted RETURN f.name UNION MATCH (f:Function {name: "RefreshAtomicWithOptions"}) RETURN f.name`})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("UNION did not deduplicate: %#v", result.Rows)
	}
}

func TestCypherExecutorRespectsCancellationAndRowCeiling(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "graph.db")
	buildFixture(t, ctx, path, "cypher-generation")
	store, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.executeCypher(cancelled, api.CypherRequest{Project: "fixture", Query: `MATCH (f) RETURN f`}); err == nil {
		t.Fatal("cancelled query unexpectedly succeeded")
	}
	result, err := store.executeCypher(ctx, api.CypherRequest{Project: "fixture", Query: `MATCH (f) RETURN f`, MaxRows: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 || !result.Page.Truncated || result.Page.Total != 2 {
		t.Fatalf("ceiling result = %#v", result)
	}
	result, err = store.executeCypher(ctx, api.CypherRequest{Project: "fixture", Query: `MATCH (f) RETURN f LIMIT 2`, MaxRows: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 2 || result.Page.Limit != 2 {
		t.Fatalf("explicit LIMIT must override max_rows: %#v", result)
	}
}

func TestCypherExecutorUpstreamExpressionSurface(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "graph.db")
	buildFixture(t, ctx, path, "cypher-generation")
	store, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	tests := []struct {
		query string
		rows  int
	}{
		{`MATCH (f:Function) WHERE f.name CONTAINS "Options" RETURN f.name`, 2},
		{`MATCH (f:Function) WHERE f.name ENDS WITH "Options" RETURN f.name`, 2},
		{`MATCH (f:Function) WHERE f.name NOT IN ["BuildWithOptions"] RETURN f.name`, 1},
		{`MATCH (f:Function) WHERE f:Function AND f.missing IS NULL RETURN f.name`, 2},
		{`MATCH (f:Function) WHERE f.file_path IS NOT NULL RETURN f.name`, 2},
		{`MATCH (f:Function) RETURN DISTINCT f.label`, 1},
		{`MATCH (f:Function) WITH DISTINCT f.label AS label RETURN label`, 1},
		{`MATCH (f:Function) WHERE coalesce(f.missing, "fallback") = "fallback" RETURN f.name`, 2},
		{`MATCH (f:Function) WHERE substring(f.name, 0, 5) = "Build" RETURN f.name`, 1},
		{`MATCH (f:Function) WHERE NOT f.name = "BuildWithOptions" RETURN f.name`, 1},
		{`MATCH (f:Function) WHERE f.in_degree = 0 AND f.out_degree = 1 RETURN f.name`, 1},
	}
	for _, test := range tests {
		result, err := store.executeCypher(ctx, api.CypherRequest{Project: "fixture", Query: test.query})
		if err != nil {
			t.Errorf("%s: %v", test.query, err)
			continue
		}
		if len(result.Rows) != test.rows {
			t.Errorf("%s: rows=%#v, want %d", test.query, result.Rows, test.rows)
		}
	}

	result, err := store.executeCypher(ctx, api.CypherRequest{Project: "fixture", Query: `MATCH (f:Function {name: "BuildWithOptions"}) RETURN size(f.name), length(f.name), left(f.name, 5), right(f.name, 7), replace(f.name, "Build", "Make"), reverse(f.name)`})
	if err != nil {
		t.Fatal(err)
	}
	row := result.Rows[0]
	if row["size"] != int64(16) || row["length"] != int64(16) || row["left"] != "Build" || row["right"] != "Options" || row["replace"] != "MakeWithOptions" || row["reverse"] != "snoitpOhtiWdliuB" {
		t.Fatalf("function row = %#v", row)
	}

	result, err = store.executeCypher(ctx, api.CypherRequest{Project: "fixture", Query: `MATCH (f:Function) RETURN * LIMIT 1`})
	if err != nil {
		t.Fatal(err)
	}
	wantColumns := []string{"f.name", "f.qualified_name", "f.label", "f.file_path"}
	if !sameStrings(result.Columns, wantColumns) || len(result.Rows) != 1 || result.Rows[0]["f.label"] != "Function" {
		t.Fatalf("RETURN * = columns %#v rows %#v", result.Columns, result.Rows)
	}
	result, err = store.executeCypher(ctx, api.CypherRequest{Project: "fixture", Query: `MATCH (f:Function)`})
	if err != nil {
		t.Fatal(err)
	}
	wantColumns = []string{"f.name", "f.qualified_name", "f.label"}
	if !sameStrings(result.Columns, wantColumns) || len(result.Rows) != 2 {
		t.Fatalf("default projection = columns %#v rows %#v", result.Columns, result.Rows)
	}
}

func TestCypherExecutorMissedGraph(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "graph.db")
	buildFixture(t, ctx, path, "cypher-generation")
	writable, err := OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	err = writable.Build(ctx, func(builder *Builder) error {
		return builder.PutCoverage("fixture", api.Coverage{
			Generation: "cypher-generation", IndexMode: "full", RecordingStatus: "complete",
			Rows: []api.CoverageRow{
				{Path: "internal/broken.go", Kind: "parse_partial", Detail: "ERROR node at line 9"},
				{Path: "vendor/ignored.go", Kind: "not_indexed_ignored", Detail: "gitignore"},
			},
		})
	})
	if closeErr := writable.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	result, err := store.executeCypher(ctx, api.CypherRequest{Project: "fixture", Graph: "missed",
		Query: `MATCH (f:File) RETURN f.name, f.kind, f.detail`})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 || result.Rows[0]["f.name"] != "broken.go" ||
		result.Rows[0]["f.kind"] != "parse_partial" || result.Rows[0]["f.detail"] != "ERROR node at line 9" {
		t.Fatalf("missed graph rows = %#v", result.Rows)
	}
	projects, err := store.Projects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Name != "fixture" {
		t.Fatalf("missed shadow leaked through project listing: %#v", projects)
	}
	if _, err := store.executeCypher(ctx, api.CypherRequest{Project: "fixture", Graph: "other", Query: `MATCH (n) RETURN n`}); err == nil {
		t.Fatal("invalid graph unexpectedly succeeded")
	}
}

func TestCypherAggregateOverEmptyMatch(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "graph.db")
	buildFixture(t, ctx, path, "cypher-generation")
	store, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	result, err := store.executeCypher(ctx, api.CypherRequest{Project: "fixture", Query: `MATCH (n:Missing) RETURN count(*) AS n`})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 || result.Rows[0]["n"] != int64(0) {
		t.Fatalf("empty aggregate = %#v", result.Rows)
	}
}
