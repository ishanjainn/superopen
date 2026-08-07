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
	// sessions/ is a deliberate full ignore, not a "broad ignore" mistake:
	// session meta/transcripts can carry the user's email, prompts, and file
	// contents, which must never land in a shared (esp. OSS) repo.
	for _, want := range []string{"traces/", "session-state/", "graph/cache/", "sessions/"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing local ignore %q in:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"*\n", "memory/\n", "graph/\n"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("unexpected broad ignore %q in:\n%s", unwanted, got)
		}
	}
}

func TestSeedUpgradesOnlyGeneratedDefaultGitignore(t *testing.T) {
	repoRoot := t.TempDir()
	paths := harness.Resolve(repoRoot)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	initial := "# Superopen - commit team-useful harness docs; ignore local runtime state.\n*\n!.gitignore\n!knowledge/\n!knowledge/**\n!rules/\n!rules/**\n!skills/\n!skills/**\n!guardrails/\n!guardrails/**\n!evals/\n!evals/**\n!AGENT.md\n!config.yaml\n"
	gitignore := filepath.Join(paths.Root, ".gitignore")
	if err := os.WriteFile(gitignore, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := seed.Seed(paths, seed.SeedOptions{TemplateRoot: filepath.Join("..", "..", "templates")}); err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(gitignore)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(updated), "*\n") || !strings.Contains(string(updated), "traces/") {
		t.Fatalf("initial gitignore default was not upgraded:\n%s", updated)
	}

	custom := []byte("custom-state/\n")
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

func TestSeedUpgradesPreviousDefaultToIgnoreSessions(t *testing.T) {
	// Existing installs seeded before session data was fully ignored (only
	// sessions/pending-spawns.json was excluded) must get upgraded by a plain
	// re-run of `so init`/`so sync`, not just brand-new repos - session meta
	// can carry the user's email, prompts, and file contents.
	repoRoot := t.TempDir()
	paths := harness.Resolve(repoRoot)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	previous := "# Superopen - commit the portable team harness. Ignore only machine-local\n" +
		"# telemetry, liveness state, caches, and transient work-in-progress records.\n" +
		"traces/\naudit/\nrun/\nsession-state/\nport/\nui-prefs.json\nfinalize-pending\n\n" +
		"# These are regenerated or describe only a local agent process.\n" +
		"memory/history/\nmemory/harvest-ledger.json\nmemory/idle-sweep-at\n" +
		"memory/last-refresh.json\nmemory/pending-harvest.json\nmemory/refresh-status.json\n" +
		"sessions/pending-spawns.json\ngraph/cache/\n"
	gitignore := filepath.Join(paths.Root, ".gitignore")
	if err := os.WriteFile(gitignore, []byte(previous), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := seed.Seed(paths, seed.SeedOptions{TemplateRoot: filepath.Join("..", "..", "templates")}); err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(gitignore)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "sessions/\n") {
		t.Fatalf("expected the previous default to be upgraded to fully ignore sessions/:\n%s", updated)
	}
}
