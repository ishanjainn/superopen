package harvest_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/harvest"
	"github.com/ishanjainn/superopen/internal/session"
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
	_ = os.WriteFile(filepath.Join(paths.SessionDir(id), "events.jsonl"), []byte(`{"role":"user","text":"hi"}`+"\n"), 0o644)
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
	_ = os.WriteFile(filepath.Join(paths.SessionDir(old), "events.jsonl"), []byte(`{"role":"user","text":"learned something"}`+"\n"), 0o644)
	if err := harvest.MarkPending(paths, old, "codex"); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	results := harvest.FlushPending(paths, cfg, "ses_new")
	if len(results) != 1 || results[0].SessionID != old {
		t.Fatalf("expected flush of old session, got %+v", results)
	}
	// pending should be cleared after successful harvest / attempt
	if err := harvest.MarkPending(paths, old, "codex"); err != nil {
		t.Fatal(err)
	}
	_ = harvest.ClearPending(paths, old)
}

func TestPendingVendorSelectsOnlyImmediatelyPriorSameVendor(t *testing.T) {
	paths := harness.Resolve(t.TempDir())
	_ = paths.EnsureDirs()
	store := session.NewStore(paths)
	now := time.Now().UTC()
	for _, meta := range []session.Meta{
		{ID: "codex-old", Vendor: "codex", StartedAt: now.Add(-3 * time.Hour)},
		{ID: "claude-latest", Vendor: "claude-code", StartedAt: now.Add(-2 * time.Hour)},
		{ID: "codex-latest", Vendor: "codex", StartedAt: now.Add(-time.Hour)},
	} {
		if err := store.Start(meta); err != nil {
			t.Fatal(err)
		}
	}
	_ = harvest.MarkPending(paths, "codex-old", "codex")
	_ = harvest.MarkPending(paths, "claude-latest", "claude-code")
	_ = harvest.MarkPending(paths, "codex-latest", "codex")
	if got := harvest.PendingVendor(paths, "codex-new", "codex"); got != "codex-latest" {
		t.Fatalf("got %q, want latest same-vendor session", got)
	}
	_ = store.WriteDocument("codex-latest", func(d *session.Document) { d.Review.Status = "complete" })
	if got := harvest.PendingVendor(paths, "codex-new", "codex"); got != "" {
		t.Fatalf("must not process older backlog, got %q", got)
	}
	if got := harvest.PendingVendor(paths, "claude-new", "claude-code"); got != "claude-latest" {
		t.Fatalf("cross-vendor selection failed: %q", got)
	}
}
