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
