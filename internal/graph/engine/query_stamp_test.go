package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestQueryStampFreshWithinTTL(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".so", "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	if QueryStampFresh(repo) {
		t.Fatal("expected no stamp")
	}
	RecordQueryStamp(repo)
	if !QueryStampFresh(repo) {
		t.Fatal("expected fresh stamp")
	}
	t.Setenv(strictTTLEnv, "0.01")
	time.Sleep(30 * time.Millisecond)
	if QueryStampFresh(repo) {
		t.Fatal("expected expired stamp")
	}
}

func TestQueryStampFreshIsPerSession(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".so", "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	RecordQueryStampFor(repo, "session-a")
	if !QueryStampFreshFor(repo, "session-a") {
		t.Fatal("session-a should be fresh")
	}
	if QueryStampFreshFor(repo, "session-b") {
		t.Fatal("session-b must not inherit session-a's stamp")
	}
	if QueryStampFresh(repo) {
		t.Fatal("repo-wide stamp must stay empty when only a session stamp was written")
	}
}
