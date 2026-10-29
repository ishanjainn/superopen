package memory

import "testing"

func TestCaptureIsIdempotentAndRevivesForgotten(t *testing.T) {
	store, err := OpenRoot(testRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	in := CaptureInput{Kind: "knowledge", Source: SourceAgent, Title: "audit note", Text: "scratch note for the audit", Horizon: "medium"}
	first, err := store.Capture(in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Capture(in)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("a repeated capture must return the stored memory: %d vs %d", first.ID, second.ID)
	}
	if err := store.ForgetEpisode(first.ID); err != nil {
		t.Fatal(err)
	}
	again, err := store.Capture(in)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != first.ID || again.Faded {
		t.Fatalf("capturing forgotten text must revive it: %+v", again)
	}
}
