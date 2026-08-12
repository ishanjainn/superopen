package tracestore_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/tracestore"
)

func TestLocalJSONLWriteQuery(t *testing.T) {
	dir := t.TempDir()
	s := tracestore.NewLocalJSONL(dir)
	sp := tracestore.Span{
		TraceID: "t1", SpanID: "s1", Name: "coding_agent.read",
		StartTimeUnixN: time.Now().UnixNano(), SessionID: "ses1",
		Attributes: map[string]string{"coding_agent.file_path": "a.go"},
	}
	if err := s.Write([]tracestore.Span{sp}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Query(tracestore.QueryFilter{SessionID: "ses1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d spans", len(got))
	}
}

func TestInboxRemovedAfterTraceResolves(t *testing.T) {
	dir := t.TempDir()
	s := tracestore.NewLocalJSONL(dir)
	if err := s.Write([]tracestore.Span{{TraceID: "trace", SpanID: "early", Name: "prompt"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "inbox.jsonl")); err != nil {
		t.Fatal("expected lazy inbox", err)
	}
	if err := s.Write([]tracestore.Span{{TraceID: "trace", SpanID: "late", Name: "session", SessionID: "resolved"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "inbox.jsonl")); !os.IsNotExist(err) {
		t.Fatal("empty inbox must be removed")
	}
	got, err := s.Query(tracestore.QueryFilter{SessionID: "resolved"})
	if err != nil || len(got) != 2 {
		t.Fatalf("resolved events: %d (%v)", len(got), err)
	}
}
