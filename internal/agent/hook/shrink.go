package hook

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ishanjainn/superopen/internal/compact"
	"github.com/ishanjainn/superopen/third_party/caveman/engine"
)

// replaceToolOutput returns a same-shaped tool result when the bytes shrink
// and the original is stored. It returns false to leave the host output alone.
func replaceToolOutput(payload []byte) (any, bool) {
	// Only Bash output has a stable shape that Claude can replace. Restrict
	// automatic loss to noisy commands; source reads, diffs and graph answers
	// need their complete evidence for a correct edit.
	if !strings.EqualFold(toolNameFromPayload(payload), "Bash") ||
		!autoShrinkCommand(bashCommandFromPayload(payload)) {
		return nil, false
	}
	root := hookRepoRoot(payload)
	if root == "" {
		return nil, false
	}
	var probe map[string]any
	if json.Unmarshal(payload, &probe) != nil {
		return nil, false
	}
	raw, ok := probe["tool_response"]
	if !ok || raw == nil {
		return nil, false
	}
	switch v := raw.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, val := range v {
			out[k] = val
		}
		changed := false
		for _, key := range []string{"stdout", "output"} {
			s, ok := out[key].(string)
			if !ok {
				continue
			}
			next, ok := shrinkText(root, s)
			if !ok {
				continue
			}
			out[key] = next
			changed = true
		}
		if !changed {
			return nil, false
		}
		return out, true
	default:
		return nil, false
	}
}

// replaceTextToolOutput handles hosts whose shell result is a plain string.
// Their plugins apply this response only to the matching result text.
func replaceTextToolOutput(payload []byte) (string, bool) {
	name := toolNameFromPayload(payload)
	if !strings.EqualFold(name, "bash") && !strings.EqualFold(name, "shell") && !strings.EqualFold(name, "exec") {
		return "", false
	}
	if !autoShrinkCommand(bashCommandFromPayload(payload)) {
		return "", false
	}
	root := hookRepoRoot(payload)
	if root == "" {
		return "", false
	}
	var probe map[string]any
	if json.Unmarshal(payload, &probe) != nil {
		return "", false
	}
	for _, key := range []string{"tool_result", "tool_response"} {
		if original, ok := probe[key].(string); ok {
			return shrinkText(root, original)
		}
	}
	if result, ok := probe["toolResult"].(map[string]any); ok && result["resultType"] == "success" {
		if original, ok := result["textResultForLlm"].(string); ok {
			return shrinkText(root, original)
		}
	}
	return "", false
}

func shrinkText(root, text string) (string, bool) {
	if len(text) < 2048 {
		return "", false
	}
	switch engine.New(nil, nil).Detect([]byte(text)) {
	case engine.TypeLog, engine.TypeTestReport:
	default:
		return "", false
	}
	out := compact.Compress(root, []byte(text), "")
	if out.PassedThrough || out.Handle == "" || len(out.Output) >= len(text) {
		return "", false
	}
	wrapped := fmt.Sprintf("content_type: %s\noriginal_bytes: %d\nrecover: so retrieve --raw %s\ncontent:\n%s", out.ContentType, len(text), out.Handle, out.Output)
	if len(wrapped) >= len(text) {
		return "", false
	}
	return wrapped, true
}

// A narrow command allowlist keeps automatic compression out of file reads,
// search results, source diffs and graph output. Other data can be shrunk on
// demand with `so shrink` after the agent has decided the loss is acceptable.
func autoShrinkCommand(command string) bool {
	command = strings.TrimSpace(command)
	if compact.ProtectCommand(command) || command == "" {
		return false
	}
	for _, name := range []string{
		"go test", "pytest", "python -m pytest", "npm test", "pnpm test",
		"yarn test", "cargo test", "bun test", "docker logs", "kubectl logs", "journalctl",
	} {
		if command == name || strings.HasPrefix(command, name+" ") {
			return true
		}
	}
	return false
}
