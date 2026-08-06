package harvest_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/superopen/so/internal/config"
	"github.com/superopen/so/internal/harness"
	"github.com/superopen/so/internal/harvest"
	"github.com/superopen/so/internal/session"
)

func TestHarvestSkipEmpty(t *testing.T) {
	dir := t.TempDir()
	paths := harness.Resolve(dir)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	res, err := harvest.Run(paths, cfg, "missing", harvest.TriggerFinalize)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Skipped {
		t.Fatalf("expected skip, got %+v", res)
	}
}

func TestIdleSweepSkipsRecent(t *testing.T) {
	dir := t.TempDir()
	paths := harness.Resolve(dir)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	store := session.NewStore(paths)
	id := "ses_recent"
	_ = store.Start(session.Meta{ID: id, Vendor: "cursor", StartedAt: time.Now().UTC(), Status: session.StatusActive})
	_ = os.WriteFile(filepath.Join(paths.SessionDir(id), "transcript.jsonl"), []byte(`{"role":"user","text":"hi"}`+"\n"), 0o644)
	cfg := config.Default()
	cfg.Memory.IdleHarvestHours = 6
	results, err := harvest.IdleSweep(paths, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no idle harvest for recent session, got %+v", results)
	}
}

func TestPendingHarvestFlush(t *testing.T) {
	dir := t.TempDir()
	paths := harness.Resolve(dir)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	store := session.NewStore(paths)
	old := "ses_old"
	_ = store.Start(session.Meta{ID: old, Vendor: "codex", StartedAt: time.Now().Add(-time.Hour).UTC(), Status: session.StatusActive})
	_ = os.WriteFile(filepath.Join(paths.SessionDir(old), "transcript.jsonl"), []byte(`{"role":"user","text":"learned something"}`+"\n"), 0o644)
	if err := harvest.MarkPending(paths, old); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	results := harvest.FlushPending(paths, cfg, "ses_new")
	if len(results) != 1 || results[0].SessionID != old {
		t.Fatalf("expected flush of old session, got %+v", results)
	}
	// pending should be cleared after successful harvest / attempt
	if err := harvest.MarkPending(paths, old); err != nil {
		t.Fatal(err)
	}
	_ = harvest.ClearPending(paths, old)
}
