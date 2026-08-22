package retention

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/agent/config"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/session"
)

func TestSweepDeletesOldSessionAndPromptKeepsTeaching(t *testing.T) {
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
	store := session.NewStore(layout)
	oldStart := time.Now().UTC().Add(-10 * 24 * time.Hour)
	if err := store.Start(session.Meta{ID: "old-ses", Vendor: "cursor", StartedAt: oldStart}); err != nil {
		t.Fatal(err)
	}
	meta, err := store.Get("old-ses")
	if err != nil {
		t.Fatal(err)
	}
	ended := oldStart.Add(time.Hour)
	meta.Status = session.StatusEnded
	meta.EndedAt = &ended
	meta.StartedAt = oldStart
	if err := store.UpdateMeta(meta); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(layout.SessionDir("old-ses"), "session.json"), oldStart, oldStart); err != nil {
		t.Fatal(err)
	}

	mem, err := memory.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer mem.Close()
	prompt, err := mem.Capture(memory.CaptureInput{
		SessionID: "old-ses",
		Kind:      memory.KindPrompt,
		Title:     "old session prompt about login timeout",
		Text:      "old session prompt about login timeout",
	})
	if err != nil {
		t.Fatal(err)
	}
	teach, err := mem.Capture(memory.CaptureInput{
		SessionID: "old-ses",
		Kind:      memory.KindTeaching,
		Title:     "keep teaching about sqlite diary",
		Text:      "keep teaching about sqlite diary",
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := Sweep(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.SessionsDeleted) != 1 || out.SessionsDeleted[0] != "old-ses" {
		t.Fatalf("sessions=%v", out.SessionsDeleted)
	}
	if _, err := store.Get("old-ses"); err == nil {
		t.Fatal("session should be gone")
	}
	if _, err := mem.Get(prompt.ID); err == nil {
		t.Fatal("session prompt should be gone")
	}
	if _, err := mem.Get(teach.ID); err != nil {
		t.Fatal("teaching must remain")
	}
}

func TestSweepZeroHoursKeepsEverything(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := config.Save(map[string]string{
		config.EnvSessionRetentionHours: "0",
		config.EnvMemoryRetentionHours:  "0",
	}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	layout := paths.Resolve(root)
	if err := layout.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	store := session.NewStore(layout)
	oldStart := time.Now().UTC().Add(-40 * 24 * time.Hour)
	if err := store.Start(session.Meta{ID: "keep", Vendor: "cursor", StartedAt: oldStart}); err != nil {
		t.Fatal(err)
	}
	out, err := Sweep(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.SessionsDeleted) != 0 || out.MemoriesDeleted != 0 {
		t.Fatalf("expected noop: %+v", out)
	}
	if _, err := store.Get("keep"); err != nil {
		t.Fatal(err)
	}
}
