package generic

import (
	"context"
	"testing"

	"github.com/ishanjainn/superopen/internal/agent/normalize"
	"github.com/ishanjainn/superopen/internal/agent/pricing"
)

type recorder struct {
	sessions []normalize.Session
	turns    []normalize.LLMTurn
	tools    []normalize.ToolCall
}

func (r *recorder) EmitSession(s normalize.Session) error {
	r.sessions = append(r.sessions, s)
	return nil
}
func (r *recorder) EmitToolCall(t normalize.ToolCall) error {
	r.tools = append(r.tools, t)
	return nil
}
func (r *recorder) EmitLLMTurn(turn normalize.LLMTurn) error {
	r.turns = append(r.turns, turn)
	return nil
}
func (r *recorder) EmitEditDecision(normalize.EditDecision) error     { return nil }
func (r *recorder) EmitSubagent(normalize.Subagent) error             { return nil }
func (r *recorder) EmitEvent(normalize.EventEmission) error           { return nil }
func (r *recorder) EmitGitCommit(normalize.GitCommit) error           { return nil }
func (r *recorder) EmitGitPullRequest(normalize.GitPullRequest) error { return nil }

func TestCopilotTransformDoesNotDuplicateCanonicalPrompt(t *testing.T) {
	emit := &recorder{}
	adapter := New("copilot-cli")
	payload := []byte(`{"session_id":"s1","prompt":"fix auth","generation_id":"turn_1"}`)
	if err := adapter.Handle(context.Background(), normalize.Input{Event: "userPromptSubmitted", Payload: payload, ContentCapture: "full", Emit: emit}); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Handle(context.Background(), normalize.Input{Event: "userPromptTransformed", Payload: payload, ContentCapture: "full", Emit: emit}); err != nil {
		t.Fatal(err)
	}
	if len(emit.turns) != 1 {
		t.Fatalf("prompt turns=%d, want exactly one", len(emit.turns))
	}
	if emit.turns[0].GenerationID != "turn_1" {
		t.Fatalf("generation id=%q", emit.turns[0].GenerationID)
	}
	if emit.turns[0].Prompt != "fix auth" {
		t.Fatalf("userPromptSubmitted prompt=%q", emit.turns[0].Prompt)
	}
}

func TestHermesPromptFromExtra(t *testing.T) {
	emit := &recorder{}
	payload := []byte(`{"session_id":"s1","extra":{"user_message":"remember the JWT expiry"}}`)
	if err := New("hermes").Handle(context.Background(), normalize.Input{Event: "UserPromptSubmit", Payload: payload, ContentCapture: "full", Emit: emit}); err != nil {
		t.Fatal(err)
	}
	if len(emit.turns) != 1 || emit.turns[0].Prompt != "remember the JWT expiry" {
		t.Fatalf("hermes prompt turns=%+v", emit.turns)
	}
}

func TestGenericShellArgsAreNotFilePath(t *testing.T) {
	emit := &recorder{}
	adapter := New("gemini")
	payload := []byte(`{"session_id":"s1","tool_name":"shell","tool_input":"so graph query \"who wraps app\""}`)
	if err := adapter.Handle(context.Background(), normalize.Input{Event: "postToolUse", Payload: payload, ContentCapture: "full", Emit: emit}); err != nil {
		t.Fatal(err)
	}
	if len(emit.tools) != 1 {
		t.Fatalf("tools=%d", len(emit.tools))
	}
	if emit.tools[0].FilePath != "" {
		t.Fatalf("generic shell FilePath=%q", emit.tools[0].FilePath)
	}
}

func TestGenericAfterAgentAppliesUsageAndCost(t *testing.T) {
	emit := &recorder{}
	adapter := New("gemini")
	payload := []byte(`{"session_id":"s1","model":"gemini-2.5-flash","usage":{"input_tokens":1000,"output_tokens":500}}`)
	if err := adapter.Handle(context.Background(), normalize.Input{Event: "afterAgent", Payload: payload, ContentCapture: "full", Emit: emit}); err != nil {
		t.Fatal(err)
	}
	if len(emit.turns) != 1 {
		t.Fatalf("turns=%d, want 1", len(emit.turns))
	}
	turn := emit.turns[0]
	if turn.InputTokens != 1000 || turn.OutputTokens != 500 {
		t.Fatalf("tokens in=%d out=%d", turn.InputTokens, turn.OutputTokens)
	}
	want := pricing.Lookup("gemini-2.5-flash").Cost(1000, 500, 0, 0)
	if want <= 0 {
		t.Fatal("expected a priced gemini-2.5-flash rate")
	}
	if turn.CostUSD != want {
		t.Fatalf("cost=%v want %v", turn.CostUSD, want)
	}
}

