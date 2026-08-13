package discover_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/discover"
	"github.com/ishanjainn/superopen/internal/harness"
)

func TestCatalogCandidatesNextStack(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{
  "dependencies": {"next": "14.0.0", "react": "18.0.0", "@sentry/nextjs": "7.0.0"},
  "devDependencies": {"@playwright/test": "1.40.0"}
}`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "playwright.config.ts"), []byte("export default {}"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "pnpm-lock.yaml"), []byte("lockfileVersion: 9"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, ".env.example"), []byte("SECRET=\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, "web"), 0o755)
	_ = os.MkdirAll(filepath.Join(dir, "tests"), 0o755)

	paths := harness.Resolve(dir)
	_ = paths.EnsureDirs()
	sig := discover.DetectSignals(dir)
	if !sig.HasDep("next") || !sig.HasDep("playwright") || !sig.HasDep("sentry") {
		t.Fatalf("signals deps=%v", sig.Deps)
	}
	cands := discover.CatalogCandidates(dir, paths, sig, discover.GraphSummary{NodeCount: 100})
	kinds := map[discover.CandidateKind][]string{}
	for _, c := range cands {
		kinds[c.Kind] = append(kinds[c.Kind], c.Name)
		if c.Kind == discover.CandidateMCP && strings.Contains(strings.Join(c.Args, " "), "@latest") {
			t.Fatalf("mcp args must pin versions, got %v", c.Args)
		}
	}
	if !contains(kinds[discover.CandidateMCP], "playwright") {
		t.Fatalf("expected playwright mcp, got %v", kinds[discover.CandidateMCP])
	}
	if !contains(kinds[discover.CandidateMCP], "sentry") {
		t.Fatalf("expected sentry mcp, got %v", kinds[discover.CandidateMCP])
	}
	if contains(kinds[discover.CandidateSkill], "gen-test") || contains(kinds[discover.CandidateSkill], "frontend-design") || contains(kinds[discover.CandidateSkill], "pr-check") {
		t.Fatalf("catalog must not emit template skills, got %v", kinds[discover.CandidateSkill])
	}
	if len(kinds[discover.CandidateSkill]) != 0 {
		t.Fatalf("catalog must not emit skills, got %v", kinds[discover.CandidateSkill])
	}
	if !contains(kinds[discover.CandidateGuardrail], "block-env-edits") {
		t.Fatalf("expected env guardrail, got %v", kinds[discover.CandidateGuardrail])
	}
}

func TestCatalogCandidatesGoCLI(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/cli\n\ngo 1.22\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "go.sum"), []byte(""), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, "cmd"), 0o755)

	paths := harness.Resolve(dir)
	_ = paths.EnsureDirs()
	sig := discover.DetectSignals(dir)
	cands := discover.CatalogCandidates(dir, paths, sig, discover.GraphSummary{NodeCount: 40})
	for _, c := range cands {
		if c.Kind == discover.CandidateMCP && c.Name == "playwright" {
			t.Fatal("go CLI should not get playwright mcp")
		}
		if strings.EqualFold(c.Name, "memory") {
			t.Fatal("must never recommend memory mcp")
		}
	}
}

func TestBuildProfileIncludesCandidates(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"next":"14.0.0","react":"18.0.0"}}`), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, "web"), 0o755)
	paths := harness.Resolve(dir)
	_ = paths.EnsureDirs()
	p := discover.BuildProfile(dir, paths, "Node/TypeScript", "- web")
	if len(p.Candidates) == 0 {
		t.Fatal("expected catalog candidates on profile")
	}
	if len(p.Signals.Manifests) == 0 {
		t.Fatal("expected signals on profile")
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
