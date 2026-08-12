package opencode

import (
	"encoding/json"
	"testing"

	"github.com/ishanjainn/superopen/internal/coding/normalize"
)

type rec struct {
	sessions []normalize.Session
	turns    []normalize.LLMTurn
	tools    []normalize.ToolCall
}

func (r *rec) EmitSession(s normalize.Session) error             { r.sessions = append(r.sessions, s); return nil }
func (r *rec) EmitToolCall(t normalize.ToolCall) error           { r.tools = append(r.tools, t); return nil }
func (r *rec) EmitLLMTurn(t normalize.LLMTurn) error             { r.turns = append(r.turns, t); return nil }
func (r *rec) EmitEvent(normalize.EventEmission) error           { return nil }
func (r *rec) EmitEditDecision(normalize.EditDecision) error     { return nil }
func (r *rec) EmitSubagent(normalize.Subagent) error             { return nil }
func (r *rec) EmitGitCommit(normalize.GitCommit) error           { return nil }
func (r *rec) EmitGitPullRequest(normalize.GitPullRequest) error { return nil }

func TestOpenCodeMessageUsagePrefersHostCost(t *testing.T) {
	em := &rec{}
	cost := 0.0123
	payload, _ := json.Marshal(map[string]any{
		"session_id": "s1", "role": "assistant", "text": "hi",
		"model":  "claude-sonnet-4",
		"tokens": map[string]any{"input": 100, "output": 20, "cache": map[string]any{"read": 50, "write": 10}},
		"cost":   cost,
	})
	if err := handle(normalize.Input{Vendor: "opencode", Event: "message.updated", Payload: payload, Emit: em, ContentCapture: "full"}); err != nil {
		t.Fatal(err)
	}
	if len(em.turns) != 1 {
		t.Fatalf("turns=%d", len(em.turns))
	}
	if em.turns[0].CostUSD != cost {
		t.Fatalf("cost=%v want %v", em.turns[0].CostUSD, cost)
	}
	if em.turns[0].InputTokens != 160 {
		t.Fatalf("input=%d want 160", em.turns[0].InputTokens)
	}
}

func TestOpenCodeSkipsUnknownSession(t *testing.T) {
	em := &rec{}
	payload, _ := json.Marshal(map[string]any{
		"session_id": "unknown", "role": "user", "text": "In @go.mod",
	})
	if err := handle(normalize.Input{Vendor: "opencode", Event: "message.updated", Payload: payload, Emit: em, ContentCapture: "full"}); err != nil {
		t.Fatal(err)
	}
	if len(em.turns) != 0 || len(em.sessions) != 0 {
		t.Fatalf("expected skip for unknown session id")
	}
}

func TestOpenCodeDoesNotEmitSessionOnNoise(t *testing.T) {
	em := &rec{}
	payload, _ := json.Marshal(map[string]any{
		"session_id": "ses_abc", "type": "file.edited",
	})
	if err := handle(normalize.Input{Vendor: "opencode", Event: "file.edited", Payload: payload, Emit: em}); err != nil {
		t.Fatal(err)
	}
	if len(em.sessions) != 0 {
		t.Fatalf("noise must not emit session spans, got %d", len(em.sessions))
	}
}
