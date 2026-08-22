package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/paths"
)

func TestDeleteOlderThanKeepsRecentAndActiveFiles(t *testing.T) {
	root := t.TempDir()
	layout := paths.Resolve(root)
	if err := layout.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	store := NewStore(layout)
	oldStart := time.Now().UTC().Add(-10 * 24 * time.Hour)
	if err := store.Start(Meta{ID: "old", Vendor: "cursor", StartedAt: oldStart}); err != nil {
		t.Fatal(err)
	}
	ended := oldStart.Add(time.Hour)
	oldMeta, err := store.Get("old")
	if err != nil {
		t.Fatal(err)
	}
	oldMeta.Status = StatusEnded
	oldMeta.EndedAt = &ended
	oldMeta.StartedAt = oldStart
	if err := store.UpdateMeta(oldMeta); err != nil {
		t.Fatal(err)
	}
	oldJSON := filepath.Join(layout.SessionDir("old"), "session.json")
	oldTime := oldStart
	if err := os.Chtimes(oldJSON, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	if err := store.Start(Meta{ID: "fresh", Vendor: "cursor", StartedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}

	cutoff := time.Now().UTC().Add(-7 * 24 * time.Hour)
	deleted, err := store.DeleteOlderThan(cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0] != "old" {
		t.Fatalf("deleted=%v", deleted)
	}
	if _, err := store.Get("fresh"); err != nil {
		t.Fatal("fresh session must remain")
	}
	if _, err := store.Get("old"); err == nil {
		t.Fatal("old session must be gone")
	}
}

func TestDeleteOlderThanZeroCutoffIsNoop(t *testing.T) {
	root := t.TempDir()
	layout := paths.Resolve(root)
	if err := layout.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	store := NewStore(layout)
	if err := store.Start(Meta{ID: "keep", Vendor: "cursor", StartedAt: time.Now().Add(-30 * 24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	deleted, err := store.DeleteOlderThan(time.Time{})
	if err != nil || len(deleted) != 0 {
		t.Fatalf("deleted=%v err=%v", deleted, err)
	}
}
