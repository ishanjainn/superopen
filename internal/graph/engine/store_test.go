package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/paths"
)

func TestStoreRoundTripAndSearch(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "graph.db")
	buildFixture(t, ctx, path, "generation-one")

	store, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	status, err := store.Status(ctx, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "ready" || status.NodeCount != 2 || status.EdgeCount != 1 || status.FileCount != 1 {
		t.Fatalf("unexpected status: %+v", status)
	}
	result, err := store.Search(ctx, api.SearchRequest{Project: "fixture", Query: "refresh atomic", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) == 0 || result.Matches[0].QualifiedName != "graph.RefreshAtomicWithOptions" {
		t.Fatalf("unexpected search result: %+v", result)
	}
	projects, err := store.Projects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Name != "fixture" {
		t.Fatalf("unexpected projects: %+v", projects)
	}
	coverage, err := store.Coverage(ctx, api.CoverageRequest{
		Project: "fixture", Paths: []string{"internal/graph/graph.go", "missing.go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(coverage.Rows) != 2 || coverage.Rows[0].Kind != "indexed_no_recorded_gap" || coverage.Rows[0].Freshness != "fresh" || coverage.Rows[1].Kind != "not_indexed" || coverage.Rows[1].Freshness != "deleted" {
		t.Fatalf("unexpected exact coverage: %+v", coverage)
	}
}

func TestSearchPaginationIsExactAndCursorBound(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "graph.db")
	buildFixture(t, ctx, path, "generation-one")
	store, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first, err := store.Search(ctx, api.SearchRequest{Project: "fixture", Query: "graph", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if first.Page.Total != 2 || !first.Page.Truncated || first.Page.NextCursor == "" || len(first.Matches) != 1 {
		t.Fatalf("unexpected first page: %+v", first)
	}
	second, err := store.Search(ctx, api.SearchRequest{Project: "fixture", Query: "graph", Limit: 1, Cursor: first.Page.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if second.Page.Total != 2 || second.Page.Truncated || second.Page.NextCursor != "" || len(second.Matches) != 1 || second.Matches[0].ID == first.Matches[0].ID {
		t.Fatalf("unexpected second page: %+v", second)
	}
	if _, err := store.Search(ctx, api.SearchRequest{Project: "fixture", Query: "different", Limit: 1, Cursor: first.Page.NextCursor}); err == nil {
		t.Fatal("cursor was accepted for a different query")
	}
}

func TestSearchPatternFiltersAndRanksStructurally(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "graph.db")
	buildFixture(t, ctx, path, "generation-one")
	store, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	result, err := store.Search(ctx, api.SearchRequest{
		Project: "fixture", NamePattern: `^(Refresh|Build)`, QualifiedNamePattern: `^graph\.`,
		FilePattern: `internal/graph/*.go`, Relationship: "CALLS", MinDegree: intPointer(1), IncludeConnected: true, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Page.Total != 2 || len(result.Matches) != 2 {
		t.Fatalf("unexpected pattern results: %+v", result)
	}
	for _, match := range result.Matches {
		if match.Signals["degree"] != float64(1) && match.Signals["degree"] != 1 {
			t.Fatalf("degree signal missing: %+v", match)
		}
		if match.QualifiedName == "graph.RefreshAtomicWithOptions" && (match.Signals["in_degree"] != 0 || match.Signals["out_degree"] != 1) {
			t.Fatalf("entry degrees=%+v", match.Signals)
		}
		if len(match.Connected) != 1 {
			t.Fatalf("connected names=%+v", match)
		}
		if match.QualifiedName == "graph.BuildWithOptions" && (match.Signals["in_degree"] != 1 || match.Signals["out_degree"] != 0) {
			t.Fatalf("callee degrees=%+v", match.Signals)
		}
	}
	if _, err := store.Search(ctx, api.SearchRequest{Project: "fixture", NamePattern: "["}); err == nil {
		t.Fatal("invalid regex accepted")
	}
	if _, err := store.Search(ctx, api.SearchRequest{Project: "fixture", NamePattern: ".", Relationship: "calls"}); err == nil {
		t.Fatal("invalid relationship accepted")
	}
}

func intPointer(value int) *int { return &value }

func TestPinnedSearchPatternConversions(t *testing.T) {
	tests := map[string]string{
		"**/*.py":              "%%.py",
		"**/dir/**":            "%dir%",
		"*.go":                 "%.go",
		"src/**":               "src%",
		"**/test_*.py":         "%test_%.py",
		"file?.txt":            "file_.txt",
		"exact.go":             "exact.go",
		"**/custom-package/**": "%custom-package%",
		"**/**":                "%%",
		"src/[abc]/*.ts":       "src/[abc]/%.ts",
	}
	for pattern, want := range tests {
		if got := globToLike(pattern); got != want {
			t.Errorf("globToLike(%q)=%q, want %q", pattern, got, want)
		}
	}
	name, err := optionalPattern("refresh")
	if err != nil || !name.MatchString("RefreshAtomicWithOptions") {
		t.Fatalf("default name matching must be case-insensitive: %v, %v", name, err)
	}
	file, err := optionalFilePattern("graph.go")
	if err != nil || !file.MatchString("internal/graph/graph.go") {
		t.Fatalf("literal file patterns must be substring matches: %v, %v", file, err)
	}
}

func TestPublishDoesNotReplaceValidGraphOnFailure(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	repo := t.TempDir()
	ctx := context.Background()
	live, err := Publish(ctx, repo, func(ctx context.Context, path string) error {
		buildFixture(t, ctx, path, "generation-one")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Publish(ctx, repo, func(context.Context, string) error {
		return errors.New("injected build failure")
	}); err == nil {
		t.Fatal("expected publish failure")
	}
	if _, err := Publish(ctx, repo, func(_ context.Context, path string) error {
		return os.WriteFile(path, []byte("not a database"), 0o600)
	}); err == nil {
		t.Fatal("expected invalid-database failure")
	}
	store, err := OpenReadOnly(live)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	status, err := store.Status(ctx, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if status.Generation != "generation-one" {
		t.Fatalf("failed build replaced live graph: %+v", status)
	}
}

func TestPublishPreservesMemory(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	repo := t.TempDir()
	ctx := context.Background()
	if _, err := Publish(ctx, repo, func(ctx context.Context, path string) error {
		buildFixture(t, ctx, path, "generation-one")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	store, err := memory.OpenRoot(repo)
	if err != nil {
		t.Fatal(err)
	}
	ep, err := store.Capture(memory.CaptureInput{Kind: memory.KindSession, Title: "keep me", Text: "graph refresh must not wipe memory"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Publish(ctx, repo, func(ctx context.Context, path string) error {
		buildFixture(t, ctx, path, "generation-two")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	store, err = memory.OpenRoot(repo)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	got, err := store.Get(ep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "keep me" {
		t.Fatalf("memory wiped on graph publish: %+v", got)
	}
	graph, err := OpenReadOnly(paths.Resolve(repo).Database)
	if err != nil {
		t.Fatal(err)
	}
	defer graph.Close()
	status, err := graph.Status(ctx, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if status.Generation != "generation-two" {
		t.Fatalf("graph did not publish: %+v", status)
	}
}

func TestCacheIdentityCanonicalizesSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not generally available to unprivileged Windows tests")
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	repo := t.TempDir()
	link := filepath.Join(t.TempDir(), "repo-link")
	if err := os.Symlink(repo, link); err != nil {
		t.Fatal(err)
	}
	direct, err := CachePaths(repo)
	if err != nil {
		t.Fatal(err)
	}
	linked, err := CachePaths(link)
	if err != nil {
		t.Fatal(err)
	}
	if direct.Database != linked.Database {
		t.Fatalf("canonical identities differ: %s != %s", direct.Database, linked.Database)
	}
}

func TestPinnedGrammarInventory(t *testing.T) {
	if len(Languages) != 159 {
		t.Fatalf("grammar inventory = %d, want 159", len(Languages))
	}
	seen := map[string]bool{}
	for _, language := range Languages {
		if seen[language] {
			t.Fatalf("duplicate grammar %q", language)
		}
		seen[language] = true
	}
	if Capabilities().Complete {
		t.Fatal("development engine must not claim readiness before all gates pass")
	}
}

func buildFixture(t *testing.T, ctx context.Context, path, generation string) {
	t.Helper()
	root := t.TempDir()
	writeFixtureFile(t, root, "internal/graph/graph.go", "package graph\n")
	body, err := os.ReadFile(filepath.Join(root, "internal", "graph", "graph.go"))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body)
	store, err := OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(ctx, func(b *Builder) error {
		if err := b.PutProject(ProjectRecord{
			Name:          "fixture",
			RootPath:      root,
			Generation:    generation,
			EngineVersion: "development",
			IndexedAt:     time.Now().UTC(),
		}); err != nil {
			return err
		}
		if err := b.PutFile(FileRecord{Project: "fixture", Path: "internal/graph/graph.go", SHA256: hex.EncodeToString(digest[:]), Language: "go"}); err != nil {
			return err
		}
		first, err := b.PutNode(api.Node{
			Project: "fixture", Label: "Function", Name: "RefreshAtomicWithOptions",
			QualifiedName: "graph.RefreshAtomicWithOptions",
			Location:      api.Location{File: "internal/graph/graph.go", StartLine: 91, EndLine: 130},
		})
		if err != nil {
			return err
		}
		second, err := b.PutNode(api.Node{
			Project: "fixture", Label: "Function", Name: "BuildWithOptions",
			QualifiedName: "graph.BuildWithOptions",
			Location:      api.Location{File: "internal/graph/graph.go", StartLine: 73, EndLine: 89},
		})
		if err != nil {
			return err
		}
		if _, err := b.PutEdge(api.Edge{
			Project: "fixture", SourceID: first, TargetID: second, Type: "CALLS",
			Evidence: &api.Evidence{Strategy: "go_types", Confidence: 1},
		}); err != nil {
			return err
		}
		return b.PutCoverage("fixture", api.Coverage{
			Generation: generation, IndexMode: "full", RecordingStatus: "complete", HashRecordsComplete: true,
		})
	})
	if err == nil {
		err = store.Seal(ctx)
	}
	if closeErr := store.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestSearchDegreeZeroAndEntryPointSemantics(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "graph.db")
	buildFixture(t, ctx, path, "generation-one")
	writable, err := OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := writable.db.ExecContext(ctx, `INSERT INTO nodes(
		project,label,name,qualified_name,file_path,start_line,end_line,properties
	) VALUES('fixture','Function','UnusedHelper','graph.UnusedHelper','internal/graph/graph.go',132,134,'{}')`)
	if err != nil {
		writable.Close()
		t.Fatal(err)
	}
	id, err := result.LastInsertId()
	if err == nil {
		_, err = writable.db.ExecContext(ctx, `INSERT INTO nodes_fts(rowid,name,qualified_name,label,file_path) VALUES(?,?,?,?,?)`,
			id, "UnusedHelper", "graph.UnusedHelper", "Function", "internal/graph/graph.go")
	}
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

	zero := 0
	dead, err := store.Search(ctx, api.SearchRequest{
		Project: "fixture", Labels: []string{"Function"}, MaxDegree: &zero,
		ExcludeEntryPoints: true, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(dead.Matches) != 1 || dead.Matches[0].QualifiedName != "graph.UnusedHelper" {
		t.Fatalf("dead-code search=%+v", dead)
	}
	one := 1
	live, err := store.Search(ctx, api.SearchRequest{
		Project: "fixture", Labels: []string{"Function"}, MaxDegree: &one,
		ExcludeEntryPoints: true, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, match := range live.Matches {
		if match.QualifiedName == "graph.RefreshAtomicWithOptions" {
			t.Fatalf("outbound-only entry point was not excluded: %+v", live)
		}
	}
}

func TestContentlessFTSNodeUpdateRemovesOldTerms(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "graph.db")
	store, err := OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{Name: "fixture", RootPath: t.TempDir(), Generation: "one", EngineVersion: "test"}); err != nil {
			return err
		}
		if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "OldUniqueTerm", QualifiedName: "fixture.symbol"}); err != nil {
			return err
		}
		_, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "NewUniqueTerm", QualifiedName: "fixture.symbol"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	old, err := store.Search(ctx, api.SearchRequest{Project: "fixture", Query: "OldUniqueTerm"})
	if err != nil {
		t.Fatal(err)
	}
	newResult, err := store.Search(ctx, api.SearchRequest{Project: "fixture", Query: "NewUniqueTerm"})
	if err != nil || len(old.Matches) != 0 || len(newResult.Matches) != 1 {
		t.Fatalf("old=%#v new=%#v err=%v", old, newResult, err)
	}
}

func TestMarshalObjectTypedNilProperties(t *testing.T) {
	var properties api.Properties
	got, err := marshalObject(properties)
	if err != nil {
		t.Fatal(err)
	}
	if got != "{}" {
		t.Fatalf("typed nil properties = %q", got)
	}
	got, err = marshalObject(api.Properties{})
	if err != nil || got != "{}" {
		t.Fatalf("empty properties = %q err=%v", got, err)
	}
}
