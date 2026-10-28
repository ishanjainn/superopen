package generic

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/agent/normalize"
	"github.com/ishanjainn/superopen/internal/agent/pricing"
)

func TestGeminiSessionEndReadsJSONLUsage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	body := `{"sessionId":"s1","projectHash":"repo"}
{"id":"a","type":"gemini","model":"gemini-2.5-flash","tokens":{"input":100,"output":20,"cached":30,"thoughts":5,"total":125}}
{"id":"a","type":"gemini","model":"gemini-2.5-flash","tokens":{"input":100,"output":20,"cached":30,"thoughts":5,"total":125}}
{"$rewindTo":"a"}
{"id":"b","type":"gemini","model":"gemini-2.5-flash","tokens":{"input":200,"output":40,"cached":50,"thoughts":10,"total":250}}
`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	emit := &recorder{}
	payload := []byte(`{"session_id":"s1","transcript_path":"` + path + `"}`)
	if err := New("gemini").Handle(context.Background(), normalize.Input{Event: "SessionEnd", Payload: payload, Emit: emit}); err != nil {
		t.Fatal(err)
	}
	if len(emit.sessions) != 1 {
		t.Fatalf("sessions=%d", len(emit.sessions))
	}
	s := emit.sessions[0]
	if s.InputTokens != 300 || s.OutputTokens != 75 || s.TotalTokens != 375 {
		t.Fatalf("usage=%+v", s)
	}
	rate := pricing.Lookup("gemini-2.5-flash")
	wantCost := rate.Cost(100, 25, 30, 0) + rate.Cost(200, 50, 50, 0)
	if s.CostUSD != wantCost {
		t.Fatalf("cost=%v want=%v", s.CostUSD, wantCost)
	}
}

func TestGeminiSessionEndReadsLegacyJSONAndRejectsWrongSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	body := `{"sessionId":"s1","messages":[{"id":"x","type":"gemini","model":"gemini-2.5-flash","tokens":{"input":5,"output":2,"thoughts":1,"total":8}}]}`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	usage, ok := geminiTranscriptUsage(path, "s1", "")
	if !ok || usage.InputTokens != 5 || usage.OutputTokens != 3 || usage.TotalTokens != 8 {
		t.Fatalf("usage=%+v ok=%v", usage, ok)
	}
	if _, ok := geminiTranscriptUsage(path, "other", ""); ok {
		t.Fatal("accepted a different session's transcript")
	}
}

func TestGeminiSessionEndPrefersHostUsage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	if err := os.WriteFile(path, []byte(`{"sessionId":"s1","messages":[{"id":"x","type":"gemini","tokens":{"input":5,"output":2,"total":7}}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	emit := &recorder{}
	payload := []byte(`{"session_id":"s1","model":"gemini-2.5-flash","transcript_path":"` + path + `","usage":{"input_tokens":10,"output_tokens":20}}`)
	if err := New("gemini").Handle(context.Background(), normalize.Input{Event: "SessionEnd", Payload: payload, Emit: emit}); err != nil {
		t.Fatal(err)
	}
	if got := emit.sessions[0].TotalTokens; got != 30 {
		t.Fatalf("host usage total=%d", got)
	}
}
