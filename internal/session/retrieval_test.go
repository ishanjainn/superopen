package session

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/tracestore"
)

func TestMaterializeRetrievalReferencesCanonicalTurn(t *testing.T) {
	root := t.TempDir()
	paths := harness.Resolve(root)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	store := NewStore(paths)
	now := time.Now().UTC()
	spans := []tracestore.Span{
		{SpanID: "prompt-span", Name: "coding_agent.llm.turn", SessionID: "s1", StartTimeUnixN: now.UnixNano(), Attributes: map[string]string{"coding_agent.client": "codex", "gen_ai.prompt": "fix auth", "gen_ai.response.id": "turn_1"}},
		{SpanID: "retrieval-span", Name: "superopen.memory.retrieved", SessionID: "s1", StartTimeUnixN: now.Add(time.Millisecond).UnixNano(), Attributes: map[string]string{"coding_agent.client": "codex", "superopen.memory.pattern_ids": "[\"fp1\"]", "superopen.memory.estimated_tokens": "42", "superopen.memory.turn_id": "turn_1", "superopen.memory.delivery": "codex:prompt"}},
	}
	if _, err := store.MaterializeFromSpans("s1", spans, 0, 0); err != nil {
		t.Fatal(err)
	}
	doc, err := store.ReadDocument("s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.MemoryRetrievals) != 1 || doc.MemoryRetrievals[0].TurnID != "turn_1" || doc.MemoryRetrievals[0].PatternIDs[0] != "fp1" {
		t.Fatalf("retrievals=%+v", doc.MemoryRetrievals)
	}
	events, err := os.ReadFile(filepath.Join(paths.SessionDir("s1"), "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if count := bytes.Count(events, []byte(`"gen_ai.prompt":"fix auth"`)); count != 1 {
		t.Fatalf("canonical prompt count=%d\n%s", count, events)
	}
}
