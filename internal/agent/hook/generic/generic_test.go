package generic

import (
	"context"
	"testing"

	"github.com/ishanjainn/superopen/internal/agent/normalize"
)

type recorder struct{ turns []normalize.LLMTurn }

func (r *recorder) EmitSession(normalize.Session) error   { return nil }
func (r *recorder) EmitToolCall(normalize.ToolCall) error { return nil }
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
}
