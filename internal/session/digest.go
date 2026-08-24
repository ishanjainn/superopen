package session

import (
	"strings"

	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/session/trace"
)

// Digest is a compact session summary for harvest/distill prompts.
// Traces and session.json only — not full jsonl, not playbooks.
func Digest(root, sessionID string) string {
	layout := paths.Resolve(root)
	meta, err := NewStore(layout).Get(sessionID)
	title := sessionID
	if err == nil {
		title = DisplayName(meta)
	}
	var lines []string
	lines = append(lines, "Title: "+clipDigest(title, 120))
	if err == nil {
		if p := strings.TrimSpace(meta.PromptPreview); p != "" && p != title {
			lines = append(lines, "Prompt: "+clipDigest(p, 160))
		}
	}
	spans, _ := trace.NewLocalJSONL(layout.TracesDir).Query(trace.QueryFilter{SessionID: sessionID, Limit: 24})
	tools := 0
	for _, sp := range spans {
		if sp.Attributes == nil {
			continue
		}
		if p := strings.TrimSpace(firstAttr(sp.Attributes, "gen_ai.prompt", "gen_ai.content.prompt")); p != "" {
			lines = append(lines, "- "+clipDigest(p, 80))
		}
		path := strings.TrimSpace(firstAttr(sp.Attributes, "coding_agent.file_path", "coding_agent.file.path"))
		tool := strings.TrimSpace(firstAttr(sp.Attributes, "gen_ai.tool.name", "coding_agent.tool.name"))
		if tool != "" || path != "" {
			line := "- tool"
			if tool != "" {
				line += " " + clipDigest(tool, 32)
			}
			if path != "" {
				line += " " + clipDigest(path, 48)
			}
			lines = append(lines, line)
			tools++
		}
		if len(lines) >= 14 {
			break
		}
	}
	return strings.Join(lines, "\n")
}

// HasActivity is true when the session has real turns or tool work.
func HasActivity(root, sessionID string) bool {
	layout := paths.Resolve(root)
	st := NewStore(layout)
	if !st.IsEmpty(sessionID) {
		return true
	}
	spans, err := trace.NewLocalJSONL(layout.TracesDir).Query(trace.QueryFilter{SessionID: sessionID, Limit: 32})
	if err != nil {
		return false
	}
	return SpansHaveActivity(spans)
}

func clipDigest(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func firstAttr(attrs map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(attrs[k]); v != "" {
			return v
		}
	}
	return ""
}
