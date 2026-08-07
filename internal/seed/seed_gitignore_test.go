package seed_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/discover"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/seed"
)

func TestSeedWritesTeamHarnessGitignore(t *testing.T) {
	repoRoot := t.TempDir()
	paths := harness.Resolve(repoRoot)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	if err := seed.Seed(paths, seed.SeedOptions{
		TemplateRoot: filepath.Join("..", "..", "templates"),
		Profile:      discover.Profile{},
	}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(paths.Root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{"traces/", "session-state/", "graph/", "memory/", "sessions/", "evals/history.json"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing local ignore %q in:\n%s", want, got)
		}
	}
}

func TestSeedDoesNotOverwriteCustomGitignore(t *testing.T) {
	repoRoot := t.TempDir()
	paths := harness.Resolve(repoRoot)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	custom := []byte("custom-state/\n")
	gitignore := filepath.Join(paths.Root, ".gitignore")
	if err := os.WriteFile(gitignore, custom, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := seed.Seed(paths, seed.SeedOptions{TemplateRoot: filepath.Join("..", "..", "templates")}); err != nil {
		t.Fatal(err)
	}
	kept, err := os.ReadFile(gitignore)
	if err != nil {
		t.Fatal(err)
	}
	if string(kept) != string(custom) {
		t.Fatalf("custom gitignore was overwritten:\n%s", kept)
	}
}
