package harvest_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/agent/config"
	"github.com/ishanjainn/superopen/internal/harvest"
	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/retention"
)

func TestSweepDeletesOldHarvestSkipKeepsOpen(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := config.Save(map[string]string{
		config.EnvSessionRetentionHours: "168",
		config.EnvMemoryRetentionHours:  "168",
	}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	layout := paths.Resolve(root)
	if err := layout.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# Agents\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hstore, err := harvest.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	skipID, err := hstore.InsertRun("old-skip", harvest.StatusSkipped, "", "nothing to change")
	if err != nil {
		t.Fatal(err)
	}
	if err := hstore.BackdateRun(skipID, time.Now().UTC().Add(-10*24*time.Hour)); err != nil {
		hstore.Close()
		t.Fatal(err)
	}
	open, err := hstore.InsertProposal(harvest.Proposal{
		SessionID: "live", Status: harvest.StatusOpen, Kind: harvest.KindImprove, Target: "AGENTS.md",
		Title: "still open", Reason: "waiting",
	})
	if err != nil {
		hstore.Close()
		t.Fatal(err)
	}
	hstore.Close()

	out, err := retention.Sweep(root)
	if err != nil {
		t.Fatal(err)
	}
	if out.HarvestDeleted < 1 {
		t.Fatalf("expected harvest history deleted, got %+v", out)
	}
	hstore, err = harvest.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer hstore.Close()
	if _, err := hstore.GetProposal(open.ID); err != nil {
		t.Fatal("open proposal must remain")
	}
	items, err := hstore.ListHistory(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Source == "run" && it.SessionID == "old-skip" {
			t.Fatal("10-day-old skip should be gone")
		}
	}
}
