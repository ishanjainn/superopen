package hook

import (
	"encoding/json"

	"github.com/ishanjainn/superopen/internal/compact"
)

// replaceToolOutput returns a same-shaped tool result when the bytes shrink
// and the original is stored. It returns false to leave the host output alone.
func replaceToolOutput(payload []byte) (any, bool) {
	if compact.ProtectCommand(bashCommandFromPayload(payload)) {
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
	case string:
		next, ok := shrinkText(root, v)
		if !ok {
			return nil, false
		}
		return next, true
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, val := range v {
			out[k] = val
		}
		changed := false
		for _, key := range []string{"stdout", "stderr", "output", "content"} {
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

func shrinkText(root, text string) (string, bool) {
	out := compact.Compress(root, []byte(text), "")
	if out.PassedThrough || out.Handle == "" || len(out.Output) >= len(text) {
		return "", false
	}
	wrapped := "recover so retrieve " + out.Handle + "\n" + string(out.Output)
	if len(wrapped) >= len(text) {
		return "", false
	}
	return wrapped, true
}
