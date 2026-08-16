package graph

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/harness"
)

func versionBinary(t *testing.T, version string) string {
	t.Helper()
	name, body := "graphify", "#!/bin/sh\necho 'graphify "+version+"'\n"
	if runtime.GOOS == "windows" {
		name = "graphify.cmd"
		body = "@echo off\r\necho graphify " + version + "\r\n"
	}
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExplicitGraphifyOverrideRequiresExactPin(t *testing.T) {
	t.Setenv("SUPEROPEN_GRAPHIFY_BIN", versionBinary(t, "0.9.43"))
	if _, err := resolveGraphifyBin(); err == nil || !strings.Contains(err.Error(), PinnedVersion) {
		t.Fatalf("expected exact-pin rejection, got %v", err)
	}
	t.Setenv("SUPEROPEN_GRAPHIFY_BIN", versionBinary(t, PinnedVersion))
	if _, err := resolveGraphifyBin(); err != nil {
		t.Fatal(err)
	}
}

func TestMissingRuntimeNeverPublishesStub(t *testing.T) {
	repo := t.TempDir()
	paths := harness.Resolve(repo)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SUPEROPEN_GRAPHIFY_BIN", "")
	t.Setenv("PATH", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if _, err := Build(repo, paths, true, ""); err == nil {
		t.Fatal("expected missing runtime failure")
	}
	if _, err := os.Stat(paths.GraphJSON); !os.IsNotExist(err) {
		t.Fatalf("fresh failure published graph.json: %v", err)
	}
}

func TestLegacyStubStateIsRejected(t *testing.T) {
	repo := t.TempDir()
	paths := harness.Resolve(repo)
	_ = paths.EnsureDirs()
	_ = os.WriteFile(paths.GraphJSON, []byte(`{"nodes":[{"id":"dir"},{"id":"file"}],"edges":[{"source":"dir","target":"file"}]}`), 0o644)
	_ = os.WriteFile(paths.GraphState, []byte(`{"schema_version":2,"source":"stub"}`), 0o644)
	if err := ValidateQueryableGraph(repo); err == nil || !strings.Contains(err.Error(), "legacy stub") {
		t.Fatalf("got %v", err)
	}
}

func TestManagedInstallSmoke(t *testing.T) {
	if os.Getenv("SUPEROPEN_GRAPHIFY_INSTALL_TEST") != "1" {
		t.Skip("set SUPEROPEN_GRAPHIFY_INSTALL_TEST=1 for the release-gate installation smoke test")
	}
	t.Setenv("SUPEROPEN_GRAPHIFY_BIN", "")
	if err := EnsureTool(); err != nil {
		t.Fatal(err)
	}
	status := Status()
	if !status.Available || status.Version != PinnedVersion || !status.Managed || !status.ModuleOK || !status.ConsoleOK {
		t.Fatalf("unexpected managed toolchain status: %+v", status)
	}
	_, _, console, err := managedPaths()
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		body, err := os.ReadFile(console)
		if err != nil {
			t.Fatal(err)
		}
		first, _, _ := strings.Cut(string(body), "\n")
		if strings.Contains(first, ".install-") {
			t.Fatalf("published console script retains staging shebang: %s", first)
		}
	}
	for name, state := range status.Extras {
		if state != "available" && state != "not_applicable" {
			t.Errorf("extra %s = %s", name, state)
		}
	}
}
