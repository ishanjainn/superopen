package paths_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/paths"
)

func TestLinkedWorktreeParent(t *testing.T) {
	base := t.TempDir()
	parent := filepath.Join(base, "main")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = parent
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v %s", strings.Join(args, " "), err, out)
		}
	}
	run("init")
	run("config", "user.email", "so@example.com")
	run("config", "user.name", "so")
	if err := os.WriteFile(filepath.Join(parent, "README"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "init")
	if got, ok := paths.LinkedWorktreeParent(parent); ok {
		t.Fatalf("primary checkout is not a linked worktree, got %q", got)
	}
	wt := filepath.Join(base, "feat")
	cmd := exec.Command("git", "-C", parent, "worktree", "add", "-b", "feat", wt)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("worktree add: %v %s", err, out)
	}
	got, ok := paths.LinkedWorktreeParent(wt)
	if !ok {
		t.Fatal("linked worktree parent not detected")
	}
	want, err := filepath.EvalSymlinks(parent)
	if err != nil {
		want = parent
	}
	got, err = filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(got) != filepath.Clean(want) {
		t.Fatalf("parent = %q want %q", got, want)
	}
}
