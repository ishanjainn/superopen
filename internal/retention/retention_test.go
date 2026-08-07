package retention_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/audit"
	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/eval"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/recommend"
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

	hist := []eval.Result{
		{SessionID: "a", At: old, Badge: "ok"},
		{SessionID: "b", At: time.Now().UTC(), Badge: "good"},
	}
	hdata, _ := json.MarshalIndent(hist, "", "  ")
	_ = os.WriteFile(paths.EvalsHistory, hdata, 0o644)

	_ = audit.Append(paths, audit.Event{At: old, Action: "deny", Detail: "old"})
	_ = audit.Append(paths, audit.Event{At: time.Now().UTC(), Action: "allow", Detail: "new"})

	_ = recommend.SavePending(paths, []recommend.Recommendation{
		{ID: "old-rec", Title: "old", Status: "pending", CreatedAt: old},
		{ID: "new-rec", Title: "new", Status: "pending", CreatedAt: time.Now().UTC()},
	})

	oldTrace := filepath.Join(paths.TracesDir, old.Format("2006-01-02")+".jsonl")
	_ = os.WriteFile(oldTrace, []byte("{}\n"), 0o644)
	newTrace := filepath.Join(paths.TracesDir, time.Now().UTC().Format("2006-01-02")+".jsonl")
	_ = os.WriteFile(newTrace, []byte("{}\n"), 0o644)

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
	if rep.EvalHistory < 1 || rep.AuditEvents < 1 || rep.Recommendations < 1 || rep.TraceFiles < 1 {
		t.Fatalf("expected history/audit/recs/trace prune, got %+v", rep)
	}
}
