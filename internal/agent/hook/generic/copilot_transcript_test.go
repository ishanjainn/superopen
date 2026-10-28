package generic

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/agent/normalize"
	"github.com/ishanjainn/superopen/internal/agent/pricing"
	"github.com/ishanjainn/superopen/internal/session/trace"
)

func TestCopilotSessionEndReadsPerModelShutdownUsage(t *testing.T) {
	root := t.TempDir()
	t.Setenv("COPILOT_HOME", root)
	path := filepath.Join(root, "session-state", "s1", "events.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	body := `{"type":"session.start","data":{"sessionId":"s1"}}
{"type":"assistant.message","data":{"outputTokens":999}}
{"type":"session.shutdown","data":{"modelMetrics":{"gemini-2.5-flash":{"usage":{"inputTokens":100,"outputTokens":20,"cacheReadTokens":30,"cacheWriteTokens":0,"reasoningTokens":5}},"gpt-5":{"usage":{"inputTokens":200,"outputTokens":40,"cacheReadTokens":50,"cacheWriteTokens":10}}}}}
`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	emit := &recorder{}
	if err := New("copilot-cli").Handle(context.Background(), normalize.Input{Event: "sessionEnd", Payload: []byte(`{"sessionId":"s1"}`), Emit: emit}); err != nil {
		t.Fatal(err)
	}
	s := emit.sessions[0]
	if s.InputTokens != 300 || s.OutputTokens != 60 || s.TotalTokens != 360 {
		t.Fatalf("usage=%+v", s)
	}
	want := pricing.Lookup("gemini-2.5-flash").Cost(100, 20, 30, 0) + pricing.Lookup("gpt-5").Cost(200, 40, 50, 10)
	if s.CostUSD != want {
		t.Fatalf("cost=%v want=%v", s.CostUSD, want)
	}
	if _, ok := copilotTranscriptUsage(path, "other"); ok {
		t.Fatal("accepted different session id")
	}
}

func TestCopilotTranscriptWithoutShutdownHasNoInventedTotals(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"session.start","data":{"sessionId":"s1"}}`+"\n"+`{"type":"assistant.message","data":{"outputTokens":50}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, ok := copilotTranscriptUsage(path, "s1"); ok {
		t.Fatal("accepted output-only transcript as complete usage")
	}
}

func TestCopilotSessionUsageBackfillAfterShutdown(t *testing.T) {
	root := t.TempDir()
	t.Setenv("COPILOT_HOME", root)
	path := filepath.Join(root, "session-state", "s1", "events.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	spans := []trace.Span{{Name: "coding_agent.session", SpanID: "root", Attributes: map[string]string{"coding_agent.client": "copilot-cli"}}}
	if BackfillCopilotSessionUsage("s1", spans) {
		t.Fatal("backfilled before shutdown")
	}
	body := `{"type":"session.start","data":{"sessionId":"s1"}}
{"type":"session.shutdown","data":{"modelMetrics":{"gpt-5":{"usage":{"inputTokens":100,"outputTokens":25}}}}}
`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	if !BackfillCopilotSessionUsage("s1", spans) {
		t.Fatal("shutdown usage was not backfilled")
	}
	if got := spans[0].Attributes["gen_ai.usage.total_tokens"]; got != "125" {
		t.Fatalf("total tokens = %q", got)
	}
	if got := spans[0].Attributes["gen_ai.usage.cost"]; got == "" {
		t.Fatal("missing estimated cost")
	}
	if BackfillCopilotSessionUsage("s1", spans) {
		t.Fatal("overwrote complete usage")
	}
}
