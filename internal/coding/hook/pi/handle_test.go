package pi

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

func TestPiHostCostPreferred(t *testing.T) {
	em := &rec{}
	payload, _ := json.Marshal(map[string]any{
		"session_id": "019fdd1a-6662-7a13-8fad-c54abef89fd4", "type": "message_end",
		"usage": map[string]any{
			"input": 10, "output": 5, "cacheRead": 2, "cacheWrite": 1,
			"cost": map[string]any{"total": 0.42},
		},
	})
	if err := handle(normalize.Input{Vendor: "pi", Event: "message_end", Payload: payload, Emit: em}); err != nil {
		t.Fatal(err)
	}
	if len(em.turns) != 1 || em.turns[0].CostUSD != 0.42 {
		t.Fatalf("got %+v", em.turns)
	}
}

func TestPiSessionIDFromFileStripsTimestamp(t *testing.T) {
	em := &rec{}
	payload, _ := json.Marshal(map[string]any{
		"type":         "tool_execution_end",
		"session_file": "/tmp/2026-08-07T16-42-02-722Z_019fdd1a-6662-7a13-8fad-c54abef89fd4.jsonl",
		"toolName":     "bash",
		"toolCallId":   "t1",
		"result":       "ok",
	})
	if err := handle(normalize.Input{Vendor: "pi", Event: "tool_execution_end", Payload: payload, Emit: em, ContentCapture: "full"}); err != nil {
		t.Fatal(err)
	}
	if len(em.tools) != 1 {
		t.Fatalf("tools=%d", len(em.tools))
	}
	if em.tools[0].SessionID != "019fdd1a-6662-7a13-8fad-c54abef89fd4" {
		t.Fatalf("session=%q", em.tools[0].SessionID)
	}
}

func TestPiSkipsUnknownSession(t *testing.T) {
	em := &rec{}
	payload, _ := json.Marshal(map[string]any{"type": "message_end", "text": "hi"})
	if err := handle(normalize.Input{Vendor: "pi", Event: "message_end", Payload: payload, Emit: em, ContentCapture: "full"}); err != nil {
		t.Fatal(err)
	}
	if len(em.turns) != 0 {
		t.Fatalf("expected skip, got %d turns", len(em.turns))
	}
}
