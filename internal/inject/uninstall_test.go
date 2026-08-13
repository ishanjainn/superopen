package inject

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUninstallRemovesSkillsAndInjectors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(home, ".agents", "skills", "so"), 0o755)
	_ = os.WriteFile(filepath.Join(home, ".agents", "skills", "so", "SKILL.md"), []byte("x"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, ".cursor", "skills", "so"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".cursor", "skills", "so", "SKILL.md"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("keep\n\n<!-- superopen:start -->\n## Superopen\n<!-- superopen:end -->\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, ".cursor", "rules"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".cursor", "rules", "superopen.mdc"), []byte("x"), 0o644)

	res, err := Uninstall(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Removed) == 0 {
		t.Fatal("expected removals")
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "so")); !os.IsNotExist(err) {
		t.Fatal("global skill should be gone")
	}
	if _, err := os.Stat(filepath.Join(root, ".cursor", "skills", "so")); !os.IsNotExist(err) {
		t.Fatal("project skill should be gone")
	}
	if _, err := os.Stat(filepath.Join(root, ".cursor", "rules", "superopen.mdc")); !os.IsNotExist(err) {
		t.Fatal("cursor rule should be gone")
	}
	agents, _ := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if strings.Contains(string(agents), "superopen:start") {
		t.Fatalf("inject left behind: %s", agents)
	}
	if !strings.Contains(string(agents), "keep") {
		t.Fatalf("should preserve non-inject content: %s", agents)
	}
}
