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
	for _, want := range []string{"audit/", "graph/", "memory/", "sessions/", ".graph-v2-*"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing local ignore %q in:\n%s", want, got)
		}
	}
	for _, obsolete := range []string{"traces/", "session-state/", "evals/", "recommendations/", "run/"} {
		if strings.Contains(got, obsolete) {
			t.Errorf("obsolete ignore %q in:\n%s", obsolete, got)
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

func TestSeedRemovesObsoleteRuntimeMarkers(t *testing.T) {
	repoRoot := t.TempDir()
	paths := harness.Resolve(repoRoot)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	legacy := []string{
		filepath.Join(paths.Root, "finalize-pending"),
		filepath.Join(paths.MemoryDir, "approval-mismatch-at"),
		filepath.Join(paths.MemoryDir, "idle-sweep-at"),
		filepath.Join(paths.MemoryDir, "pending-harvest.json"),
		filepath.Join(paths.MemoryDir, "last-refresh.json"),
		filepath.Join(paths.GraphDir, "cache", "last_query_stamp"),
		filepath.Join(paths.Root, ".graph-v2-leftover", "x"),
	}
	for _, path := range legacy {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("legacy"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := seed.Seed(paths, seed.SeedOptions{TemplateRoot: filepath.Join("..", "..", "templates")}); err != nil {
		t.Fatal(err)
	}
	for _, path := range legacy {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("obsolete artifact remains: %s (%v)", path, err)
		}
	}
}
