package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallOpenCodeAndPiUseHostDiscoveryPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// resolveSoBin looks on PATH — plant a fake so.
	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	so := filepath.Join(binDir, "so")
	if err := os.WriteFile(so, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// Seed stale wrong-path installs that used to fool doctor.
	staleOC := filepath.Join(home, ".opencode", "plugins", "superopen.ts")
	stalePi := filepath.Join(home, ".pi", "extensions", "superopen", "index.ts")
	for _, p := range []string{staleOC, stalePi} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("// stale\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writtenOC, err := installGenericVendor("opencode", false)
	if err != nil {
		t.Fatal(err)
	}
	wantOC := filepath.Join(home, ".config", "opencode", "plugins", "superopen.ts")
	if _, err := os.Stat(wantOC); err != nil {
		t.Fatalf("opencode plugin missing at host path %s: %v (wrote %v)", wantOC, err, writtenOC)
	}
	if _, err := os.Stat(staleOC); !os.IsNotExist(err) {
		t.Fatalf("stale ~/.opencode/plugins/superopen.ts should be removed")
	}
	body, err := os.ReadFile(wantOC)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "client.session.message") {
		t.Fatalf("opencode plugin should hydrate assistant parts via client.session.message")
	}

	writtenPi, err := installGenericVendor("pi", false)
	if err != nil {
		t.Fatal(err)
	}
	wantPi := filepath.Join(home, ".pi", "agent", "extensions", "superopen", "index.ts")
	if _, err := os.Stat(wantPi); err != nil {
		t.Fatalf("pi extension missing at host path %s: %v (wrote %v)", wantPi, err, writtenPi)
	}
	if _, err := os.Stat(filepath.Join(home, ".pi", "extensions", "superopen")); !os.IsNotExist(err) {
		t.Fatalf("stale ~/.pi/extensions/superopen should be removed")
	}
}
