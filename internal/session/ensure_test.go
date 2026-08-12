package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/harness"
)

func TestEnsureRepairsEventStreamWithoutSessionDocument(t *testing.T) {
	paths := harness.Resolve(t.TempDir())
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	id := "orphan-events"
	events := filepath.Join(paths.SessionDir(id), "events.jsonl")
	if err := os.MkdirAll(filepath.Dir(events), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"type":"superopen.file_manifest"}` + "\n" +
		`{"at":"2026-08-12T17:29:46Z","action":"allow","vendor":"codex","session_id":"orphan-events"}` + "\n"
	if err := os.WriteFile(events, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	store := NewStore(paths)
	if err := store.Ensure(); err != nil {
		t.Fatal(err)
	}
	doc, err := store.ReadDocument(id)
	if err != nil {
		t.Fatal(err)
	}
	if doc.ID != id || doc.Vendor != "codex" || doc.Status != StatusActive {
		t.Fatalf("repaired document = %#v", doc.Meta)
	}
	entries, err := store.List()
	if err != nil || len(entries) != 1 || entries[0].ID != id {
		t.Fatalf("repaired index = %#v, %v", entries, err)
	}
}
