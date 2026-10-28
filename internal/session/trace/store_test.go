package trace_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/session/trace"
)

func TestLocalJSONLWriteQuery(t *testing.T) {
	dir := t.TempDir()
	s := trace.NewLocalJSONL(dir)
	sp := trace.Span{
		TraceID: "t1", SpanID: "s1", Name: "coding_agent.read",
		StartTimeUnixN: time.Now().UnixNano(), SessionID: "ses1",
		Attributes: map[string]string{"coding_agent.file_path": "a.go"},
	}
	if err := s.Write([]trace.Span{sp}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Query(trace.QueryFilter{SessionID: "ses1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d spans", len(got))
	}
}

func TestUpdateUsageReplacesTurnWithoutDuplicatingEvents(t *testing.T) {
	s := trace.NewLocalJSONL(t.TempDir())
	id := "usage-session"
	turn := trace.Span{Name: "coding_agent.llm.turn", SpanID: "turn-1", SessionID: id,
		Attributes: map[string]string{"gen_ai.response.id": "gen-1"}}
	tool := trace.Span{Name: "coding_agent.tool.call", SpanID: "tool-1", SessionID: id}
	if err := s.Write([]trace.Span{turn, tool}); err != nil {
		t.Fatal(err)
	}
	turn.Attributes["gen_ai.usage.total_tokens"] = "110"
	turn.Attributes["gen_ai.usage.cost"] = "0.42"
	if err := s.UpdateUsage(id, []trace.Span{turn}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Query(trace.QueryFilter{SessionID: id})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Attributes["gen_ai.usage.total_tokens"] != "110" || got[1].Name != tool.Name {
		t.Fatalf("updated events = %+v", got)
	}
	tokens, cost, err := s.SessionCost(id)
	if err != nil || tokens != 110 || cost != 0.42 {
		t.Fatalf("tokens=%d cost=%v err=%v", tokens, cost, err)
	}
}

func TestInboxRemovedAfterTraceResolves(t *testing.T) {
	dir := t.TempDir()
	s := trace.NewLocalJSONL(dir)
	if err := s.Write([]trace.Span{{TraceID: "trace", SpanID: "early", Name: "prompt"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "inbox.jsonl")); err != nil {
		t.Fatal("expected lazy inbox", err)
	}
	if err := s.Write([]trace.Span{{TraceID: "trace", SpanID: "late", Name: "session", SessionID: "resolved"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "inbox.jsonl")); !os.IsNotExist(err) {
		t.Fatal("empty inbox must be removed")
	}
	got, err := s.Query(trace.QueryFilter{SessionID: "resolved"})
	if err != nil || len(got) != 2 {
		t.Fatalf("resolved events: %d (%v)", len(got), err)
	}
}

func TestLatestSessionIDIgnoresGlobalOrdering(t *testing.T) {
	dir := t.TempDir()
	s := trace.NewLocalJSONL(dir)
	spans := make([]trace.Span, 0, 5002)
	for i := 0; i < 5001; i++ {
		spans = append(spans, trace.Span{TraceID: "old-trace", SpanID: fmt.Sprintf("old-%d", i), Name: "coding_agent.tool", SessionID: "a-old", StartTimeUnixN: int64(i + 1)})
	}
	spans = append(spans, trace.Span{TraceID: "new-trace", SpanID: "new", Name: "coding_agent.llm.turn", SessionID: "z-new", StartTimeUnixN: 9000})
	if err := s.Write(spans); err != nil {
		t.Fatal(err)
	}
	id, err := s.LatestSessionID()
	if err != nil || id != "z-new" {
		t.Fatalf("latest=%q err=%v", id, err)
	}
	got, err := s.Query(trace.QueryFilter{SessionID: id})
	if err != nil || len(got) != 1 {
		t.Fatalf("exact latest query: %d spans err=%v", len(got), err)
	}
}
