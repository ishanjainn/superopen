package engine_test

import (
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/engine"
	"github.com/ishanjainn/superopen/internal/paths"
)

func TestCachePathsUsesRepoSODb(t *testing.T) {
	repo := t.TempDir()
	got, err := engine.CachePaths(repo)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := engine.CanonicalRoot(repo)
	if err != nil {
		t.Fatal(err)
	}
	layout := paths.Resolve(canonical)
	if got.Database != layout.Database {
		t.Fatalf("database=%q want %q", got.Database, layout.Database)
	}
	if filepath.Base(got.Root) != paths.DBName {
		t.Fatalf("root basename=%q want %q", filepath.Base(got.Root), paths.DBName)
	}
	if filepath.Base(got.Database) != paths.DatabaseFile {
		t.Fatalf("db file=%q want %q", filepath.Base(got.Database), paths.DatabaseFile)
	}
}

