package paths_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/paths"
)

func TestEnsureRepoIgnoreExistingGitignore(t *testing.T) {
	root := initGit(t, map[string]string{
		".gitignore": "*.pyc\n__pycache__/\n",
		"README":     "hi\n",
	})
	if err := paths.Resolve(root).EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	got, err := paths.EnsureRepoIgnore(root)
	if err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Fatal("expected gitignore path")
	}
	body, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.HasPrefix(text, "*.pyc\n__pycache__/\n") {
		t.Fatalf("must preserve prior content: %q", text)
	}
	if !strings.Contains(text, "# BEGIN SUPEROPEN") || !strings.Contains(text, ".so/") {
		t.Fatalf("missing Superopen block: %q", text)
	}
	status := gitPorcelain(t, root)
	if strings.Contains(status, ".so") {
		t.Fatalf(".so/ must be ignored, porcelain=%q", status)
	}
}

func TestEnsureRepoIgnoreFreshRepo(t *testing.T) {
	root := initGit(t, map[string]string{"README": "hi\n"})
	if _, err := paths.EnsureRepoIgnore(root); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "# BEGIN SUPEROPEN") || !strings.Contains(text, ".so/") {
		t.Fatalf("fresh gitignore: %q", text)
	}
}

func TestEnsureRepoIgnoreIdempotent(t *testing.T) {
	root := initGit(t, map[string]string{".gitignore": "vendor/\n", "README": "hi\n"})
	if _, err := paths.EnsureRepoIgnore(root); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := paths.EnsureRepoIgnore(root); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("second call must be a no-op\nfirst=%q\nsecond=%q", first, second)
	}
}

func TestEnsureRepoIgnoreAlreadyIgnored(t *testing.T) {
	root := initGit(t, map[string]string{
		".gitignore": ".so/\n",
		"README":     "hi\n",
	})
	path, err := paths.EnsureRepoIgnore(root)
	if err != nil {
		t.Fatal(err)
	}
	if path != "" {
		t.Fatalf("already-ignored must not rewrite gitignore, path=%q", path)
	}
	body, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "# BEGIN SUPEROPEN") {
		t.Fatalf("must not add a block when git already ignores .so/: %q", body)
	}
}

func TestEnsureRepoIgnoreNonGit(t *testing.T) {
	dir := t.TempDir()
	path, err := paths.EnsureRepoIgnore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if path != "" {
		t.Fatalf("non-git must no-op, got %q", path)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); !os.IsNotExist(err) {
		t.Fatal("must not create .gitignore outside git")
	}
}

func TestEnsureRepoIgnoreNestedRoot(t *testing.T) {
	root := initGit(t, map[string]string{"README": "hi\n"})
	pkg := filepath.Join(root, "internal", "pkg")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := paths.EnsureRepoIgnore(pkg); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "internal/pkg/.so/") {
		t.Fatalf("nested pattern missing: %q", body)
	}
}

func TestEnsureRepoIgnoreCRLF(t *testing.T) {
	root := initGit(t, map[string]string{"README": "hi\n"})
	ignore := filepath.Join(root, ".gitignore")
	if err := os.WriteFile(ignore, []byte("*.pyc\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := paths.EnsureRepoIgnore(root); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(ignore)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "\r\n") {
		t.Fatalf("must keep CRLF: %q", body)
	}
	if strings.Contains(strings.ReplaceAll(string(body), "\r\n", ""), "\n") && !strings.Contains(string(body), "\r\n# BEGIN SUPEROPEN\r\n") {
		t.Fatalf("block must use CRLF: %q", body)
	}
}

func TestRemoveRepoIgnoreStripsOnlyBlock(t *testing.T) {
	root := initGit(t, map[string]string{".gitignore": "vendor/\n", "README": "hi\n"})
	if _, err := paths.EnsureRepoIgnore(root); err != nil {
		t.Fatal(err)
	}
	if err := paths.RemoveRepoIgnore(root); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if strings.Contains(text, "BEGIN SUPEROPEN") || strings.Contains(text, ".so/") {
		t.Fatalf("block must be gone: %q", text)
	}
	if !strings.Contains(text, "vendor/") {
		t.Fatalf("must keep other rules: %q", text)
	}
}

func TestEnsureRepoIgnoreHidesSOFromPorcelain(t *testing.T) {
	root := initGit(t, map[string]string{
		".gitignore": "*.pyc\n",
		"README":     "hi\n",
	})
	layout := paths.Resolve(root)
	if err := layout.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout.Root, ".gitignore"), []byte(paths.GitignoreContents), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := paths.EnsureRepoIgnore(root); err != nil {
		t.Fatal(err)
	}
	status := gitPorcelain(t, root)
	if strings.Contains(status, ".so") {
		t.Fatalf("git status must not mention .so after init gitignores: %q", status)
	}
}

func initGit(t *testing.T, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "so@example.com")
	runGit(t, root, "config", "user.name", "so")
	runGit(t, root, "config", "commit.gpgsign", "false")
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
	return root
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v %s", strings.Join(args, " "), err, out)
	}
}

func gitPorcelain(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "status", "--porcelain")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git status: %v %s", err, out)
	}
	return string(out)
}
