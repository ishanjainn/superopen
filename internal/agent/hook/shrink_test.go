package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplaceToolOutputShrinksJSON(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".so"), 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString("[\n")
	for i := 0; i < 400; i++ {
		if i > 0 {
			b.WriteString(",\n")
		}
		b.WriteString(`{"id":1,"name":"row","noise":"` + strings.Repeat("x", 40) + `"}`)
	}
	b.WriteString("\n]\n")
	payload, err := json.Marshal(map[string]any{
		"cwd":        root,
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": "cat noise.json"},
		"tool_response": map[string]any{
			"stdout":      b.String(),
			"stderr":      "",
			"interrupted": false,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := replaceToolOutput(payload)
	if !ok {
		t.Fatal("expected a replacement")
	}
	obj, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("shape: %T", got)
	}
	stdout, _ := obj["stdout"].(string)
	if !strings.Contains(stdout, "recover so retrieve ccr_") {
		t.Fatalf("missing handle: %s", stdout[:80])
	}
	if _, ok := obj["interrupted"]; !ok {
		t.Fatal("object shape dropped interrupted")
	}
	if len(stdout) >= len(b.String()) {
		t.Fatal("replacement is not smaller")
	}
}

func TestReplaceToolOutputLeavesGraph(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".so"), 0o755); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"cwd":        root,
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": "so graph query \"main\""},
		"tool_response": map[string]any{
			"stdout": strings.Repeat("NODE a\n", 800),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := replaceToolOutput(payload); ok {
		t.Fatal("graph stdout must stay original")
	}
}

func TestBlockHintDoesNotDeny(t *testing.T) {
	root := t.TempDir()
	writeHookSession(t, root, "blk")
	payload, err := json.Marshal(map[string]any{
		"session_id": "blk",
		"cwd":        root,
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": "go test ./... | tail -20"},
	})
	if err != nil {
		t.Fatal(err)
	}
	d, ok := steerDecisionFor("claude-code", "PreToolUse", "", payload)
	if !ok || d.deny || d.text != "use so blocks run test-summary" {
		t.Fatalf("hint: ok=%v deny=%v text=%q", ok, d.deny, d.text)
	}
}
