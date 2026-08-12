// Package generic provides a minimal hook adapter for additional coding agents
// (Gemini CLI, OpenCode, Copilot CLI, Pi).
package generic

import (
	"context"
	"encoding/json"
	"fmt"
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
	now := time.Now().UTC()

	switch {
	case ev == "sessionstart" || ev == "session_start":
		return in.Emit.EmitSession(normalize.Session{
			SessionID: sid,
			Vendor:    a.name,
			Model:     model,
			CWD:       cwd,
			StartedAt: now,
		})
	case ev == "sessionend" || ev == "session_end":
		return in.Emit.EmitSession(normalize.Session{
			SessionID: sid,
			Vendor:    a.name,
			Model:     model,
			CWD:       cwd,
			EndedAt:   now,
		})
	case ev == "beforeagent" || ev == "userpromptsubmitted":
		if prompt == "" {
			return nil
		}
		return in.Emit.EmitLLMTurn(normalize.LLMTurn{
			SessionID: sid, Vendor: a.name, Model: model, Prompt: prompt, StartedAt: now,
		})
	case ev == "afteragent" || ev == "agentstop":
		response := str(payload, "prompt_response", "response", "output", "text")
		if prompt == "" && response == "" {
			return nil
		}
		return in.Emit.EmitLLMTurn(normalize.LLMTurn{
			SessionID: sid, Vendor: a.name, Model: model, Prompt: prompt, Response: response, EndedAt: now,
		})
	case ev == "beforetool" || ev == "pretooluse" || ev == "aftertool" || ev == "posttooluse" || ev == "posttoolusefailure":
		toolName := str(payload, "tool_name", "toolName", "tool")
		toolID := str(payload, "tool_call_id", "toolCallId", "tool_use_id")
		toolInput := jsonText(payload["tool_input"])
		toolResponse := jsonText(payload["tool_response"])
		if toolResponse == "" {
			toolResponse = jsonText(payload["tool_result"])
		}
		call := normalize.ToolCall{
			SessionID: sid, Vendor: a.name, Model: model, ToolName: toolName,
			ToolUseID: toolID, Args: toolInput, Result: toolResponse,
		}
		if ev == "beforetool" || ev == "pretooluse" {
			call.StartedAt = now
		} else {
			call.EndedAt = now
		}
		if ev == "posttoolusefailure" || hasError(payload["tool_response"]) {
			call.Errored = true
			call.ErrorMsg = toolResponse
		}
		return in.Emit.EmitToolCall(call)
	default:
		if prompt != "" {
			return in.Emit.EmitLLMTurn(normalize.LLMTurn{
				SessionID: sid,
				Vendor:    a.name,
				Model:     model,
				Prompt:    prompt,
			})
		}
		return nil
	}
}

func jsonText(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(body)
}

func hasError(value any) bool {
	obj, ok := value.(map[string]any)
	if !ok {
		return false
	}
	if errValue, exists := obj["error"]; exists && errValue != nil && fmt.Sprint(errValue) != "" {
		return true
	}
	return false
}

func str(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}
