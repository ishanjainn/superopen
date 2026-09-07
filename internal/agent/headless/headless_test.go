package headless

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/session"
)

func TestAvailableLiveFalseWithoutBinaries(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	isolateHome(t, t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, ok := AvailableLive(""); ok {
		t.Fatal("expected no headless provider")
	}
	if _, ok := AvailableLive("claude-code"); ok {
		t.Fatal("claude-code without binary must be empty")
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

func TestHasOneShot(t *testing.T) {
	if !HasOneShot("claude-code") || !HasOneShot("cc") || !HasOneShot("opencode") || !HasOneShot("codex") || !HasOneShot("pi") {
		t.Fatal("one-shot vendors")
	}
	if HasOneShot("cursor") || HasOneShot("copilot-cli") || HasOneShot("gemini") {
		t.Fatal("cursor/copilot/gemini must not be one-shot")
	}
}

func TestAvailableLiveDoesNotFallThrough(t *testing.T) {
	home := t.TempDir()
	binDir := t.TempDir()
	writeFakeBin(t, binDir, "claude")
	writeFakeBin(t, binDir, "opencode")
	t.Setenv("PATH", binDir)
	isolateHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "opencode"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "opencode", "auth.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"oauthAccount":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	p, ok := AvailableLive("opencode")
	if !ok || p.Name != "opencode" {
		t.Fatalf("opencode live: %+v ok=%v", p, ok)
	}
	if _, ok := AvailableLive("cursor"); ok {
		t.Fatal("AvailableLive(cursor) must be empty")
	}
	if _, ok := AvailableLive(""); ok {
		t.Fatal("empty vendor must not fall through to another CLI")
	}
}

func TestWorkerFingerprint(t *testing.T) {
	if !WorkerFingerprint(WorkerHarvestPrefix+" x", "", "") {
		t.Fatal("harvest prefix")
	}
	if !WorkerFingerprint("", WorkerDistillPrefix+" x", "") {
		t.Fatal("distill prefix")
	}
	if !WorkerFingerprint("ok", "", "<synthetic>") {
		t.Fatal("synthetic model")
	}
	if WorkerFingerprint("fix login", "please fix login", "grok") {
		t.Fatal("real chat")
	}
}

func TestSessionEndPolicyCursorAwaitsLive(t *testing.T) {
	root := t.TempDir()
	if err := paths.Resolve(root).EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	store := session.NewStore(paths.Resolve(root))
	if err := store.Start(session.Meta{ID: "c1", Vendor: "cursor", StartedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	skipped, pending, spawn := SessionEndPolicy(root, "c1")
	if spawn || !pending || skipped != "await-live" {
		t.Fatalf("skipped=%s pending=%v spawn=%v", skipped, pending, spawn)
	}
}

func TestIsolatedEnv(t *testing.T) {
	t.Setenv(EnvIsolated, "1")
	if !Isolated() {
		t.Fatal("expected isolated")
	}
	skipped, _, spawn := SessionEndPolicy(t.TempDir(), "any")
	if spawn || skipped != "worker-env" {
		t.Fatalf("got skip=%s spawn=%v", skipped, spawn)
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
