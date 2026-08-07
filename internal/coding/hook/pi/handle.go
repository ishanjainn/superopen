package pi

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/coding/normalize"
	"github.com/ishanjainn/superopen/internal/coding/pricing"
	"github.com/ishanjainn/superopen/internal/coding/sessionstate"
)

type payload struct {
	Type        string `json:"type"`
	Event       string `json:"event"`
	SessionID   string `json:"session_id"`
	SessionFile string `json:"session_file"`
	CWD         string `json:"cwd"`
	Prompt      string `json:"prompt"`
	Text        string `json:"text"`
	Model       string `json:"model"`
	Role        string `json:"role"`
	ToolName    string `json:"toolName"`
	ToolCallID  string `json:"toolCallId"`
	IsError     bool   `json:"isError"`
	Input       json.RawMessage `json:"input"`
	Result      string `json:"result"`
	Usage *struct {
		Input      int64 `json:"input"`
		Output     int64 `json:"output"`
		CacheRead  int64 `json:"cacheRead"`
		CacheWrite int64 `json:"cacheWrite"`
		TotalTokens int64 `json:"totalTokens"`
		Cost *struct {
			Total float64 `json:"total"`
		} `json:"cost"`
	} `json:"usage"`
	// Compaction / summary metadata
	SummaryKind string `json:"summary_kind"`
	TokensBefore int64 `json:"tokensBefore"`
}

func handle(in normalize.Input) error {
	var p payload
	_ = json.Unmarshal(in.Payload, &p)
	event := strings.ToLower(firstNonEmpty(in.Event, p.Type, p.Event))
	sid := p.SessionID
	if sid == "" && p.SessionFile != "" {
		// derive from path basename without extension
		base := p.SessionFile
		if i := strings.LastIndexByte(base, '/'); i >= 0 {
			base = base[i+1:]
		}
		sid = strings.TrimSuffix(base, ".jsonl")
	}
	if sid == "" {
		sid = "unknown"
	}
	now := time.Now().UTC()
	model := p.Model

	switch {
	case event == "session_start" || event == "sessionstart":
		return in.Emit.EmitSession(normalize.Session{
			SessionID: sid, Vendor: in.Vendor, Model: model, CWD: p.CWD, StartedAt: now,
			Extras: map[string]string{"pi.session_file": p.SessionFile},
		})

	case event == "session_shutdown" || event == "agent_end" || event == "sessionend" || event == "session_end":
		s := normalize.Session{
			SessionID: sid, Vendor: in.Vendor, Model: model, CWD: p.CWD, EndedAt: now,
			Outcome: "completed",
		}
		applyAccumulated(&s, sid, in.Vendor)
		return in.Emit.EmitSession(s)

	case event == "before_agent_start" || event == "turn_start":
		return in.Emit.EmitLLMTurn(normalize.LLMTurn{
			SessionID: sid, Vendor: in.Vendor, Model: model, StartedAt: now,
			Prompt: capture(in, p.Prompt),
		})

	case event == "message_end" || event == "message_update":
		turn := normalize.LLMTurn{
			SessionID: sid, Vendor: in.Vendor, Model: model,
			StartedAt: now.Add(-time.Second), EndedAt: now,
			Response: capture(in, p.Text), AssistantMessageOnly: true,
		}
		stampUsage(&turn, p, model)
		accumulate(sid, in.Vendor, turn)
		return in.Emit.EmitLLMTurn(turn)

	case event == "tool_call" || event == "tool_execution_start":
		return in.Emit.EmitEvent(normalize.EventEmission{
			SessionID: sid, Name: "coding_agent.tool.requested", At: now,
			Attrs: map[string]any{
				"coding_agent.client": in.Vendor,
				"gen_ai.tool.name":    firstNonEmpty(p.ToolName),
				"gen_ai.tool.call.id": p.ToolCallID,
				"code.cwd":            p.CWD,
			},
		})

	case event == "tool_execution_end":
		return in.Emit.EmitToolCall(normalize.ToolCall{
			SessionID: sid, Vendor: in.Vendor, Model: model,
			ToolName: p.ToolName, ToolUseID: p.ToolCallID, WorkingDir: p.CWD,
			StartedAt: now.Add(-50 * time.Millisecond), EndedAt: now,
			Errored: p.IsError,
			Args:    capture(in, string(p.Input)),
			Result:  capture(in, p.Result),
		})

	case event == "compaction" || event == "branch_summary" || p.SummaryKind != "":
		turn := normalize.LLMTurn{
			SessionID: sid, Vendor: in.Vendor, Model: model,
			StartedAt: now, EndedAt: now, AssistantMessageOnly: true,
			Extras: map[string]string{
				"pi.summary_kind": firstNonEmpty(p.SummaryKind, event),
			},
		}
		if p.TokensBefore > 0 {
			turn.Extras["pi.tokens_before"] = itoa(p.TokensBefore)
		}
		stampUsage(&turn, p, model)
		accumulate(sid, in.Vendor, turn)
		return in.Emit.EmitLLMTurn(turn)

	default:
		if p.Prompt != "" {
			return in.Emit.EmitLLMTurn(normalize.LLMTurn{
				SessionID: sid, Vendor: in.Vendor, Model: model, StartedAt: now,
				Prompt: capture(in, p.Prompt),
			})
		}
		return nil
	}
}

func stampUsage(turn *normalize.LLMTurn, p payload, model string) {
	if p.Usage == nil {
		return
	}
	u := p.Usage
	turn.InputTokens = int64(u.Input) + int64(u.CacheRead) + int64(u.CacheWrite)
	turn.OutputTokens = int64(u.Output)
	turn.CacheReadTokens = int64(u.CacheRead)
	turn.CacheCreationTokens = int64(u.CacheWrite)
	if u.TotalTokens > 0 {
		turn.TotalTokens = int64(u.TotalTokens)
	} else {
		turn.TotalTokens = turn.InputTokens + turn.OutputTokens
	}
	// Prefer host-reported cost (Pi prices calls accurately).
	if u.Cost != nil {
		turn.CostUSD = u.Cost.Total
		return
	}
	if rate := pricing.Lookup(firstNonEmpty(model, turn.Model)); rate.InputPer1M > 0 || rate.OutputPer1M > 0 {
		turn.CostUSD = rate.Cost(turn.InputTokens, turn.OutputTokens, turn.CacheReadTokens, turn.CacheCreationTokens)
	}
}

func accumulate(sid, vendor string, turn normalize.LLMTurn) {
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

func itoa(n int64) string {
	return strings.TrimSpace(strings.Replace(jsonNumber(n), "\n", "", -1))
}

func jsonNumber(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
