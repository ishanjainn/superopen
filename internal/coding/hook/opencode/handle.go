package opencode

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/coding/pricing"
	"github.com/ishanjainn/superopen/internal/coding/sessionstate"
	"github.com/ishanjainn/superopen/internal/coding/normalize"
)

// Payload covers the JSON we receive from plugins/opencode.
type payload struct {
	Type      string `json:"type"`
	Event     string `json:"event"`
	SessionID string `json:"session_id"`
	SessionId string `json:"sessionId"`
	CWD       string `json:"cwd"`
	Directory string `json:"directory"`
	Model     string `json:"model"`
	Provider  string `json:"provider"`
	Title     string `json:"title"`
	Prompt    string `json:"prompt"`
	Text      string `json:"text"`
	Role      string `json:"role"`
	MessageID string `json:"message_id"`
	ToolName  string `json:"tool_name"`
	ToolID    string `json:"tool_use_id"`
	ToolInput json.RawMessage `json:"tool_input"`
	ToolResult string `json:"tool_result"`
	Errored   bool   `json:"errored"`
	// Tokens - OpenCode shape (input excludes cache; we recombine for pricing).
	Tokens *struct {
		Input     int64 `json:"input"`
		Output    int64 `json:"output"`
		Reasoning int64 `json:"reasoning"`
		Cache     *struct {
			Read  int64 `json:"read"`
			Write int64 `json:"write"`
		} `json:"cache"`
	} `json:"tokens"`
	// Host-reported cost when present (OpenCode AssistantMessage.cost).
	Cost *float64 `json:"cost"`
	PartType string `json:"part_type"`
}

func handle(in normalize.Input) error {
	var p payload
	_ = json.Unmarshal(in.Payload, &p)
	event := strings.ToLower(in.Event)
	if event == "" {
		event = strings.ToLower(firstNonEmpty(p.Type, p.Event))
	}
	sid := firstNonEmpty(p.SessionID, p.SessionId)
	if sid == "" {
		sid = "unknown"
	}
	cwd := firstNonEmpty(p.CWD, p.Directory)
	model := p.Model
	now := time.Now().UTC()

	switch {
	case strings.Contains(event, "session.created") || event == "sessionstart" || event == "session_start":
		return in.Emit.EmitSession(normalize.Session{
			SessionID: sid, Vendor: in.Vendor, Model: model, CWD: cwd, StartedAt: now,
		})

	case strings.Contains(event, "dispose") || strings.Contains(event, "session.end") ||
		event == "sessionend" || event == "session_end":
		s := normalize.Session{
			SessionID: sid, Vendor: in.Vendor, Model: model, CWD: cwd, EndedAt: now,
			Outcome: "completed",
		}
		applyAccumulated(&s, sid, in.Vendor)
		return in.Emit.EmitSession(s)

	case strings.Contains(event, "tool.execute.before") || event == "pretooluse" || event == "tool_start":
		return in.Emit.EmitEvent(normalize.EventEmission{
			SessionID: sid, Name: "coding_agent.tool.requested", At: now,
			Attrs: map[string]any{
				"coding_agent.client": in.Vendor,
				"gen_ai.tool.name":    p.ToolName,
				"gen_ai.tool.call.id": firstNonEmpty(p.ToolID, p.MessageID),
				"code.cwd":            cwd,
			},
		})

	case strings.Contains(event, "tool.execute.after") || event == "posttooluse" || event == "tool_end":
		return in.Emit.EmitToolCall(normalize.ToolCall{
			SessionID: sid, Vendor: in.Vendor, Model: model, ToolName: p.ToolName,
			ToolUseID: firstNonEmpty(p.ToolID, p.MessageID), WorkingDir: cwd,
			StartedAt: now.Add(-50 * time.Millisecond), EndedAt: now,
			Errored: p.Errored,
			Args:    capture(in, string(p.ToolInput)),
			Result:  capture(in, p.ToolResult),
		})

	case strings.Contains(event, "message.updated") || event == "message" || strings.Contains(event, "chat.message"):
		role := strings.ToLower(p.Role)
		if role == "user" || p.Prompt != "" {
			return in.Emit.EmitLLMTurn(normalize.LLMTurn{
				SessionID: sid, Vendor: in.Vendor, Model: model, StartedAt: now,
				Prompt: capture(in, firstNonEmpty(p.Prompt, p.Text)),
			})
		}
		turn := normalize.LLMTurn{
			SessionID: sid, Vendor: in.Vendor, Model: model,
			StartedAt: now.Add(-time.Second), EndedAt: now,
			Response:             capture(in, p.Text),
			AssistantMessageOnly: true,
		}
		stampUsage(&turn, p, model)
		accumulateUsage(sid, in.Vendor, turn)
		return in.Emit.EmitLLMTurn(turn)

	case strings.Contains(event, "message.part.updated") || strings.Contains(event, "step-finish"):
		// Prefer accumulating step-finish tokens (msg.tokens is last-step only).
		if p.PartType != "" && p.PartType != "step-finish" && !strings.Contains(event, "step-finish") {
			return nil
		}
		turn := normalize.LLMTurn{
			SessionID: sid, Vendor: in.Vendor, Model: model,
			StartedAt: now, EndedAt: now, AssistantMessageOnly: true,
		}
		stampUsage(&turn, p, model)
		if turn.InputTokens == 0 && turn.OutputTokens == 0 {
			return nil
		}
		accumulateUsage(sid, in.Vendor, turn)
		return in.Emit.EmitLLMTurn(turn)

	default:
		if p.Prompt != "" || p.Text != "" {
			return in.Emit.EmitLLMTurn(normalize.LLMTurn{
				SessionID: sid, Vendor: in.Vendor, Model: model, StartedAt: now,
				Prompt: capture(in, firstNonEmpty(p.Prompt, p.Text)),
			})
		}
		return in.Emit.EmitSession(normalize.Session{
			SessionID: sid, Vendor: in.Vendor, Model: model, CWD: cwd, StartedAt: now,
		})
	}
}

