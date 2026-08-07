package retrieve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/harness"
)

func TestRebuildAndSearch(t *testing.T) {
	dir := t.TempDir()
	paths := harness.Resolve(dir)
	_ = paths.EnsureDirs()
	_ = os.WriteFile(paths.AgentsMD, []byte("# Auth\n\nJWT validation lives in pkg/auth.\n"), 0o644)
	n, err := Rebuild(dir, paths)
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("expected indexed chunks")
	}
	hits, err := Search(paths, "JWT auth", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("expected hits")
	}
}

func TestSearchVendorWeighted(t *testing.T) {
	dir := t.TempDir()
	paths := harness.Resolve(dir)
	_ = paths.EnsureDirs()
	_ = os.WriteFile(paths.AgentsMD, []byte("# Shared\nrace patterns matter\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, ".claude", "rules"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, ".claude", "rules", "go.md"), []byte("race patterns in claude rules\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, ".cursor", "rules"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, ".cursor", "rules", "go.mdc"), []byte("race patterns in cursor rules\n"), 0o644)
	if _, err := Rebuild(dir, paths); err != nil {
		t.Fatal(err)
	}
	hits, err := SearchWith(paths, "race patterns", SearchOptions{Limit: 10, Vendor: "claude-code"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) < 2 {
		t.Fatalf("hits=%v", hits)
	}
	// Claude rules should outrank Cursor for a Claude session (AGENTS may still be first).
	var claudeScore, cursorScore float64
	for _, h := range hits {
		switch {
		case strings.Contains(h.Path, ".claude/rules"):
			claudeScore = h.Score
		case strings.Contains(h.Path, ".cursor/rules"):
			cursorScore = h.Score
		}
	}
	if claudeScore <= cursorScore {
		t.Fatalf("claude=%v cursor=%v hits=%+v", claudeScore, cursorScore, hits)
	}
}

func TestRebuildIndexesAllVendorGuidance(t *testing.T) {
	dir := t.TempDir()
	paths := harness.Resolve(dir)
	_ = paths.EnsureDirs()

	_ = os.WriteFile(paths.AgentsMD, []byte("# Root agents\n"), 0o644)
	nested := filepath.Join(dir, "cmd", "so")
	_ = os.MkdirAll(nested, 0o755)
	_ = os.WriteFile(filepath.Join(nested, "AGENTS.md"), []byte("# Nested CLI agents\n"), 0o644)

	_ = os.MkdirAll(filepath.Join(dir, ".claude", "rules"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, ".claude", "rules", "go.md"), []byte("# Claude go rules\nAlways race test.\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, ".agents", "rules"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, ".agents", "rules", "coding.md"), []byte("# Agents coding\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, ".cursor", "rules"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, ".cursor", "rules", "team.mdc"), []byte("---\n---\n# Cursor team\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, ".cursor", "rules", "superopen.mdc"), []byte("skip me\n"), 0o644)

	_ = os.MkdirAll(filepath.Join(dir, ".cursor", "skills", "pr-hygiene"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, ".cursor", "skills", "pr-hygiene", "SKILL.md"), []byte("# PR hygiene skill\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, ".claude", "skills", "so"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, ".claude", "skills", "so", "SKILL.md"), []byte("# reserved so skill\n"), 0o644)

	n, err := Rebuild(dir, paths)
	if err != nil {
		t.Fatal(err)
	}
	if n < 5 {
		t.Fatalf("expected multi-file index, got %d", n)
	}
	data, err := os.ReadFile(filepath.Join(paths.GraphDir, "retrieve_index.json"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, want := range []string{
		"AGENTS.md",
		"cmd/so/AGENTS.md",
		".claude/rules/go.md",
		".agents/rules/coding.md",
		".cursor/rules/team.mdc",
		".cursor/skills/pr-hygiene/SKILL.md",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("index missing %s:\n%s", want, body)
		}
	}
	if strings.Contains(body, "superopen.mdc") {
		t.Fatal("should skip injector superopen.mdc")
	}
	if strings.Contains(body, ".claude/skills/so/SKILL.md") {
		t.Fatal("should skip reserved /so skill")
	}
}
