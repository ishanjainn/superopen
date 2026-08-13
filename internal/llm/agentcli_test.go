package llm

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ishanjainn/superopen/internal/config"
)

func TestAutoDoesNotUseAmbientAPIKeyWithoutProjectOptIn(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("SUPEROPEN_LLM_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", "")

	cfg := config.Default()
	if got := NewCompleterForBackend(cfg, "auto"); got != nil {
		t.Fatalf("auto selected unconfigured API backend: %T", got)
	}
	if got := NewCompleterForBackend(cfg, "llm_api"); got == nil {
		t.Fatal("explicit llm_api backend should use the ambient key")
	}
	cfg.LLM.Provider = "openai"
	if got := NewCompleterForBackend(cfg, "auto"); got == nil {
		t.Fatal("explicit llm config should enable API fallback")
	}
}

func TestPreferCLIForVendor(t *testing.T) {
	cases := map[string]string{
		"claude-code": "claude",
		"codex":       "codex",
		"opencode":    "opencode",
		"pi":          "pi",
		"cursor":      "",
		"gemini":      "",
		"copilot-cli": "",
	}
	for vendor, want := range cases {
		if got := preferCLIForVendor(vendor); got != want {
			t.Fatalf("%s: got %q want %q", vendor, got, want)
		}
	}
}

func TestVendorCompleterPrefersMatchingCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub CLIs are shell scripts")
	}
	dir := t.TempDir()
	for _, name := range []string{"claude", "opencode", "pi"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)

	cfg := config.Default()
	got := NewVendorCompleter(cfg, "opencode")
	if got == nil || got.Backend() != "agent_cli:opencode" {
		t.Fatalf("opencode session: %#v", got)
	}
	got = NewVendorCompleter(cfg, "pi")
	if got == nil || got.Backend() != "agent_cli:pi" {
		t.Fatalf("pi session: %#v", got)
	}
	got = NewVendorCompleter(cfg, "claude-code")
	if got == nil || got.Backend() != "agent_cli:claude" {
		t.Fatalf("claude session: %#v", got)
	}
	got = NewVendorCompleter(cfg, "cursor")
	if got == nil || got.Backend() != "agent_cli:claude" {
		t.Fatalf("cursor fallback should be first Supported CLI, got %#v", got)
	}
}
