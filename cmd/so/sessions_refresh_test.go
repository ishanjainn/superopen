package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/session"
	"github.com/ishanjainn/superopen/internal/session/trace"
)

func TestRefreshIngestsWithoutFinalize(t *testing.T) {
	root := t.TempDir()
	layout := paths.Resolve(root)
	if err := layout.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	id := "codex-refresh"
	dir := layout.SessionDir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixNano()
	sp := trace.Span{
		TraceID:        id,
		SpanID:         "s1",
		Name:           "coding_agent.llm.turn",
		SessionID:      id,
		StartTimeUnixN: now,
		Attributes:     map[string]string{"gen_ai.prompt": "please fix the login timeout"},
	}
	f, err := os.Create(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(f).Encode(sp); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	_ = session.NewStore(layout).Start(session.Meta{ID: id, Vendor: "codex", StartedAt: time.Now().UTC(), PromptPreview: "please fix the login timeout"})

	if err := refreshSession(root, id); err != nil {
		t.Fatal(err)
	}
	store, err := memory.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hits, err := store.Search(memory.SearchFilter{Query: "login timeout", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range hits {
		if h.Kind == memory.KindPrompt {
			found = true
		}
		if h.Kind == memory.KindSession && h.Source == memory.SourceHeadless {
			t.Fatalf("refresh must not distill: %+v", h)
		}
	}
	if !found {
		t.Fatalf("refresh must ingest prompts, got %+v", hits)
	}
	if store.HasSessionRollup(id) {
		t.Fatal("refresh must not create a session rollup")
	}
}
