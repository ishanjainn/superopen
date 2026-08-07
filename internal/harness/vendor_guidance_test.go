package harness

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeVendorKind(t *testing.T) {
	cases := map[string]string{
		"claude-code": "claude",
		"Claude_Code": "claude",
		"cursor":      "cursor",
		"codex":       "codex",
		"copilot-cli": "copilot",
		"pi":          "pi",
		"":            "",
	}
	for in, want := range cases {
		if got := NormalizeVendorKind(in); got != want {
			t.Fatalf("%q → %q want %q", in, got, want)
		}
	}
}

func TestFindExistingRulesAndSkills(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".claude", "rules"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".claude", "rules", "coding.md"), []byte("claude\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, ".cursor", "rules"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".cursor", "rules", "coding.mdc"), []byte("cursor\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, ".cursor", "rules", "superopen.mdc"), []byte("skip\n"), 0o644)

	found := FindExistingRules(root, "coding")
	if len(found) != 2 {
		t.Fatalf("rules=%v", found)
	}

	_ = os.MkdirAll(filepath.Join(root, ".codex", "skills", "deploy"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".codex", "skills", "deploy", "SKILL.md"), []byte("codex\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, ".pi", "skills", "deploy"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".pi", "skills", "deploy", "SKILL.md"), []byte("pi\n"), 0o644)
	skills := FindExistingSkills(root, "deploy")
	if len(skills) != 2 {
		t.Fatalf("skills=%v", skills)
	}
}

func TestVendorWeight(t *testing.T) {
	if VendorWeight("AGENTS.md", "claude-code") < VendorWeight(".cursor/rules/x.mdc", "claude-code") {
		t.Fatal("shared AGENTS should stay high vs other vendor")
	}
	if VendorWeight(".claude/rules/go.md", "claude-code") <= VendorWeight(".cursor/rules/x.mdc", "claude-code") {
		t.Fatal("matching vendor should beat other vendor")
	}
	if VendorWeight(".codex/skills/a/SKILL.md", "pi") >= VendorWeight(".pi/skills/a/SKILL.md", "pi") {
		t.Fatal("pi session should prefer pi skills")
	}
}

func TestRulesDirForVendor(t *testing.T) {
	root := t.TempDir()
	dir := RulesDirForVendor(root, "claude-code")
	if filepath.Base(filepath.Dir(dir)) != ".claude" {
		t.Fatalf("got %s", dir)
	}
}
