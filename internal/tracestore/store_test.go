package tracestore_test

import (
	"testing"
	"time"

	"github.com/superopen/so/internal/tracestore"
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
