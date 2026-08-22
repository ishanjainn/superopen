package memory

import (
	"testing"
	"time"
)

func TestDeleteExpiredKeepsTeachingPinAndRecent(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	oldPrompt, err := store.Capture(CaptureInput{Kind: KindPrompt, Title: "old prompt about login timeout", Text: "old prompt about login timeout"})
	if err != nil {
		t.Fatal(err)
	}
	oldTeach, err := store.Capture(CaptureInput{Kind: KindTeaching, Title: "keep this teaching about sqlite", Text: "keep this teaching about sqlite"})
	if err != nil {
		t.Fatal(err)
	}
	pinned, err := store.Capture(CaptureInput{Kind: KindPrompt, Title: "pinned prompt about cookies", Text: "pinned prompt about cookies", Pin: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Pin(pinned.ID); err != nil {
		t.Fatal(err)
	}
	fresh, err := store.Capture(CaptureInput{Kind: KindPrompt, Title: "fresh prompt about layout", Text: "fresh prompt about layout"})
	if err != nil {
		t.Fatal(err)
	}

	old := time.Now().UTC().Add(-10 * 24 * time.Hour).Format(time.RFC3339Nano)
	if _, err := store.db.Exec(`UPDATE memory_episodes SET created_at=? WHERE id IN (?,?,?)`, old, oldPrompt.ID, oldTeach.ID, pinned.ID); err != nil {
		t.Fatal(err)
	}

	n, err := store.DeleteExpired(time.Now().UTC().Add(-7 * 24 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("deleted=%d want 1 (old prompt only)", n)
	}
	if _, err := store.Get(oldPrompt.ID); err == nil {
		t.Fatal("old prompt should be deleted")
	}
	if _, err := store.Get(oldTeach.ID); err != nil {
		t.Fatal("teaching must remain")
	}
	if _, err := store.Get(pinned.ID); err != nil {
		t.Fatal("pin must remain")
	}
	if _, err := store.Get(fresh.ID); err != nil {
		t.Fatal("fresh prompt must remain")
	}
}

func TestDeleteUnprotectedForSessions(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	prompt, err := store.Capture(CaptureInput{SessionID: "s1", Kind: KindPrompt, Title: "session prompt about auth cookies", Text: "session prompt about auth cookies"})
	if err != nil {
		t.Fatal(err)
	}
	teach, err := store.Capture(CaptureInput{SessionID: "s1", Kind: KindTeaching, Title: "teaching still in sqlite", Text: "teaching still in sqlite"})
	if err != nil {
		t.Fatal(err)
	}
	n, err := store.DeleteUnprotectedForSessions([]string{"s1"})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("deleted=%d", n)
	}
	if _, err := store.Get(prompt.ID); err == nil {
		t.Fatal("session prompt should be deleted")
	}
	if _, err := store.Get(teach.ID); err != nil {
		t.Fatal("teaching must remain")
	}
}
