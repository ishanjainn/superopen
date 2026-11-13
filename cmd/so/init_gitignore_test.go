package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/paths"
)

func TestInitExposesManagedFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	root := t.TempDir()
	runGitInit(t, root)
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.pyc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := paths.Resolve(root).EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(root, ".so", "db", "so.db")
	if err := os.WriteFile(db, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := clearLegacyInitIgnores(root); err != nil {
		t.Fatal(err)
	}
	repoIgnore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if string(repoIgnore) != "*.pyc\n" {
		t.Fatalf("init must not edit the repo .gitignore, got %q", repoIgnore)
	}
	got := strings.Fields(runGitOutput(t, root, "ls-files", "--others", "--exclude-standard", ".so"))
	if strings.Join(got, ",") != ".so/.gitignore,.so/README.md" {
		t.Fatalf("only commit-safe files should be visible: %q", got)
	}
	if err := clearLegacyInitIgnores(root); err != nil {
		t.Fatal(err)
	}
	excludePath := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "--git-path", "info/exclude"))
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(root, excludePath)
	}
	exclude, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if strings.Contains(string(exclude), "/.so/") {
		t.Fatalf("local ignore must not hide commit-safe files: %q", exclude)
	}
}

func TestInitMigratesLocalExclude(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	root := t.TempDir()
	runGitInit(t, root)
	if err := paths.Resolve(root).EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	excludePath := strings.TrimSpace(runGitOutput(t, root, "rev-parse", "--git-path", "info/exclude"))
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(root, excludePath)
	}
	if err := os.WriteFile(excludePath, []byte("# Superopen machine-local data\n/.so/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := clearLegacyInitIgnores(root); err != nil {
		t.Fatal(err)
	}
	got := strings.Fields(runGitOutput(t, root, "ls-files", "--others", "--exclude-standard", ".so"))
	if strings.Join(got, ",") != ".so/.gitignore,.so/README.md" {
		t.Fatalf("legacy local exclude must be removed: %q", got)
	}
}

func TestInitNestedPackage(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	top := t.TempDir()
	runGitInit(t, top)
	root := filepath.Join(top, "pkg")
	if err := paths.Resolve(root).EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	if err := clearLegacyInitIgnores(root); err != nil {
		t.Fatal(err)
	}
	got := strings.Fields(runGitOutput(t, top, "ls-files", "--others", "--exclude-standard", "pkg/.so"))
	if strings.Join(got, ",") != "pkg/.so/.gitignore,pkg/.so/README.md" {
		t.Fatalf("nested setup should expose only commit-safe files: %q", got)
	}
	excludePath := strings.TrimSpace(runGitOutput(t, top, "rev-parse", "--git-path", "info/exclude"))
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(top, excludePath)
	}
	content, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "/pkg/.so/") {
		t.Fatalf("nested package must not be hidden by local exclude: %q", content)
	}
}

func TestInitPreservesExistingGitignore(t *testing.T) {
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
	if err := clearLegacyInitIgnores(root); err != nil {
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

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v %s", strings.Join(args, " "), err, out)
	}
	return string(out)
}
