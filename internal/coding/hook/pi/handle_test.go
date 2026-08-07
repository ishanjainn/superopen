package pi

import (
	"encoding/json"
	"testing"

	"github.com/ishanjainn/superopen/internal/coding/normalize"
)

type rec struct {
	sessions []normalize.Session
	turns    []normalize.LLMTurn
}

func (r *rec) EmitSession(s normalize.Session) error                 { r.sessions = append(r.sessions, s); return nil }
func (r *rec) EmitToolCall(normalize.ToolCall) error                 { return nil }
func (r *rec) EmitLLMTurn(t normalize.LLMTurn) error                 { r.turns = append(r.turns, t); return nil }
func (r *rec) EmitEvent(normalize.EventEmission) error               { return nil }
func (r *rec) EmitEditDecision(normalize.EditDecision) error         { return nil }
func (r *rec) EmitSubagent(normalize.Subagent) error                 { return nil }
func (r *rec) EmitGitCommit(normalize.GitCommit) error               { return nil }
func (r *rec) EmitGitPullRequest(normalize.GitPullRequest) error     { return nil }

func TestPiHostCostPreferred(t *testing.T) {
	em := &rec{}
	payload, _ := json.Marshal(map[string]any{
		"session_id": "p1", "type": "message_end",
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
