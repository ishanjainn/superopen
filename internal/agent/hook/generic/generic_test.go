package generic

import (
	"context"
	"testing"

	"github.com/ishanjainn/superopen/internal/agent/normalize"
	"github.com/ishanjainn/superopen/internal/agent/pricing"
)

type recorder struct {
	turns []normalize.LLMTurn
	tools []normalize.ToolCall
}

func (r *recorder) EmitSession(normalize.Session) error { return nil }
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
