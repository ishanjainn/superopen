package main

import (
	"fmt"
	"testing"

	"github.com/ishanjainn/superopen/internal/tracestore"
)

func TestLoadSessionSpansQueriesRequestedAndLatestExactly(t *testing.T) {
	store := tracestore.NewLocalJSONL(t.TempDir())
	spans := make([]tracestore.Span, 0, 5003)
	for i := 0; i < 5001; i++ {
		spans = append(spans, tracestore.Span{TraceID: "old", SpanID: fmt.Sprintf("old-%d", i), Name: "coding_agent.tool", SessionID: "a-old", StartTimeUnixN: int64(i + 1)})
	}
	spans = append(spans,
		tracestore.Span{TraceID: "requested", SpanID: "requested", Name: "coding_agent.llm.turn", SessionID: "z-requested", StartTimeUnixN: 8000},
		tracestore.Span{TraceID: "latest", SpanID: "latest", Name: "coding_agent.llm.turn", SessionID: "zz-latest", StartTimeUnixN: 9000},
	)
	if err := store.Write(spans); err != nil {
		t.Fatal(err)
	}
	id, got, err := loadSessionSpans(store, "z-requested")
	if err != nil || id != "z-requested" || len(got) != 1 {
		t.Fatalf("requested id=%q spans=%d err=%v", id, len(got), err)
	}
	id, got, err = loadSessionSpans(store, "")
	if err != nil || id != "zz-latest" || len(got) != 1 {
		t.Fatalf("latest id=%q spans=%d err=%v", id, len(got), err)
	}
}
