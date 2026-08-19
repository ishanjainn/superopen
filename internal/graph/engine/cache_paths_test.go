package engine_test

import (
	"os"
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

func TestMigrateLegacyCacheIfNeeded(t *testing.T) {
	repo := t.TempDir()
	legacy, err := engine.LegacyCachePaths(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(legacy.Root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy.Database, []byte("sqlite-placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := engine.MigrateLegacyCacheIfNeeded(repo); err != nil {
		t.Fatal(err)
	}
	dst, err := engine.CachePaths(repo)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(dst.Database)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "sqlite-placeholder" {
		t.Fatalf("migrated body %q", body)
	}
}
