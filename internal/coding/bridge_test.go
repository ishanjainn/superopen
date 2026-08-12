package coding

import (
	"os"
	"path/filepath"
	"strings"
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

func TestInstallRemovesNetworkTelemetryConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := filepath.Join(home, ".config", "superopen", "config.env")
	if err := os.MkdirAll(filepath.Dir(cfg), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "# written by so init / so coding install\nSUPEROPEN_OTLP_ENDPOINT=https://collector.example\nOTEL_EXPORTER_OTLP_ENDPOINT=https://other.example\nOTEL_EXPORTER_OTLP_HEADERS=authorization=secret\nSUPEROPEN_API_KEY=secret\nSUPEROPEN_ENVIRONMENT=test\n"
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	auth := filepath.Join(filepath.Dir(cfg), "auth.json")
	if err := os.WriteFile(auth, []byte(`{"token":"obsolete"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeNetworkTelemetryConfig(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "OTLP") || strings.Contains(string(got), "SUPEROPEN_API_KEY") || !strings.Contains(string(got), "SUPEROPEN_ENVIRONMENT=test") {
		t.Fatalf("config.env migration = %q", got)
	}
	if _, err := os.Stat(auth); !os.IsNotExist(err) {
		t.Fatalf("obsolete auth file remains: %v", err)
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
