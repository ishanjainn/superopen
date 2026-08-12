package eval

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/session"
)

func writeEval(t *testing.T, paths harness.Paths, id string, at time.Time) {
	t.Helper()
	res := Result{SessionID: id, At: at, Badge: "ok", Score: 0.5}
	data, _ := json.Marshal(res)
	store := session.NewStore(paths)
	if err := store.Start(session.Meta{ID: id, StartedAt: at}); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteDocument(id, func(d *session.Document) { d.Evaluation = data }); err != nil {
		t.Fatal(err)
	}
}

func TestDecideSkipEndedFinal(t *testing.T) {
	paths := harness.Resolve(t.TempDir())
	_ = paths.EnsureDirs()
	ended := time.Now().UTC().Add(-time.Hour)
	writeEval(t, paths, "s1", ended.Add(time.Minute))
	meta := session.Meta{ID: "s1", Status: session.StatusEnded, EndedAt: &ended}
	d := DecideSkip(paths, config.Config{}, meta, false)
	if !d.Skip || d.Reason != "ended_final" {
		t.Fatalf("got skip=%v reason=%q", d.Skip, d.Reason)
	}
	if DecideSkip(paths, config.Config{}, meta, true).Skip {
		t.Fatal("force should not skip")
	}
}

func TestDecideSkipActiveCooldown(t *testing.T) {
	paths := harness.Resolve(t.TempDir())
	_ = paths.EnsureDirs()
	writeEval(t, paths, "s2", time.Now().UTC().Add(-30*time.Minute))
	meta := session.Meta{ID: "s2", Status: session.StatusActive}
	cfg := config.Config{Evals: config.EvalsConfig{ActiveCooldownHours: 6}}
	d := DecideSkip(paths, cfg, meta, false)
	if !d.Skip || d.Reason != "active_cooldown" {
		t.Fatalf("got skip=%v reason=%q", d.Skip, d.Reason)
	}
	writeEval(t, paths, "s2", time.Now().UTC().Add(-7*time.Hour))
	d = DecideSkip(paths, cfg, meta, false)
	if d.Skip {
		t.Fatalf("cooldown elapsed should allow eval, got %+v", d)
	}
}
