package paths_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/paths"
)

func TestResolveAndEnsureDirs(t *testing.T) {
	dir := t.TempDir()
	p := paths.Resolve(dir)
	if err := p.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	if !p.Exists() {
		t.Fatal("expected harness to exist")
	}
	if _, err := os.Stat(filepath.Join(p.Root, "sessions")); err != nil {
		t.Fatal(err)
	}
	ignore, err := os.ReadFile(filepath.Join(p.Root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if string(ignore) != paths.GitignoreContents {
		t.Fatalf("gitignore = %q", ignore)
	}
	readmePath := filepath.Join(p.Root, "README.md")
	readme, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(readme) != paths.ReadmeContents {
		t.Fatalf("readme = %q", readme)
	}
	if err := os.WriteFile(readmePath, []byte("custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := p.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	if readme, err := os.ReadFile(readmePath); err != nil || string(readme) != "custom\n" {
		t.Fatalf("existing readme must be preserved: %q, %v", readme, err)
	}
}

func TestManaged(t *testing.T) {
	dir := t.TempDir()
	if paths.Managed(dir) {
		t.Fatal("empty tree is not managed")
	}
	if err := paths.Resolve(dir).EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	if !paths.Managed(dir) {
		t.Fatal("expected managed after .so exists")
	}
}

func TestWriteGitignoreMigratesGeneratedVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".so", ".gitignore")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# Superopen machine-local data (do not commit).\nsessions/\ndb/\nharvest/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := paths.WriteGitignore(dir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != paths.GitignoreContents {
		t.Fatalf("generated gitignore was not updated: %q", got)
	}
}
