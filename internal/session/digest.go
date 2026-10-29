package session

import (
	"encoding/json"
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
	lines := []string{"Title: " + clipDigest(title, 120)}
	spans, _ := trace.NewLocalJSONL(layout.TracesDir).Query(trace.QueryFilter{SessionID: sessionID, Limit: 24})
	var prompts, toolLines []string
	seenPrompts := make(map[string]bool)
	for _, sp := range spans {
		if sp.Attributes == nil {
			continue
		}
		p := strings.TrimSpace(firstAttr(sp.Attributes, "gen_ai.prompt", "gen_ai.content.prompt"))
		if p == "" {
			p = inputMessagePrompt(sp.Attributes["gen_ai.input.messages"])
		}
		if p != "" && !seenPrompts[p] && len(prompts) < 3 {
			seenPrompts[p] = true
			prompts = append(prompts, p)
		}
		path := strings.TrimSpace(firstAttr(sp.Attributes, "coding_agent.file_path", "coding_agent.file.path"))
		tool := strings.TrimSpace(firstAttr(sp.Attributes, "gen_ai.tool.name", "coding_agent.tool.name"))
		if (tool != "" || path != "") && len(toolLines) < 10 {
			line := "- tool"
			if tool != "" {
				line += " " + clipDigest(tool, 32)
			}
			if path != "" {
				line += " " + clipDigest(path, 48)
			}
			toolLines = append(toolLines, line)
		}
	}
	if len(prompts) == 0 && err == nil && strings.TrimSpace(meta.PromptPreview) != "" {
		prompts = append(prompts, meta.PromptPreview)
	}
	for _, p := range prompts {
		lines = append(lines, "Prompt: "+clipDigest(p, 640))
	}
	lines = append(lines, toolLines...)
	return strings.Join(lines, "\n")
}

func inputMessagePrompt(raw string) string {
	var messages []struct {
		Role  string `json:"role"`
		Parts []struct {
			Type    string `json:"type"`
			Content string `json:"content"`
		} `json:"parts"`
	}
	if json.Unmarshal([]byte(raw), &messages) != nil {
		return ""
	}
	for _, message := range messages {
		if message.Role != "user" {
			continue
		}
		for _, part := range message.Parts {
			if part.Type == "text" && strings.TrimSpace(part.Content) != "" {
				return strings.TrimSpace(part.Content)
			}
		}
	}
	return ""
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
