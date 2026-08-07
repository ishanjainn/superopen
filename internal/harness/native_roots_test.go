package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverNativeRootsPrefersClaudeRules(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".claude", "rules"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".claude", "rules", "go.md"), []byte("# go\n"), 0o644)
	// Injector-only cursor rules must not steal the preference.
	_ = os.MkdirAll(filepath.Join(root, ".cursor", "rules"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".cursor", "rules", "superopen.mdc"), []byte("---\n"), 0o644)

	rules, skills := discoverNativeRoots(root)
	if !strings.Contains(filepath.ToSlash(rules), "/.claude/rules") {
		t.Fatalf("RulesDir=%s", rules)
	}
	if !strings.Contains(filepath.ToSlash(skills), "/.agents/skills") {
		t.Fatalf("default SkillsDir=%s", skills)
	}
	if KindForRulesDir(rules) != "claude" {
		t.Fatalf("kind=%s", KindForRulesDir(rules))
	}
}

func TestDiscoverNativeRootsPrefersCursorSkills(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".cursor", "skills", "pr-hygiene"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".cursor", "skills", "pr-hygiene", "SKILL.md"), []byte("# pr\n"), 0o644)

	_, skills := discoverNativeRoots(root)
	if !strings.Contains(filepath.ToSlash(skills), "/.cursor/skills") {
		t.Fatalf("SkillsDir=%s", skills)
	}
}

func TestDiscoverNativeRootsAllVendorRules(t *testing.T) {
	cases := []struct {
		rel  string
		kind string
		file string
	}{
		{".gemini/rules", "gemini", "style.md"},
		{".codex/rules", "codex", "safety.md"},
		{".opencode/rules", "opencode", "ux.md"},
		{".github/instructions", "copilot", "go.instructions.md"},
		{".pi/rules", "pi", "notes.md"},
		{".agents/rules", "agents", "coding.md"},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, filepath.FromSlash(tc.rel))
			_ = os.MkdirAll(dir, 0o755)
			_ = os.WriteFile(filepath.Join(dir, tc.file), []byte("# x\n"), 0o644)
			rules, _ := discoverNativeRoots(root)
			if filepath.Clean(rules) != filepath.Clean(dir) {
				t.Fatalf("got %s want %s", rules, dir)
			}
			if KindForRulesDir(rules) != tc.kind {
				t.Fatalf("kind=%s", KindForRulesDir(rules))
			}
		})
	}
}

func TestDiscoverNativeRootsNestedClaudeRule(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".claude", "rules", "backend")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "api.md"), []byte("# api\n"), 0o644)
	rules, _ := discoverNativeRoots(root)
	if !strings.Contains(filepath.ToSlash(rules), "/.claude/rules") {
		t.Fatalf("RulesDir=%s", rules)
	}
}

func TestDiscoverNativeRootsSkillsVendors(t *testing.T) {
	for _, rel := range []string{".gemini/skills", ".opencode/skills", ".codex/skills", ".github/skills", ".pi/skills"} {
		t.Run(rel, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, filepath.FromSlash(rel), "deploy")
			_ = os.MkdirAll(dir, 0o755)
			_ = os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# deploy\n"), 0o644)
			_, skills := discoverNativeRoots(root)
			want := filepath.Join(root, filepath.FromSlash(rel))
			if filepath.Clean(skills) != filepath.Clean(want) {
				t.Fatalf("got %s want %s", skills, want)
			}
		})
	}
}
