package steer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDurableTargetsWritesClaudeConfigDir(t *testing.T) {
	home := t.TempDir()
	cfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	t.Setenv("HOME", home)
	got := durableTargets(home)
	wantHome := filepath.Join(home, ".claude", "CLAUDE.md")
	wantCfg := filepath.Join(cfg, "CLAUDE.md")
	var haveHome, haveCfg bool
	for _, p := range got {
		if p == wantHome {
			haveHome = true
		}
		if p == wantCfg {
			haveCfg = true
		}
	}
	if !haveHome || !haveCfg {
		t.Fatalf("durableTargets missing Claude paths: home=%v cfg=%v got=%v", haveHome, haveCfg, got)
	}
}

func TestInstallProjectCursorRule(t *testing.T) {
	root := t.TempDir()
	path, err := InstallProjectCursorRule(root)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, ".cursor", "rules", "superopen.mdc")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "ignore Superopen entirely") {
		t.Fatalf("missing gate: %s", body)
	}
	if !strings.Contains(string(body), "alwaysApply: true") {
		t.Fatalf("expected alwaysApply: %s", body)
	}
	removed := RemoveProjectCursorRule(root)
	if removed != want {
		t.Fatalf("removed = %q, want %q", removed, want)
	}
	if _, err := os.Stat(want); !os.IsNotExist(err) {
		t.Fatal("expected rule file gone")
	}
}
