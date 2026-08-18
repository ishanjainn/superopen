package codex

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFindRolloutHonorsCodexHome(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CODEX_HOME", root)
	sessionID := "session-portability-test"
	dir := filepath.Join(root, "sessions", time.Now().UTC().Format("2006/01/02"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "rollout-test.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"session_meta","payload":{"id":"`+sessionID+`"}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := findRolloutForSession(sessionID); got != path {
		t.Fatalf("findRolloutForSession() = %q, want %q", got, path)
	}
}
