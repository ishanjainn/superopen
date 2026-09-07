package watch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/engine"
	"github.com/ishanjainn/superopen/internal/paths"
)

func TestProbeSeesSecondEditOfDirtyFile(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "so@example.com")
	runGit(t, root, "config", "user.name", "so")
	file := filepath.Join(root, "a.go")
	if err := os.WriteFile(file, []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "init")
	if err := os.MkdirAll(paths.Resolve(root).DBDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := engine.WriteFingerprint(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("package a\nconst x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirty, err := engine.ProbeDirty(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !dirty {
		t.Fatal("first edit must dirty the probe")
	}
	if err := os.WriteFile(file, []byte("package a\nconst x = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirty, err = engine.ProbeDirty(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !dirty {
		t.Fatal("second edit of an already-dirty file must still be dirty")
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v %s", args, err, out)
	}
}
