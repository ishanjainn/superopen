package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsExploreToolExcludesShellAndListing(t *testing.T) {
	for _, name := range []string{"Grep", "glob", "Read", "codebase_search"} {
		if !isExploreTool(name) {
			t.Fatalf("%s should be an explore tool", name)
		}
	}
	// These carry no term the graph can rank; augmenting them was the bulk
	// of the old nudge's per-session token cost.
	for _, name := range []string{"Bash", "shell", "ls", "list_dir", "Edit", "Write", "TodoWrite", ""} {
		if isExploreTool(name) {
			t.Fatalf("%s should not be an explore tool", name)
		}
	}
}

func TestSearchTermFromPayloadStripsRegexSyntax(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    string
	}{
		{
			name:    "grep pattern keeps the identifier",
			payload: `{"tool_name":"Grep","tool_input":{"pattern":".*HandleRequest.*"}}`,
			want:    "HandleRequest",
		},
		{
			name:    "dotted symbol survives intact",
			payload: `{"tool_name":"Grep","tool_input":{"pattern":"engine.Query"}}`,
			want:    "engine.Query",
		},
		{
			name:    "read falls back to the file stem",
			payload: `{"tool_name":"Read","tool_input":{"file_path":"/repo/internal/api/handler.go"}}`,
			want:    "handler",
		},
		{
			name:    "pure punctuation yields nothing",
			payload: `{"tool_name":"Grep","tool_input":{"pattern":"^\\s+$"}}`,
			want:    "",
		},
		{
			name:    "short terms are skipped",
			payload: `{"tool_name":"Grep","tool_input":{"pattern":"id"}}`,
			want:    "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := searchTermFromPayload([]byte(tc.payload)); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExploreAugmentSilentForNonExploreTool(t *testing.T) {
	payload := []byte(`{"tool_name":"Bash","tool_input":{"command":"ls -la"},"cwd":"/tmp"}`)
	if got := exploreAugment(payload, "claude-code"); got != "" {
		t.Fatalf("expected silence for Bash, got %q", got)
	}
}

func TestExploreAugmentSilentWithoutGraph(t *testing.T) {
	// t.TempDir has no .so database, so the hook must add nothing rather
	// than emitting an unconditional reminder.
	payload := []byte(`{"tool_name":"Grep","tool_input":{"pattern":"HandleRequest"},"cwd":"` + t.TempDir() + `"}`)
	if got := exploreAugment(payload, "claude-code"); got != "" {
		t.Fatalf("expected silence without a graph, got %q", got)
	}
}

func TestSteerTextForIgnoresEditTools(t *testing.T) {
	payload := []byte(`{"tool_name":"Edit","session_id":"s1","tool_input":{"file_path":"/repo/main.go"}}`)
	if _, _, ok := steerTextFor("claude-code", "PreToolUse", payload); ok {
		t.Fatal("edit tools must not receive steer context")
	}
}

func TestSteerTextForUnknownVendorStaysSilent(t *testing.T) {
	payload := []byte(`{"tool_name":"Grep","tool_input":{"pattern":"Handler"}}`)
	if _, _, ok := steerTextFor("unknown-vendor", "PreToolUse", payload); ok {
		t.Fatal("unknown vendors must not receive steer context")
	}
}

func TestSessionReminderClaimedOncePerSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	payload, err := json.Marshal(map[string]any{
		"session_id": "steer-test-session",
		"cwd":        t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	first, _, ok := steerTextFor("claude-code", "SessionStart", payload)
	if !ok || !strings.Contains(first, "Superopen") {
		t.Fatalf("first session event should carry the reminder, got %q", first)
	}
	if _, _, ok := steerTextFor("claude-code", "UserPromptSubmit", payload); ok {
		t.Fatal("reminder must not repeat on every prompt in the same session")
	}
}

func TestPreCompactInjectsWorkingSnapshotFailOpen(t *testing.T) {
	payload, err := json.Marshal(map[string]any{
		"session_id": "compact-session",
		"cwd":        t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	text, ev, ok := steerTextFor("cursor", "preCompact", payload)
	if ok && text == "" {
		t.Fatal("ok with empty text")
	}
	if ev != "preCompact" && ok {
		t.Fatalf("event=%s", ev)
	}
	_ = text
}

func TestMemoryPackOnceAndDistillAsk(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	id := "mem-pack-session"
	writeHookSession(t, root, id)
	payload, err := json.Marshal(map[string]any{"session_id": id, "cwd": root, "prompt": "login"})
	if err != nil {
		t.Fatal(err)
	}
	first, _, ok := steerTextFor("cursor", "sessionStart", payload)
	if !ok {
		t.Fatal("expected session start context")
	}
	if !strings.Contains(first, "Superopen") {
		t.Fatalf("missing graph reminder: %q", first)
	}
	second, _, ok := steerTextFor("cursor", "beforeSubmitPrompt", payload)
	if ok {
		t.Fatalf("memory/graph pack must be once, got %q", second)
	}
}

func writeHookSession(t *testing.T, root, id string) {
	t.Helper()
	// Best-effort ingest so pack has content; empty store still fail-opens.
	_ = os.MkdirAll(filepath.Join(root, ".so", "sessions", id), 0o755)
}
