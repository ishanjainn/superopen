package session

import (
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/session/trace"
)

// CountTurnsFromSpans returns a vendor-neutral turn count.
// Keep in sync with web/src/lib/so/sessions.ts countTurnsFromSpans.
//
// When coding_agent.turn.id / generation_id is present, unique ids win
// (loop.stop + llm.turn on the same id count as one). Otherwise count
// loop.stop, then llm.turn/completion, then user-prompt markers.
func CountTurnsFromSpans(spans []trace.Span) int {
	seen := map[string]struct{}{}
	var stops, llms, prompts int
	for _, sp := range spans {
		if !isTurnMarker(sp) {
			continue
		}
		if id := turnID(sp); id != "" {
			seen[id] = struct{}{}
			continue
		}
		name := strings.ToLower(sp.Name)
		switch {
		case isLoopStopName(name):
			stops++
		case isLLMTurnName(name):
			llms++
		default:
			prompts++
		}
	}
	if len(seen) > 0 {
		return len(seen)
	}
	if stops > 0 {
		return stops
	}
	if llms > 0 {
		return llms
	}
	return prompts
}

// TurnsFor returns persisted turns, or scans this session's events.jsonl when
// the document still has 0 (legacy files, nested ids never listed).
func (s *Store) TurnsFor(id string, persisted int) int {
	if persisted > 0 {
		return persisted
	}
	return CountTurnsFromSpans(loadJSONLSpans(filepath.Join(s.Paths.SessionDir(id), "events.jsonl")))
}

func turnID(sp trace.Span) string {
	if sp.Attributes == nil {
		return ""
	}
	if v := strings.TrimSpace(sp.Attributes["coding_agent.turn.id"]); v != "" {
		return v
	}
	return strings.TrimSpace(sp.Attributes["generation_id"])
}

func isTurnMarker(sp trace.Span) bool {
	name := strings.ToLower(sp.Name)
	if isLoopStopName(name) || isLLMTurnName(name) {
		return true
	}
	if strings.Contains(name, "user_prompt") || strings.Contains(name, "user.prompt") {
		return true
	}
	attrs := sp.Attributes
	if attrs == nil {
		return false
	}
	if attrs["gen_ai.prompt"] != "" || attrs["gen_ai.content.prompt"] != "" {
		return true
	}
	raw := strings.ToLower(attrs["gen_ai.input.messages"])
	return strings.Contains(raw, `"role":"user"`) ||
		strings.Contains(raw, `"role": "user"`) ||
		strings.Contains(raw, `"role":"user_prompt"`)
}

func isLoopStopName(name string) bool {
	return name == "coding_agent.session.loop.stop" || strings.HasSuffix(name, ".loop.stop")
}

func isLLMTurnName(name string) bool {
	return name == "coding_agent.llm.turn" || strings.Contains(name, "completion")
}
