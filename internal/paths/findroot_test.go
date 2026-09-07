package paths_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ishanjainn/superopen/internal/paths"
)

func TestFindRootUnmanagedGitPrefersNearestSO(t *testing.T) {
	parent := t.TempDir()
	if err := os.Mkdir(filepath.Join(parent, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(parent, "bench", "locomo")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := paths.Resolve(child).EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(child, "run", "cwd")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := paths.FindRoot(nested)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(child)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("unmanaged git parent must not steal nested .so: got %q want %q", got, want)
	}
}

func TestFindRootManagedGitWinsOverNestedSO(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := paths.Resolve(repo).EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(repo, "internal", "pkg")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := paths.Resolve(pkg).EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	got, err := paths.FindRoot(filepath.Join(pkg, "foo"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(repo)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("managed git root must stay the project root: got %q want %q", got, want)
	}
}

func TestFindRootGitfileAndWindowsJoin(t *testing.T) {
	repo := t.TempDir()
	// Worktree-style .git file, not a directory. filepath must not split on '/'.
	if err := os.WriteFile(filepath.Join(repo, ".git"), []byte("gitdir: /somewhere"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := paths.Resolve(repo).EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	start := filepath.Join(repo, "src", "nested")
	if err := os.MkdirAll(start, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := paths.FindRoot(start)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(repo)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("gitfile root: got %q want %q", got, want)
	}
	if runtime.GOOS == "windows" && filepath.Separator != '\\' {
		t.Fatal("windows tests must use filepath separator")
	}
}
