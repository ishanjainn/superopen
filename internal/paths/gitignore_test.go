package paths_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/paths"
)

func TestRemoveRepoIgnoreStripsOnlyBlock(t *testing.T) {
	root := initGit(t, map[string]string{
		".gitignore": "vendor/\n\n# BEGIN SUPEROPEN\n.so/\n# END SUPEROPEN\n",
		"README":     "hi\n",
	})
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

func TestRemoveRepoIgnoreNonGit(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("# BEGIN SUPEROPEN\n.so/\n# END SUPEROPEN\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := paths.RemoveRepoIgnore(dir); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), ".so/") {
		t.Fatal("must not edit .gitignore outside git")
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
