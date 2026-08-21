package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileContentDigestIsStable(t *testing.T) {
	got := fileContentDigest([]byte("package fixture\n"))
	if got == "" || len(got) != 32 {
		t.Fatalf("digest=%q", got)
	}
	if fileContentDigest([]byte("package fixture\n")) != got {
		t.Fatal("digest is not stable")
	}
	if fileContentDigest([]byte("package other\n")) == got {
		t.Fatal("digest collided across distinct inputs")
	}
}

func TestGrammarsForFilesSelectsPresentLanguages(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "app.py"), []byte("x = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := grammarsForFiles(root, []string{"main.go", "app.py", "README"}, nil)
	if len(got) != 2 || got[0] != "go" || got[1] != "python" {
		t.Fatalf("grammars=%v", got)
	}
}

func TestPlanIncrementalSkipsHashWalkWhenDatabaseMissing(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := PlanIncremental(t.Context(), root, "fixture", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !got.RequiresFull {
		t.Fatalf("missing database should require a full build: %#v", got)
	}
}

func TestDatabaseExists(t *testing.T) {
	root := t.TempDir()
	if databaseExists(root) {
		t.Fatal("empty repo reported an existing graph database")
	}
}

func TestParseWorkerCountCaps(t *testing.T) {
	if got := parseWorkerCount(); got < 1 || got > maxParseWorkers {
		t.Fatalf("workers=%d", got)
	}
}
