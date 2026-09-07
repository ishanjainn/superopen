package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/paths"
)

func TestSeedLinkedWorktreeCopiesParentDB(t *testing.T) {
	base := t.TempDir()
	parent := filepath.Join(base, "main")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	initGitRepoAt(t, parent, map[string]string{"README": "hi\n"})
	seedGraphDB(t, parent, map[string]string{"README": "hi\n"})
	parentDB := paths.Resolve(parent).Database
	before, err := os.ReadFile(parentDB)
	if err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(base, "feat")
	runGit(t, parent, "worktree", "add", "-b", "feat", wt)
	if paths.Managed(wt) {
		t.Fatal("worktree must start uninited")
	}
	SeedLinkedWorktree(wt)
	if !paths.Managed(wt) {
		t.Fatal("seed must create worktree .so/")
	}
	if _, err := os.Stat(paths.Resolve(wt).Database); err != nil {
		t.Fatalf("seeded db missing: %v", err)
	}
	after, err := os.ReadFile(parentDB)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("seed must not write the parent database")
	}
}

func TestSeedLinkedWorktreeHonorsNoSeed(t *testing.T) {
	base := t.TempDir()
	parent := filepath.Join(base, "main")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	initGitRepoAt(t, parent, map[string]string{"README": "hi\n"})
	seedGraphDB(t, parent, map[string]string{"README": "hi\n"})
	wt := filepath.Join(base, "feat")
	runGit(t, parent, "worktree", "add", "-b", "feat", wt)
	t.Setenv(noSeedEnv, "1")
	SeedLinkedWorktree(wt)
	if paths.Managed(wt) {
		t.Fatal("SUPEROPEN_NO_SEED=1 must skip seeding")
	}
}

func TestSeedLinkedWorktreeSkipsNonWorktree(t *testing.T) {
	root := initGitRepo(t, map[string]string{"README": "hi\n"})
	SeedLinkedWorktree(root)
	if paths.Managed(root) {
		t.Fatal("primary unmanaged checkout must not auto-init")
	}
}

func TestSeedLinkedWorktreeFailedCopyLeavesNoSO(t *testing.T) {
	base := t.TempDir()
	parent := filepath.Join(base, "main")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	initGitRepoAt(t, parent, map[string]string{"README": "hi\n"})
	layout := paths.Resolve(parent)
	if err := layout.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.Database, []byte("not sqlite"), 0o644); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(base, "feat")
	runGit(t, parent, "worktree", "add", "-b", "feat", wt)
	SeedLinkedWorktree(wt)
	if paths.Managed(wt) {
		t.Fatal("failed copy must not leave a Managed empty .so/")
	}
}

func initGitRepoAt(t *testing.T, root string, files map[string]string) {
	t.Helper()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "so@example.com")
	runGit(t, root, "config", "user.name", "so")
	for rel, body := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "init")
}
