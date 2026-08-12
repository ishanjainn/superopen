package inject

import (
	"os"
	"path/filepath"
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
	if _, err := writeSkillBundle(root, "x", false); err != nil {
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
