package hook

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ishanjainn/superopen/internal/agent/steer"
)

// emitSteerContext writes vendor-specific additionalContext when the event
// supports it. Unknown protocols get no stdout (fail-open).
func emitSteerContext(vendor, event string, payload []byte) {
	text, hookEvent, ok := steerTextFor(vendor, event, payload)
	if !ok || text == "" {
		return
	}
	var body any
	switch vendor {
	case "claude-code":
		body = map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName":     hookEvent,
				"additionalContext": text,
			},
		}
	case "cursor":
		body = map[string]any{"additional_context": text}
	case "codex":
		body = map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName":     hookEvent,
				"additionalContext": text,
			},
		}
	case "gemini", "copilot-cli":
		// Best-effort shared shape used by several CLI hosts.
		body = map[string]any{"additionalContext": text}
	default:
		return
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(body); err != nil {
		fmt.Fprintf(os.Stderr, "so hook: steer stdout: %v\n", err)
	}
}

func steerTextFor(vendor, event string, payload []byte) (text, hookEvent string, ok bool) {
	ev := strings.TrimSpace(event)
	switch vendor {
	case "claude-code":
		switch ev {
		case "SessionStart", "UserPromptSubmit", "SubagentStart":
			return steer.HookReminder(), ev, true
		case "PreToolUse":
			tool := toolNameFromPayload(payload)
			if isExploreTool(tool) {
				return steer.ExploreNudge(tool), ev, true
			}
		}
	case "cursor":
		switch ev {
		case "sessionStart", "beforeSubmitPrompt", "subagentStart":
			return steer.HookReminder(), ev, true
		case "preToolUse", "beforeReadFile":
			tool := toolNameFromPayload(payload)
			if tool == "" || isExploreTool(tool) {
				return steer.ExploreNudge(firstNonEmpty(tool, "Read")), ev, true
			}
		}
	case "codex":
		switch ev {
		case "SessionStart", "UserPromptSubmit":
			return steer.HookReminder(), ev, true
		case "PreToolUse":
			tool := toolNameFromPayload(payload)
			if isExploreTool(tool) {
				return steer.ExploreNudge(tool), ev, true
			}
		}
	case "gemini":
		switch strings.ToLower(ev) {
		case "sessionstart", "beforeagent":
			return steer.HookReminder(), ev, true
		case "beforetool":
			tool := toolNameFromPayload(payload)
			if isExploreTool(tool) {
				return steer.ExploreNudge(tool), ev, true
			}
		}
	case "copilot-cli":
		switch ev {
		case "sessionStart", "userPromptSubmitted":
			return steer.HookReminder(), ev, true
		case "preToolUse":
			tool := toolNameFromPayload(payload)
			if isExploreTool(tool) {
				return steer.ExploreNudge(tool), ev, true
			}
		}
	}
	return "", "", false
}

func toolNameFromPayload(payload []byte) string {
	var m map[string]any
	if json.Unmarshal(payload, &m) != nil {
		return ""
	}
	for _, key := range []string{"tool_name", "toolName", "name", "tool"} {
		if v, ok := m[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func isExploreTool(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "grep", "glob", "read", "readfile", "search", "semanticsearch",
		"ripgrep", "find", "list_dir", "listdir", "ls", "bash", "shell":
		return true
	default:
		return strings.Contains(n, "grep") || strings.Contains(n, "glob") || strings.Contains(n, "search")
	}
}
