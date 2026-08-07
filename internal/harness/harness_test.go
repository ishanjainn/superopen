package harness_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/harness"
)

func TestResolveAndEnsureDirs(t *testing.T) {
	dir := t.TempDir()
	p := harness.Resolve(dir)
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