func TestGenericPrefersHostCost(t *testing.T) {
	turn := &normalize.LLMTurn{Model: "gemini-2.5-flash"}
	applyUsage(turn, map[string]any{
		"usage":    map[string]any{"input_tokens": 1000, "output_tokens": 500},
		"cost_usd": 0.42,
	})
	if turn.CostUSD != 0.42 {
		t.Fatalf("cost=%v, want host-reported 0.42", turn.CostUSD)
	}
}

func TestGenericSessionEndPreservesUsage(t *testing.T) {
	emit := &recorder{}
	adapter := New("other")
	payload := []byte(`{"session_id":"s1","model":"gemini-2.5-flash","usage":{"input_tokens":1000,"output_tokens":500},"cost_usd":0.42}`)
	if err := adapter.Handle(context.Background(), normalize.Input{Event: "sessionEnd", Payload: payload, Emit: emit}); err != nil {
		t.Fatal(err)
	}
	if len(emit.sessions) != 1 {
		t.Fatalf("sessions=%d", len(emit.sessions))
	}
	s := emit.sessions[0]
	if s.InputTokens != 1000 || s.OutputTokens != 500 || s.TotalTokens != 1500 || s.CostUSD != 0.42 {
		t.Fatalf("session usage=%+v", s)
	}
}

func TestOpenClawNestedEventsBecomePromptToolAndUsage(t *testing.T) {
	emit := &recorder{}
	adapter := New("openclaw")
	for _, tc := range []struct{ event, payload string }{
		{"message_received", `{"sessionKey":"s1","event":{"content":"fix auth"}}`},
		{"before_tool_call", `{"sessionKey":"s1","event":{"toolName":"exec","toolCallId":"call1","params":{"command":"pwd"}}}`},
		{"after_tool_call", `{"sessionKey":"s1","event":{"toolName":"exec","toolCallId":"call1","result":"ok"}}`},
		{"llm_output", `{"sessionKey":"s1","event":{"model":"gemini-2.5-flash","assistantTexts":["done"],"usage":{"input":10,"output":5,"cacheRead":3,"cacheWrite":2}}}`},
	} {
		if err := adapter.Handle(context.Background(), normalize.Input{Event: tc.event, Payload: []byte(tc.payload), Emit: emit}); err != nil {
			t.Fatal(err)
		}
	}
	if len(emit.turns) != 2 || emit.turns[0].Prompt != "fix auth" || emit.turns[1].Response != "done" || emit.turns[1].InputTokens != 15 || emit.turns[1].OutputTokens != 5 || emit.turns[1].CacheReadTokens != 3 {
		t.Fatalf("turns=%+v", emit.turns)
	}
	if len(emit.tools) != 2 || emit.tools[0].ToolName != "exec" || emit.tools[0].Args != `{"command":"pwd"}` || emit.tools[1].Result != "ok" {
		t.Fatalf("tools=%+v", emit.tools)
	}
}

func TestClineRunUsageAndPrompt(t *testing.T) {
	emit := &recorder{}
	adapter := New("cline")
	for _, tc := range []struct{ event, payload string }{
		{"UserPromptSubmit", `{"taskId":"s1","prompt":"fix auth"}`},
		{"afterRun", `{"taskId":"s1","response":"done","usage":{"input_tokens":100,"output_tokens":20,"cache_read_tokens":10},"cost_usd":0.01}`},
	} {
		if err := adapter.Handle(context.Background(), normalize.Input{Event: tc.event, Payload: []byte(tc.payload), Emit: emit}); err != nil {
			t.Fatal(err)
		}
	}
	if len(emit.turns) != 2 || emit.turns[0].Prompt != "fix auth" || emit.turns[1].InputTokens != 100 || emit.turns[1].CostUSD != 0.01 {
		t.Fatalf("turns=%+v", emit.turns)
	}
}
