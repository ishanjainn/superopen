package nativedocs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/nativedocs"
)

func TestUpsertRuleIsolatesVendorCopies(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".claude", "rules"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".claude", "rules", "coding.md"), []byte("old claude\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, ".cursor", "rules"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".cursor", "rules", "coding.mdc"), []byte("old cursor\n"), 0o644)

	paths := harness.Resolve(root)
	body := "# Coding\n\nSynced body\n"
	if err := nativedocs.UpsertRule(paths, "coding", body, nativedocs.WriteOpts{Vendor: "claude-code"}); err != nil {
		t.Fatal(err)
	}
	claude, _ := os.ReadFile(filepath.Join(root, ".claude", "rules", "coding.md"))
	cursor, _ := os.ReadFile(filepath.Join(root, ".cursor", "rules", "coding.mdc"))
	if !strings.Contains(string(claude), "Synced body") || strings.Contains(string(cursor), "Synced body") {
		t.Fatalf("vendor isolation failed: claude=%q cursor=%q", claude, cursor)
	}
}

func TestUpsertRuleCreatesUnderSessionVendor(t *testing.T) {
	root := t.TempDir()
	paths := harness.Resolve(root)
	if err := nativedocs.UpsertRule(paths, "security", "# Security\n", nativedocs.WriteOpts{Vendor: "claude-code"}); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, ".claude", "rules", "security.md")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("expected create under claude rules: %v", err)
	}
}

func TestUpsertSkillIsolatesAndCreates(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".codex", "skills", "deploy"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".codex", "skills", "deploy", "SKILL.md"), []byte("old\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, ".pi", "skills", "deploy"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".pi", "skills", "deploy", "SKILL.md"), []byte("old\n"), 0o644)

	paths := harness.Resolve(root)
	if err := nativedocs.UpsertSkill(paths, "deploy", "# Deploy\n\nnew\n", nativedocs.WriteOpts{Vendor: "pi"}); err != nil {
		t.Fatal(err)
	}
	pi, _ := os.ReadFile(filepath.Join(root, ".pi", "skills", "deploy", "SKILL.md"))
	codex, _ := os.ReadFile(filepath.Join(root, ".codex", "skills", "deploy", "SKILL.md"))
	if !strings.Contains(string(pi), "new") || strings.Contains(string(codex), "new") {
		t.Fatalf("vendor isolation failed: pi=%q codex=%q", pi, codex)
	}

	if err := nativedocs.UpsertSkill(paths, "fresh", "# Fresh\n", nativedocs.WriteOpts{Vendor: "codex"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".codex", "skills", "fresh", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverPrefersClaudeRules(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".claude", "rules"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".claude", "rules", "team.md"), []byte("# team\n"), 0o644)
	paths := harness.Resolve(root)
	if !strings.Contains(filepath.ToSlash(paths.RulesDir), "/.claude/rules") {
		t.Fatalf("RulesDir=%s", paths.RulesDir)
	}
	full, err := nativedocs.RulePath(paths, "coding")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(full, ".md") {
		t.Fatalf("expected .md got %s", full)
	}
	roots := nativedocs.DiscoverRoots(root)
	if roots.RulesKind != "claude" {
		t.Fatalf("RulesKind=%s", roots.RulesKind)
	}
}

func TestDiscoverPrefersCursorRules(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".cursor", "rules"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".cursor", "rules", "team.mdc"), []byte("# team\n"), 0o644)
	paths := harness.Resolve(root)
	if !strings.Contains(filepath.ToSlash(paths.RulesDir), "/.cursor/rules") {
		t.Fatalf("RulesDir=%s", paths.RulesDir)
	}
	full, err := nativedocs.RulePath(paths, "coding")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(full, ".mdc") {
		t.Fatalf("expected .mdc got %s", full)
	}
}

func TestDiscoverPrefersClaudeSkills(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".claude", "skills", "deploy"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".claude", "skills", "deploy", "SKILL.md"), []byte("# deploy\n"), 0o644)
	paths := harness.Resolve(root)
	if !strings.Contains(filepath.ToSlash(paths.SkillsDir), "/.claude/skills") {
		t.Fatalf("SkillsDir=%s", paths.SkillsDir)
	}
}

func TestPruneLearnedAndRules(t *testing.T) {
	root := t.TempDir()
	paths := harness.Resolve(root)
	_ = paths.EnsureDirs()
	body := nativedocs.DefaultAgentsBody("", "", "")
	if err := nativedocs.EnsureAgentsMD(paths, body, true); err != nil {
		t.Fatal(err)
	}
	if err := nativedocs.AppendLearned(paths, "- Prefer X"); err != nil {
		t.Fatal(err)
	}
	if err := nativedocs.AppendLearned(paths, "- Prefer Y"); err != nil {
		t.Fatal(err)
	}
	if err := nativedocs.RemoveLearnedContaining(paths.AgentsMD, "Prefer X"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(paths.AgentsMD)
	if strings.Contains(string(data), "Prefer X") || !strings.Contains(string(data), "Prefer Y") {
		t.Fatalf("learned prune failed:\n%s", data)
	}
	if err := nativedocs.AppendRule(paths, "coding", "- keep small"); err != nil {
		t.Fatal(err)
	}
	if err := nativedocs.AppendRule(paths, "coding", "- keep small"); err != nil {
		t.Fatal(err)
	}
	if err := nativedocs.RemoveRuleContaining(paths, "coding", "keep small"); err != nil {
		t.Fatal(err)
	}
}

func TestNestedAgentsMD(t *testing.T) {
	root := t.TempDir()
	paths := harness.Resolve(root)
	nested, err := nativedocs.AgentsFile(root, "cmd/so")
	if err != nil {
		t.Fatal(err)
	}
	if err := nativedocs.EnsureAgentsAt(nested, "# Local\n\ncmd/so notes\n", false); err != nil {
		t.Fatal(err)
	}
	if err := nativedocs.AppendLearnedAt(nested, "- Use cobra"); err != nil {
		t.Fatal(err)
	}
	found := paths.AgentsPaths()
	ok := false
	for _, p := range found {
		if p == nested {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("nested not listed: %v", found)
	}
}
