package headless

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAvailableFalseWithoutBinaries(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	isolateHome(t, t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, ok := Available(); ok {
		t.Fatal("expected no headless provider")
	}
}

func TestClaudeRequiresAuthFile(t *testing.T) {
	home := t.TempDir()
	binDir := t.TempDir()
	writeFakeBin(t, binDir, "claude")
	t.Setenv("PATH", binDir)
	isolateHome(t, home)
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

func isolateHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

func writeFakeBin(t *testing.T, dir, name string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}
