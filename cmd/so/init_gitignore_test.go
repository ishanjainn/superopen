package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/paths"
)

func TestWriteInitGitignoresHidesSO(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	root := t.TempDir()
	runGitInit(t, root)
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.pyc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "init")
	if err := paths.Resolve(root).EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	if err := writeInitGitignores(root); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", root, "status", "--porcelain")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git status: %v %s", err, out)
	}
	if strings.Contains(string(out), ".so") {
		t.Fatalf("git status must not mention .so/: %q", out)
	}
}

func TestWriteInitGitignoresPreservesExisting(t *testing.T) {
	root := t.TempDir()
	layout := paths.Resolve(root)
	if err := layout.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	ignorePath := filepath.Join(layout.Root, ".gitignore")
	custom := "# custom\nsessions/\ndb/\nharvest/\nscratch/\n"
	if err := os.WriteFile(ignorePath, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeInitGitignores(root); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(ignorePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != custom {
		t.Fatalf("existing .so/.gitignore must be left alone, got %q", got)
	}
}

func runGitInit(t *testing.T, root string) {
	t.Helper()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "so@example.com")
	runGit(t, root, "config", "user.name", "so")
	runGit(t, root, "config", "commit.gpgsign", "false")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v %s", strings.Join(args, " "), err, out)
	}
}
