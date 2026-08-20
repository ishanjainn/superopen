package headless

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAvailableFalseWithoutBinaries(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, ok := Available(); ok {
		t.Fatal("expected no headless provider")
	}
}

func TestClaudeRequiresAuthFile(t *testing.T) {
	home := t.TempDir()
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "claude"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("HOME", home)
	if _, ok := claude(); ok {
		t.Fatal("claude on PATH without oauth should be unavailable")
	}
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"oauthAccount":{"emailAddress":"dev@example.com"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	p, ok := claude()
	if !ok || p.Name != "claude" {
		t.Fatalf("got %+v ok=%v", p, ok)
	}
}
