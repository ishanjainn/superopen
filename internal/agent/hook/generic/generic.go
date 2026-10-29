// Package generic provides a minimal hook adapter for coding agents that
// do not have a dedicated adapter (Gemini CLI, Copilot CLI, and any
// unknown --vendor). OpenCode and Pi have dedicated adapters.
package generic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/agent/normalize"
	"github.com/ishanjainn/superopen/internal/agent/pricing"
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
	if a.name == "openclaw" {
		flattenOpenClawPayload(payload)
	}
	sid := str(payload, "session_id", "sessionId", "sessionID", "conversation_id", "taskId", "task_id", "sessionKey", "id")
	if sid == "" || strings.EqualFold(sid, "unknown") {
		return nil
	}
	cwd := str(payload, "cwd", "working_directory")
	model := str(payload, "model", "model_id")
	prompt := str(payload, "prompt", "message", "content", "text")
	if prompt == "" && a.name == "hermes" {
		if extra, ok := nestedMap(payload, "extra"); ok {
			prompt = str(extra, "user_message")
		}
	}
	generationID := str(payload, "generation_id", "turn_id", "message_id", "request_id")
	ev := strings.ToLower(in.Event)
	now := time.Now().UTC()

	switch {
	case ev == "userprompttransformed" || ev == "before_prompt_build":
		// The submitted event is the sole authoritative prompt trace. This
		// model-facing hook exists only to deliver locally retrieved context.
		return nil
	case ev == "sessionstart" || ev == "session_start" || ev == "on_session_start" || ev == "beforerun" || ev == "preinvocation" || ev == "session.created":
		return in.Emit.EmitSession(normalize.Session{
			SessionID: sid,
			Vendor:    a.name,
			Model:     model,
			CWD:       cwd,
			StartedAt: now,
		})
	case ev == "sessionend" || ev == "session_end" || ev == "on_session_end" || ev == "session.deleted":
		usage := normalize.LLMTurn{Model: model}
		applyUsage(&usage, payload)
		if a.name == "gemini" && usage.TotalTokens == 0 {
			if recorded, ok := geminiTranscriptUsage(str(payload, "transcript_path"), sid, model); ok {
				usage = recorded
			}
		}
		if a.name == "copilot-cli" && usage.TotalTokens == 0 {
			if recorded, ok := copilotTranscriptUsage(str(payload, "transcriptPath", "transcript_path"), sid); ok {
				usage = recorded
			}
		}
		return in.Emit.EmitSession(normalize.Session{
			SessionID:    sid,
			Vendor:       a.name,
			Model:        model,
			CWD:          cwd,
			EndedAt:      now,
			InputTokens:  usage.InputTokens,
			OutputTokens: usage.OutputTokens,
			TotalTokens:  usage.TotalTokens,
			CostUSD:      usage.CostUSD,
		})
	case ev == "beforeagent" || ev == "userpromptsubmitted" || ev == "userpromptsubmit" || ev == "pre_llm_call" || ev == "message_received":
		if prompt == "" {
			return nil
		}
		return in.Emit.EmitLLMTurn(normalize.LLMTurn{
			SessionID: sid, Vendor: a.name, Model: model, Prompt: prompt, GenerationID: generationID, StartedAt: now,
		})
	case ev == "afteragent" || ev == "agentstop" || ev == "stop" || ev == "afterrun" || ev == "postinvocation" || ev == "llm_output":
		response := str(payload, "prompt_response", "response", "output", "text")
		if prompt == "" && response == "" && !hasUsage(payload) {
			return nil
		}
		turn := normalize.LLMTurn{
			SessionID: sid, Vendor: a.name, Model: model, Prompt: prompt, Response: response, EndedAt: now,
		}
		applyUsage(&turn, payload)
		return in.Emit.EmitLLMTurn(turn)
	case ev == "beforetool" || ev == "pretooluse" || ev == "pre_tool_use" || ev == "pre_tool_call" || ev == "before_tool_call" ||
		ev == "aftertool" || ev == "posttooluse" || ev == "post_tool_call" || ev == "after_tool_call" || ev == "posttoolusefailure":
		toolName := str(payload, "tool_name", "toolName", "tool", "name")
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
		if ev == "beforetool" || ev == "pretooluse" || ev == "pre_tool_use" || ev == "pre_tool_call" || ev == "before_tool_call" {
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

// OpenClaw keeps its event under "event" so tool arguments cannot replace
// envelope identity. Lift only the fields the common adapter understands.
func flattenOpenClawPayload(payload map[string]any) {
	event, ok := nestedMap(payload, "event")
	if !ok {
		return
	}
	if payload["model"] == nil && event["model"] != nil {
		payload["model"] = event["model"]
	}
	for dest, names := range map[string][]string{
		"prompt":        {"currentUserMessage", "prompt", "content", "text"},
		"response":      {"response", "output", "text"},
		"tool_name":     {"toolName", "tool_name", "name"},
		"tool_input":    {"params", "tool_input", "input"},
		"tool_response": {"result", "tool_result", "output"},
		"tool_call_id":  {"toolCallId", "tool_call_id"},
		"usage":         {"usage"},
	} {
		if payload[dest] != nil {
			continue
		}
		for _, name := range names {
			if event[name] != nil {
				payload[dest] = event[name]
				break
			}
		}
	}
	if payload["response"] == nil {
		if texts, ok := event["assistantTexts"].([]any); ok {
			parts := make([]string, 0, len(texts))
			for _, part := range texts {
				if s, ok := part.(string); ok {
					parts = append(parts, s)
				}
			}
			payload["response"] = strings.Join(parts, "\n")
		}
	}
	if usage, ok := nestedMap(event, "usage"); ok {
		// OpenClaw's normalized input excludes cache reads and writes.
		// Our canonical input count includes both portions.
		cacheRead := intFrom(usage, "cacheRead")
		cacheWrite := intFrom(usage, "cacheWrite")
		payload["usage"] = map[string]any{
			"input_tokens":          intFrom(usage, "input") + cacheRead + cacheWrite,
			"output_tokens":         intFrom(usage, "output"),
			"cache_read_tokens":     cacheRead,
			"cache_creation_tokens": cacheWrite,
		}
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

func hasUsage(m map[string]any) bool {
	in, out, _, _ := usageFrom(m)
	return in > 0 || out > 0
}

func applyUsage(turn *normalize.LLMTurn, payload map[string]any) {
	inTok, outTok, cacheRead, cacheWrite := usageFrom(payload)
	turn.InputTokens = inTok
	turn.OutputTokens = outTok
	turn.CacheReadTokens = cacheRead
	turn.CacheCreationTokens = cacheWrite
	turn.TotalTokens = inTok + outTok
	if hostCost := floatFrom(payload, "cost", "cost_usd", "total_cost"); hostCost > 0 {
		turn.CostUSD = hostCost
		return
	}
	if rate := pricing.Lookup(turn.Model); rate.InputPer1M > 0 || rate.OutputPer1M > 0 {
		turn.CostUSD = rate.Cost(inTok, outTok, cacheRead, cacheWrite)
	}
}

func usageFrom(m map[string]any) (in, out, cacheRead, cacheWrite int64) {
	in = intFrom(m, "input_tokens", "prompt_tokens")
	out = intFrom(m, "output_tokens", "completion_tokens")
	cacheRead = intFrom(m, "cache_read_tokens", "cached_tokens")
	cacheWrite = intFrom(m, "cache_creation_tokens")
	if u, ok := nestedMap(m, "usage"); ok {
		if v := intFrom(u, "input_tokens", "prompt_tokens", "input"); v > 0 {
			in = v
		}
		if v := intFrom(u, "output_tokens", "completion_tokens", "output"); v > 0 {
			out = v
		}
		if v := intFrom(u, "cache_read_tokens", "cache_read_input_tokens", "cached_tokens"); v > 0 {
			cacheRead = v
		}
		if v := intFrom(u, "cache_creation_tokens", "cache_creation_input_tokens", "cache_write"); v > 0 {
			cacheWrite = v
		}
	}
	return
}

func nestedMap(m map[string]any, key string) (map[string]any, bool) {
	v, ok := m[key]
	if !ok {
		return nil, false
	}
	obj, ok := v.(map[string]any)
	return obj, ok
}

func intFrom(m map[string]any, keys ...string) int64 {
	for _, k := range keys {
		switch v := m[k].(type) {
		case float64:
			return int64(v)
		case int:
			return int64(v)
		case int64:
			return v
		case json.Number:
			n, _ := v.Int64()
			return n
		}
	}
	return 0
}

func floatFrom(m map[string]any, keys ...string) float64 {
	for _, k := range keys {
		switch v := m[k].(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case int64:
			return float64(v)
		case json.Number:
			n, _ := v.Float64()
			return n
		}
	}
	return 0
}

func str(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}
