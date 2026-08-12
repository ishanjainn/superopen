package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallCommandRegistersCodexHooks(t *testing.T) {
	home := t.TempDir()
	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"so", "codex", "graphify"} {
		path := filepath.Join(binDir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir)

	cmd := cmdInstall()
	cmd.SetArgs([]string{"--global", "--vendor", "codex"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	hooks := filepath.Join(home, ".local", "share", "superopen", "codex-marketplace", "plugins", "superopen", "hooks", "hooks.json")
	body, err := os.ReadFile(hooks)
	if err != nil {
		t.Fatalf("Codex hooks were not installed by so install: %v", err)
	}
	if !strings.Contains(string(body), "coding hook --vendor=codex") {
		t.Fatalf("Codex hook manifest does not invoke Superopen: %s", body)
	}

	marketplace := filepath.Join(home, ".local", "share", "superopen", "codex-marketplace", ".agents", "plugins", "marketplace.json")
	if _, err := os.Stat(marketplace); err != nil {
		t.Fatalf("Codex marketplace manifest was not installed: %v", err)
	}
}
