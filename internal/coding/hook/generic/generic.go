// Package generic provides a minimal hook adapter for additional coding agents
// (Gemini CLI, OpenCode, Copilot CLI, Pi).
package generic

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/coding/normalize"
)

// Adapter implements normalize.Adapter for a named vendor.
type Adapter struct {
	name string
}

func New(name string) *Adapter {
	return &Adapter{name: name}
}

func (a *Adapter) Vendor() string { return a.name }

func (a *Adapter) Handle(ctx context.Context, in normalize.Input) error {
	_ = ctx
	var payload map[string]any
	if len(in.Payload) > 0 {
		_ = json.Unmarshal(in.Payload, &payload)
	}
	if payload == nil {
		payload = map[string]any{}
	}
	sid := str(payload, "session_id", "sessionId", "sessionID", "conversation_id", "id")
	if sid == "" || strings.EqualFold(sid, "unknown") {
		return nil
	}
	cwd := str(payload, "cwd", "working_directory")
	model := str(payload, "model", "model_id")
	prompt := str(payload, "prompt", "message", "content", "text")
	ev := strings.ToLower(in.Event)

	switch {
	case strings.Contains(ev, "start"):
		return in.Emit.EmitSession(normalize.Session{
			SessionID: sid,
			Vendor:    a.name,
			Model:     model,
			CWD:       cwd,
			StartedAt: time.Now().UTC(),
		})
	case strings.Contains(ev, "end") || strings.Contains(ev, "stop"):
		now := time.Now().UTC()
		return in.Emit.EmitSession(normalize.Session{
			SessionID: sid,
			Vendor:    a.name,
			Model:     model,
			CWD:       cwd,
			EndedAt:   now,
		})
	default:
		if prompt != "" && in.ContentCapture != "minimal" {
			body := prompt
			if in.ContentCapture == "metadata_only" {
				body = ""
			}
			return in.Emit.EmitLLMTurn(normalize.LLMTurn{
				SessionID: sid,
				Vendor:    a.name,
				Model:     model,
				Prompt:    body,
			})
		}
		return nil
	}
}

func str(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}
