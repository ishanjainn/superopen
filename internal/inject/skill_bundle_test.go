package inject

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteSkillBundle_ProjectVendors(t *testing.T) {
	root := t.TempDir()
	body := "# /so\ntest skill\n"
	written, err := writeSkillBundleFor(root, body, []string{"codex"}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(root, ".codex", "skills", "so", "SKILL.md"),
	}
	if len(written) != len(want) {
		t.Fatalf("wrote %d paths, want %d: %v", len(written), len(want), written)
	}
	for _, p := range want {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("missing %s: %v", p, err)
		}
		if string(data) != body {
			t.Fatalf("unexpected body in %s", p)
		}
	}
}

func TestWriteSkillBundle_GlobalVendors(t *testing.T) {
	home := t.TempDir()
	// The explicit home argument is the install target under test. Do not let a
	// runner-level XDG override redirect OpenCode writes outside the fixture.
	t.Setenv("XDG_CONFIG_HOME", "")
	body := "# /so\nglobal\n"
	written, err := writeSkillBundleFor(home, body, []string{"opencode", "copilot-cli", "pi", "gemini"}, false, true)
	if err != nil {
		t.Fatal(err)
	}
	extra := []string{
		filepath.Join(home, ".config", "opencode", "skills", "so", "SKILL.md"),
		filepath.Join(home, ".copilot", "skills", "so", "SKILL.md"),
		filepath.Join(home, ".pi", "agent", "skills", "so", "SKILL.md"),
	}
	for _, p := range extra {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("missing global path %s (written=%v): %v", p, written, err)
		}
	}
	// Gemini global is the same as project-style under $HOME.
	if _, err := os.Stat(filepath.Join(home, ".gemini", "skills", "so", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveSkillBundle_RemovesNewVendors(t *testing.T) {
	root := t.TempDir()
	// Installation detection intentionally depends on the host. Removal tests
	// instead seed every project integration explicitly so a clean CI runner and
	// a developer machine exercise exactly the same files.
	if _, err := writeSkillBundleFor(root, "x", []string{"gemini", "opencode", "copilot-cli", "pi"}, false, false); err != nil {
		t.Fatal(err)
	}
	removed := removeSkillBundle(root, false)
	if len(removed) == 0 {
		t.Fatal("expected removals")
	}
	for _, rel := range []string{
		".gemini/skills/so",
		".opencode/skills/so",
		".github/skills/so",
		".pi/skills/so",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("%s should be gone", rel)
		}
	}
}

func TestCheckedInSkillCopiesMatchEmbedded(t *testing.T) {
	repo := filepath.Clean(filepath.Join("..", ".."))
	for _, rel := range []string{
		".claude/skills/so/SKILL.md", ".cursor/skills/so/SKILL.md", ".codex/skills/so/SKILL.md",
		".gemini/skills/so/SKILL.md", ".opencode/skills/so/SKILL.md", ".github/skills/so/SKILL.md", ".pi/skills/so/SKILL.md",
	} {
		data, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if string(data) != embeddedSkillMD {
			t.Fatalf("%s drifted from internal/inject/skill.md", rel)
		}
	}
}

func TestEmbeddedSkillContainsLiveAgentReview(t *testing.T) {
	for _, needle := range []string{
		"so review-brief",
		"so apply-review",
		"Do **not** ask for an API key",
		"live coding agent",
		"Never finish a review with heuristics",
	} {
		if !strings.Contains(embeddedSkillMD, needle) {
			t.Fatalf("skill.md missing %q", needle)
		}
	}
}

func TestAgentGuidanceDistinguishesChatFromShellSyntax(t *testing.T) {
	for _, needle := range []string{
		"always use `so ...` with no leading slash",
		"Never execute `/so ...` as a filesystem path",
	} {
		if !strings.Contains(embeddedSkillMD, needle) {
			t.Fatalf("skill.md missing shell-syntax warning %q", needle)
		}
	}
	for _, needle := range []string{
		"always run `so ...` with no leading slash",
		"Never type `/so ...` into Bash",
	} {
		if !strings.Contains(Brief(), needle) {
			t.Fatalf("generated agent brief missing shell-syntax warning %q", needle)
		}
	}
}
