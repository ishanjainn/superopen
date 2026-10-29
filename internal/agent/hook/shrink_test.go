package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplaceToolOutputShrinksLogAndKeepsFailure(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".so"), 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for i := 0; i < 400; i++ {
		b.WriteString("INFO repeated build progress " + strings.Repeat("x", 40) + "\n")
	}
	b.WriteString("ERROR important failure at pkg/foo.go:42\n")
	payload, err := json.Marshal(map[string]any{
		"cwd":        root,
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": "go test ./..."},
		"tool_response": map[string]any{
			"stdout":      b.String(),
			"stderr":      "IMPORTANT ERROR",
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
	if !strings.Contains(stdout, "recover: so retrieve --raw ccr_") {
		t.Fatalf("missing handle: %s", stdout[:80])
	}
	if !strings.Contains(stdout, "content_type: log\noriginal_bytes:") {
		t.Fatalf("missing compact result metadata: %s", stdout[:80])
	}
	if !strings.Contains(stdout, "ERROR important failure at pkg/foo.go:42") {
		t.Fatal("failure line disappeared")
	}
	if _, ok := obj["interrupted"]; !ok {
		t.Fatal("object shape dropped interrupted")
	}
	if obj["stderr"] != "IMPORTANT ERROR" {
		t.Fatal("stderr changed")
	}
	if len(stdout) >= len(b.String()) {
		t.Fatal("replacement is not smaller")
	}
}

func TestReplaceToolOutputPreservesSourceAndErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".so"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"cat source.go", "git diff", "rg -n foo .", "go testing"} {
		payload, _ := json.Marshal(map[string]any{
			"cwd": root, "tool_name": "Bash",
			"tool_input":    map[string]any{"command": command},
			"tool_response": map[string]any{"stdout": strings.Repeat("func important() {}\n", 400)},
		})
		if _, ok := replaceToolOutput(payload); ok {
			t.Fatalf("source output changed for %q", command)
		}
	}
	payload, _ := json.Marshal(map[string]any{
		"cwd": root, "tool_name": "Bash",
		"tool_input":    map[string]any{"command": "go test ./..."},
		"tool_response": map[string]any{"stdout": strings.Repeat("ok pkg\n", 700)},
	})
	if _, ok := replaceToolOutput(payload); ok {
		t.Fatal("ordinary test text must stay complete")
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

func TestPlainTextHostReplacement(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".so"), 0o755); err != nil {
		t.Fatal(err)
	}
	var log strings.Builder
	for i := 0; i < 400; i++ {
		log.WriteString("INFO repeated build progress " + strings.Repeat("x", 40) + "\n")
	}
	log.WriteString("ERROR failure at pkg/foo.go:42\n")
	for _, tc := range []struct{ vendor, event, key string }{
		{"opencode", "tool.execute.after", "tool_result"},
		{"cline", "transformToolResult", "tool_response"},
		{"hermes", "transformToolResult", "tool_response"},
		{"pi", "tool_result", "tool_response"},
		{"omp", "tool_result", "tool_response"},
		{"prime", "tool_result", "tool_response"},
		{"senpi", "tool_result", "tool_response"},
	} {
		t.Run(tc.vendor, func(t *testing.T) {
			payload, _ := json.Marshal(map[string]any{
				"cwd": root, "tool_name": "bash",
				"tool_input": map[string]any{"command": "go test ./..."},
				tc.key:       log.String(),
			})
			decision, ok := steerDecisionFor(tc.vendor, tc.event, "", payload)
			if !ok {
				t.Fatal("expected replacement")
			}
			replacement, ok := decision.replace.(string)
			if !ok || !strings.Contains(replacement, "ERROR failure at pkg/foo.go:42") ||
				!strings.Contains(replacement, "recover: so retrieve --raw ccr_") || len(replacement) >= log.Len() {
				t.Fatalf("bad replacement: %q", replacement)
			}
		})
	}
	for _, command := range []string{"cat source.go", "rg foo", "so graph query symbols"} {
		payload, _ := json.Marshal(map[string]any{
			"cwd": root, "tool_name": "bash",
			"tool_input":  map[string]any{"command": command},
			"tool_result": log.String(),
		})
		if _, ok := replaceTextToolOutput(payload); ok {
			t.Fatalf("changed source result for %q", command)
		}
	}
	copilotPayload, _ := json.Marshal(map[string]any{
		"cwd": root, "toolName": "bash", "toolArgs": map[string]any{"command": "go test ./..."},
		"toolResult": map[string]any{"resultType": "success", "textResultForLlm": log.String()},
	})
	decision, ok := steerDecisionFor("copilot-cli", "postToolUse", "", copilotPayload)
	if !ok || !strings.Contains(decision.replace.(string), "recover: so retrieve --raw") {
		t.Fatal("Copilot successful shell result did not shrink")
	}
}

func TestPayloadEventForExtensionHooks(t *testing.T) {
	for input, want := range map[string]string{
		`{"type":"tool_result"}`:                     "tool_result",
		`{"event":"session_start"}`:                  "session_start",
		`{"type":" tool_result ","event":"ignored"}`: "tool_result",
		`{`: "",
	} {
		if got := payloadEvent([]byte(input)); got != want {
			t.Fatalf("payloadEvent(%s) = %q, want %q", input, got, want)
		}
	}
}

func TestExtraHarnessStartIncludesManualEfficiencyCommands(t *testing.T) {
	root := t.TempDir()
	writeHookSession(t, root, "extra-start")
	writeHookSourceFile(t, root)
	payload, _ := json.Marshal(map[string]any{"cwd": root, "session_id": "extra-start"})
	for _, vendor := range []string{"cline", "hermes", "openclaw", "qwen"} {
		text := sessionStartText(payload, vendor)
		if !strings.Contains(text, "so shrink -- <command>") || !strings.Contains(text, "so blocks scan") {
			t.Fatalf("%s start guidance = %q", vendor, text)
		}
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
	if !ok || d.deny || d.text != "save full command output, then run so blocks run first-error <file>" {
		t.Fatalf("hint: ok=%v deny=%v text=%q", ok, d.deny, d.text)
	}
}

func TestGraphNudgePrecedesBlockHint(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	writeHookSession(t, root, "block-graph")
	writeHookSourceFile(t, root)
	payload, err := json.Marshal(map[string]any{
		"session_id": "block-graph",
		"cwd":        root,
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": `python3 -c 'import json; print(json.load(open("pkg/foo.go")))'`},
	})
	if err != nil {
		t.Fatal(err)
	}
	d, ok := steerDecisionFor("claude-code", "PreToolUse", "search", payload)
	if !ok || !strings.Contains(d.text, "graph query") || strings.Contains(d.text, "so blocks") {
		t.Fatalf("graph guidance lost to block hint: ok=%v text=%q", ok, d.text)
	}
}
