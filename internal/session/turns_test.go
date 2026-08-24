package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/session/trace"
)

func TestCountTurnsFromSpansSameTurnID(t *testing.T) {
	spans := []trace.Span{
		{Name: "coding_agent.session.loop.stop", Attributes: map[string]string{"coding_agent.turn.id": "t1"}},
		{Name: "coding_agent.llm.turn", Attributes: map[string]string{"coding_agent.turn.id": "t1"}},
	}
	if n := CountTurnsFromSpans(spans); n != 1 {
		t.Fatalf("same turn id: got %d want 1", n)
	}
}

func TestCountTurnsFromSpansLoopStopWithoutID(t *testing.T) {
	spans := []trace.Span{
		{Name: "coding_agent.session.loop.stop"},
		{Name: "coding_agent.llm.turn"},
		{Name: "coding_agent.user_prompt.submit", Attributes: map[string]string{"gen_ai.prompt": "hi"}},
	}
	if n := CountTurnsFromSpans(spans); n != 1 {
		t.Fatalf("loop.stop wins without ids: got %d want 1", n)
	}
}

func TestMaterializePersistsTurns(t *testing.T) {
	root := t.TempDir()
	layout := paths.Resolve(root)
	if err := layout.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	store := NewStore(layout)
	id := "turn-ses"
	if err := store.Start(Meta{ID: id, Vendor: "cursor", Status: StatusActive, StartedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	spans := []trace.Span{
		{Name: "coding_agent.session.loop.stop", Attributes: map[string]string{"coding_agent.turn.id": "t1"}},
		{Name: "coding_agent.llm.turn", Attributes: map[string]string{"coding_agent.turn.id": "t1"}},
	}
	meta, err := store.MaterializeFromSpans(id, spans, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Turns != 1 {
		t.Fatalf("materialize Turns=%d want 1", meta.Turns)
	}
	got, err := store.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Turns != 1 {
		t.Fatalf("persisted Turns=%d want 1", got.Turns)
	}
}

func TestTurnsForNestedEventsFile(t *testing.T) {
	root := t.TempDir()
	layout := paths.Resolve(root)
	if err := layout.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	store := NewStore(layout)
	child := "nested-child"
	if err := store.Start(Meta{
		ID: child, Vendor: "cursor", Status: StatusActive, ParentID: "parent",
		IsSubagent: true, StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	events := filepath.Join(layout.SessionDir(child), "events.jsonl")
	body := `{"name":"coding_agent.session.loop.stop","attributes":{"coding_agent.turn.id":"t1"}}` + "\n" +
		`{"name":"coding_agent.llm.turn","attributes":{"coding_agent.turn.id":"t1"}}` + "\n"
	if err := os.WriteFile(events, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if n := store.TurnsFor(child, 0); n != 1 {
		t.Fatalf("TurnsFor nested = %d want 1", n)
	}
}
