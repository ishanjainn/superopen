package retention_test

import (
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/audit"
	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/retention"
	"github.com/ishanjainn/superopen/internal/session"
)

func TestPruneEmptyAndExpired(t *testing.T) {
	root := t.TempDir()
	paths := harness.Resolve(root)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	ss := session.NewStore(paths)

	old := time.Now().UTC().AddDate(0, 0, -10)
	ended := old
	if err := ss.Start(session.Meta{
		ID:            "keep-recent",
		Status:        session.StatusActive,
		PromptPreview: "recent work",
		StartedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := ss.Start(session.Meta{
		ID:        "empty-now",
		Status:    session.StatusActive,
		StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := ss.Start(session.Meta{
		ID:            "expired",
		Status:        session.StatusEnded,
		PromptPreview: "old work",
		StartedAt:     old,
		EndedAt:       &ended,
	}); err != nil {
		t.Fatal(err)
	}

	_ = audit.Append(paths, audit.Event{At: old, Action: "deny", Detail: "old"})
	_ = audit.Append(paths, audit.Event{At: time.Now().UTC(), Action: "allow", Detail: "new"})
	mem := memory.NewStore(paths)
	_, _ = mem.UpsertPattern(memory.Pattern{Fingerprint: "pattern-retention", Vendor: "codex", Kind: "workflow", Summary: "Retain aggregate evidence."}, "expired", true)
	_, _ = mem.UpsertPattern(memory.Pattern{Fingerprint: "pattern-retention", Vendor: "codex", Kind: "workflow", Summary: "Retain aggregate evidence."}, "keep-recent", false)

	cfg := config.Default()
	cfg.Retention.Days = 7
	rep, err := retention.Prune(paths, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if rep.EmptySessions < 1 {
		t.Fatalf("expected empty prune, got %+v", rep)
	}
	if rep.ExpiredSessions < 1 {
		t.Fatalf("expected expired prune, got %+v", rep)
	}
	if _, err := ss.Get("keep-recent"); err != nil {
		t.Fatal("keep-recent deleted")
	}
	if _, err := ss.Get("empty-now"); err == nil {
		t.Fatal("empty-now should be deleted")
	}
	if _, err := ss.Get("expired"); err == nil {
		t.Fatal("expired should be deleted")
	}
	if rep.AuditEvents < 1 {
		t.Fatalf("expected old system audit event prune, got %+v", rep)
	}
	patterns, err := mem.ListPatterns()
	if err != nil || len(patterns) != 1 || patterns[0].Occurrences != 2 || len(patterns[0].SessionIDs) != 1 || patterns[0].SessionIDs[0] != "keep-recent" {
		t.Fatalf("retention should remove expired references but keep aggregate counts: %+v err=%v", patterns, err)
	}
}
