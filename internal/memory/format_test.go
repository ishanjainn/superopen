package memory

import (
	"strings"
	"testing"
)

func TestFormatIndexLineOpaqueTitleUsesBodyHeadline(t *testing.T) {
	line := FormatIndexLine(Episode{
		ID:    9,
		Kind:  KindSession,
		Title: "sharegpt_yywfIrx_0",
		Text:  "I work at a small bakery in Malmo. We open at six.",
	})
	if !strings.Contains(line, "#9") {
		t.Fatalf("must still cite #id, got %q", line)
	}
	if !strings.Contains(line, "bakery") {
		t.Fatalf("opaque title should show body headline, got %q", line)
	}
	if strings.Contains(line, "sharegpt_yywfIrx_0") {
		t.Fatalf("opaque import id should not be the headline, got %q", line)
	}

	uuidLine := FormatIndexLine(Episode{
		ID:    4,
		Kind:  KindSession,
		Title: "550e8400-e29b-41d4-a716-446655440000",
		Text:  "The playlist is called Night Drive.",
	})
	if !strings.Contains(uuidLine, "Night Drive") {
		t.Fatalf("uuid title should show body headline, got %q", uuidLine)
	}

	colon := FormatIndexLine(Episode{
		ID:    5,
		Kind:  KindSession,
		Title: "conv-26:session_1",
		Text:  "Caroline bought the coffee maker at Target last June.",
	})
	if !strings.Contains(colon, "coffee maker") {
		t.Fatalf("session-id title should show body headline, got %q", colon)
	}
}

func TestFormatIndexLineKeepsHumanTitle(t *testing.T) {
	line := FormatIndexLine(Episode{ID: 12, Kind: KindPrompt, Title: "fix login", Tokens: 192})
	if !strings.Contains(line, "fix login") {
		t.Fatalf("human title must stay, got %q", line)
	}
}

func TestIndexFromEpisodeUsesDisplayHeadline(t *testing.T) {
	idx := IndexFromEpisode(Episode{
		ID:    2,
		Kind:  KindSession,
		Title: "ultrachat_231069",
		Text:  "My commute is forty minutes on the bus.",
	})
	if !strings.Contains(idx.Title, "commute") {
		t.Fatalf("TOON search title should be the headline, got %q", idx.Title)
	}
	if strings.Contains(idx.Title, "ultrachat_231069") {
		t.Fatalf("stored import id must not be the display title, got %q", idx.Title)
	}
}

func TestCaptureDoesNotRewriteOpaqueTitle(t *testing.T) {
	root := testRoot(t)
	store, err := OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ep, err := store.Capture(CaptureInput{
		Kind:  KindSession,
		Title: "sharegpt_keep_me",
		Text:  "I bought the kettle at the hardware store on Main Street.",
		Topic: ObservationDecision,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "sharegpt_keep_me" {
		t.Fatalf("stored title must stay the import id, got %q", got.Title)
	}
	line := FormatIndexLine(got)
	if !strings.Contains(line, "kettle") || strings.Contains(line, "sharegpt_keep_me") {
		t.Fatalf("display headline should come from the body, got %q", line)
	}
}
