package coding

import (
	"os"
	"path/filepath"
	"testing"
)

func writeClaudeManifest(t *testing.T, home, soBin string) {
	t.Helper()
	dir := filepath.Join(home, ".claude", "plugins", "superopen-cc", "hooks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"` +
		soBin + ` coding hook --vendor=cc --event=SessionStart","timeout":5}]}]}}`
	if err := os.WriteFile(filepath.Join(dir, "hooks.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestStatusClaudeCodeDetectsStaleBinaryPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// A hooks.json pointing at a binary that doesn't exist (e.g. a Homebrew
	// `so` that was later uninstalled in favor of a different build) must not
	// report as installed - the hook is dead even though the plugin directory
	// and manifest are both present on disk.
	writeClaudeManifest(t, home, filepath.Join(home, "nonexistent-bin", "so"))
	got := Status(home, []string{"claude-code"})
	if got["claude-code"] {
		t.Fatal("expected claude-code status to be false when the hook's binary path does not exist")
	}
}

func TestStatusClaudeCodeOKWithValidBinaryPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	realBin := filepath.Join(home, "bin", "so")
	if err := os.MkdirAll(filepath.Dir(realBin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(realBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeClaudeManifest(t, home, realBin)

	got := Status(home, []string{"claude-code"})
	if !got["claude-code"] {
		t.Fatal("expected claude-code status to be true when the hook's binary path is executable")
	}
}

func TestStatusClaudeCodeMissingManifest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got := Status(home, []string{"claude-code"})
	if got["claude-code"] {
		t.Fatal("expected claude-code status to be false when no plugin is installed at all")
	}
}
