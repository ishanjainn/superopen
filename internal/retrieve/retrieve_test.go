package retrieve

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/superopen/so/internal/harness"
)

func TestRebuildAndSearch(t *testing.T) {
	dir := t.TempDir()
	paths := harness.Resolve(dir)
	_ = os.MkdirAll(paths.KnowledgeDir, 0o755)
	_ = os.MkdirAll(paths.GraphDir, 0o755)
	_ = os.WriteFile(filepath.Join(paths.KnowledgeDir, "architecture.md"), []byte("# Auth\n\nJWT validation lives in pkg/auth.\n"), 0o644)
	n, err := Rebuild(dir, paths)
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("expected indexed chunks")
	}
	hits, err := Search(paths, "JWT auth", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("expected hits")
	}
}