func stampUsage(turn *normalize.LLMTurn, p payload, model string) {
	if p.Tokens == nil {
		if p.Cost != nil && *p.Cost > 0 {
			turn.CostUSD = *p.Cost
		}
		return
	}
	cacheRead, cacheWrite := int64(0), int64(0)
	if p.Tokens.Cache != nil {
		cacheRead = p.Tokens.Cache.Read
		cacheWrite = p.Tokens.Cache.Write
	}
	// OpenCode input excludes cached tokens; recombine for pricing.Cost total-input contract.
	fresh := p.Tokens.Input
	turn.InputTokens = fresh + cacheRead + cacheWrite
	turn.OutputTokens = p.Tokens.Output
	turn.CacheReadTokens = cacheRead
	turn.CacheCreationTokens = cacheWrite
	turn.TotalTokens = turn.InputTokens + turn.OutputTokens
	if p.Cost != nil && *p.Cost > 0 {
		turn.CostUSD = *p.Cost // host-reported accurate cost preferred
		return
	}
	if rate := pricing.Lookup(firstNonEmpty(model, turn.Model)); rate.InputPer1M > 0 || rate.OutputPer1M > 0 {
		turn.CostUSD = rate.Cost(turn.InputTokens, turn.OutputTokens, cacheRead, cacheWrite)
	}
}

func accumulateUsage(sid, vendor string, turn normalize.LLMTurn) {
	if sid == "" || sid == "unknown" {
		return
	}
	st := sessionstate.Load(sid, vendor)
	if st == nil {
		st = &sessionstate.State{}
	}
	st.InputTokens += turn.InputTokens
	st.OutputTokens += turn.OutputTokens
	st.CostUSD += turn.CostUSD
	if turn.Model != "" {
		st.Model = turn.Model
	}
	sessionstate.Save(sid, vendor, st)
}

func applyAccumulated(s *normalize.Session, sid, vendor string) {
	st := sessionstate.Load(sid, vendor)
	if st == nil {
		return
	}
	if s.InputTokens == 0 {
		s.InputTokens = st.InputTokens
	}
	if s.OutputTokens == 0 {
		s.OutputTokens = st.OutputTokens
	}
	if s.TotalTokens == 0 {
		s.TotalTokens = s.InputTokens + s.OutputTokens
	}
	if s.CostUSD == 0 {
		s.CostUSD = st.CostUSD
	}
	if s.Model == "" {
		s.Model = st.Model
	}
}

func capture(in normalize.Input, s string) string {
	if in.ContentCapture != "full" {
		return ""
	}
	return s
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
