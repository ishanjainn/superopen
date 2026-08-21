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
