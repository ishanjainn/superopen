package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/engine"
)

func TestIsExploreToolExcludesShellAndListing(t *testing.T) {
	for _, name := range []string{"Grep", "glob", "Read", "codebase_search", "Bash"} {
		if !isExploreTool(name) {
			t.Fatalf("%s should be an explore tool", name)
		}
	}
	for _, name := range []string{"ls", "list_dir", "Edit", "Write", "TodoWrite", ""} {
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
			name:    "bash grep extracts the identifier",
			payload: `{"tool_name":"Bash","tool_input":{"command":"grep -r dashboardWatcher --include='*.ts' | head"}}`,
			want:    "dashboardWatcher",
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

func TestSessionReminderSilentWhenUnmanaged(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	payload, err := json.Marshal(map[string]any{
		"session_id": "steer-unmanaged-session",
		"cwd":        t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if text, _, ok := steerTextFor("claude-code", "SessionStart", payload); ok {
		t.Fatalf("unmanaged session must not inject Superopen context, got %q", text)
	}
}

func TestSessionStartHasNoAdditionalContext(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	writeHookSession(t, root, "steer-test-session")
	payload, err := json.Marshal(map[string]any{
		"session_id": "steer-test-session",
		"cwd":        root,
	})
	if err != nil {
		t.Fatal(err)
	}
	if text, _, ok := steerTextFor("claude-code", "SessionStart", payload); ok {
		t.Fatalf("SessionStart must not inject always-on steer context, got %q", text)
	}
}

func TestSubagentStartInjectsHookReminder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	writeHookSession(t, root, "steer-subagent-session")
	payload, err := json.Marshal(map[string]any{
		"session_id": "steer-subagent-session",
		"cwd":        root,
	})
	if err != nil {
		t.Fatal(err)
	}
	text, ev, ok := steerTextFor("claude-code", "SubagentStart", payload)
	if !ok {
		t.Fatal("SubagentStart must inject HookReminder for Explore children")
	}
	if ev != "SubagentStart" {
		t.Fatalf("hookEvent = %q, want SubagentStart", ev)
	}
	if !strings.Contains(text, "so graph query") {
		t.Fatalf("SubagentStart text missing graph reminder: %q", text)
	}
	cursorText, cursorEv, cursorOK := steerTextFor("cursor", "subagentStart", payload)
	if !cursorOK || cursorEv != "subagentStart" || !strings.Contains(cursorText, "so graph query") {
		t.Fatalf("cursor subagentStart must inject the same reminder, got ok=%v ev=%q text=%q", cursorOK, cursorEv, cursorText)
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

func TestMemoryPackNotOnSessionStart(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	id := "mem-pack-session"
	writeHookSession(t, root, id)
	payload, err := json.Marshal(map[string]any{"session_id": id, "cwd": root, "prompt": "login"})
	if err != nil {
		t.Fatal(err)
	}
	if first, _, ok := steerTextFor("cursor", "sessionStart", payload); ok {
		t.Fatalf("sessionStart must not inject Superopen context, got %q", first)
	}
}

func TestGrepPreToolUseEmitsMandatory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	writeHookSession(t, root, "grep-session")
	payload, err := json.Marshal(map[string]any{
		"tool_name":  "Grep",
		"tool_input": map[string]any{"pattern": "HandleRequest"},
		"cwd":        root,
		"session_id": "grep-session",
	})
	if err != nil {
		t.Fatal(err)
	}
	text, _, ok := steerTextFor("claude-code", "PreToolUse", payload)
	if !ok || !strings.Contains(text, "MANDATORY") {
		t.Fatalf("grep should get a MANDATORY graph-first nudge, got %q", text)
	}
	if strings.Contains(text, "Superopen graph:") || strings.Contains(text, "hit(s) for") {
		t.Fatalf("grep must not receive ExploreAugment hit lists, got %q", text)
	}
	if strings.Contains(text, "so graph search") {
		t.Fatalf("search nudge must not list so graph search (spray menu), got %q", text)
	}
}

func TestCodexPreToolUseStaysSilent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	writeHookSession(t, root, "codex-session")
	payload, err := json.Marshal(map[string]any{
		"tool_name":  "Grep",
		"tool_input": map[string]any{"pattern": "HandleRequest"},
		"cwd":        root,
		"session_id": "codex-session",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok := steerTextFor("codex", "PreToolUse", payload); ok {
		t.Fatal("Codex PreToolUse must not emit additionalContext")
	}
}

func TestStrictDenyFirstReadOnce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SUPEROPEN_HOOK_STRICT", "1")
	root := t.TempDir()
	writeHookSession(t, root, "strict-session")
	payload, err := json.Marshal(map[string]any{
		"tool_name":  "Read",
		"tool_input": map[string]any{"file_path": filepath.Join(root, "internal", "api", "handler.go")},
		"cwd":        root,
		"session_id": "strict-session",
	})
	if err != nil {
		t.Fatal(err)
	}
	first, ok := steerDecisionFor("claude-code", "PreToolUse", "read", payload)
	if !ok || !first.deny || !strings.Contains(first.text, "strict mode") {
		t.Fatalf("first source Read should deny, got deny=%v text=%q", first.deny, first.text)
	}
	second, ok := steerDecisionFor("claude-code", "PreToolUse", "read", payload)
	if !ok || second.deny {
		t.Fatalf("second Read must nudge, not deny; ok=%v deny=%v text=%q", ok, second.deny, second.text)
	}
	if !strings.Contains(second.text, "MANDATORY") {
		t.Fatalf("second Read should still carry MANDATORY, got %q", second.text)
	}
}

func TestStrictSkipDenyWhenQueryStampFresh(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SUPEROPEN_HOOK_STRICT", "1")
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".so", "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".so", "db", "so.db"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeHookSession(t, root, "stamped-session")
	engine.RecordQueryStamp(root)
	payload, err := json.Marshal(map[string]any{
		"tool_name":  "Read",
		"tool_input": map[string]any{"file_path": filepath.Join(root, "internal", "api", "handler.go")},
		"cwd":        root,
		"session_id": "stamped-session",
	})
	if err != nil {
		t.Fatal(err)
	}
	d, ok := steerDecisionFor("claude-code", "PreToolUse", "read", payload)
	if !ok {
		t.Fatal("expected a read nudge")
	}
	if d.deny {
		t.Fatalf("fresh query stamp must skip deny, got %q", d.text)
	}
}

func TestEvalDoesNotEnableHookStrict(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SUPEROPEN_HOOK_STRICT", "")
	root := t.TempDir()
	writeHookSession(t, root, "nonstrict")
	payload, err := json.Marshal(map[string]any{
		"tool_name":  "Read",
		"tool_input": map[string]any{"file_path": filepath.Join(root, "internal", "api", "handler.go")},
		"cwd":        root,
		"session_id": "nonstrict",
	})
	if err != nil {
		t.Fatal(err)
	}
	d, ok := steerDecisionFor("claude-code", "PreToolUse", "read", payload)
	if !ok {
		t.Fatal("expected a read nudge")
	}
	if d.deny {
		t.Fatalf("default hooks must fail-open (nudge only), got deny text=%q", d.text)
	}
}

func TestUserPromptSubmitHasNoAdditionalContext(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	writeHookSession(t, root, "ups-session")
	payload, err := json.Marshal(map[string]any{
		"session_id": "ups-session",
		"cwd":        root,
		"prompt":     "how does dashboard provisioning work",
	})
	if err != nil {
		t.Fatal(err)
	}
	if text, _, ok := steerTextFor("claude-code", "UserPromptSubmit", payload); ok {
		t.Fatalf("UserPromptSubmit must not inject memory pack or steer text, got %q", text)
	}
	if text, _, ok := steerTextFor("codex", "UserPromptSubmit", payload); ok {
		t.Fatalf("codex UserPromptSubmit must stay silent, got %q", text)
	}
}

func TestLifecycleHooksHaveNoSteer(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	writeHookSession(t, root, "life-session")
	payload, err := json.Marshal(map[string]any{
		"session_id": "life-session",
		"cwd":        root,
		"tool_name":  "Bash",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range []string{"PostToolUse", "Stop", "SessionEnd"} {
		if text, _, ok := steerTextFor("claude-code", ev, payload); ok {
			t.Fatalf("%s must not inject additionalContext, got %q", ev, text)
		}
	}
}

func TestOpenCodePiToolBeforeEmitsGraphNudge(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	writeHookSession(t, root, "oc-pi-session")
	payload, err := json.Marshal(map[string]any{
		"session_id": "oc-pi-session",
		"cwd":        root,
		"tool_name":  "bash",
		"command":    "grep Foo pkg",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, vendor := range []string{"opencode", "pi"} {
		text, _, ok := steerTextFor(vendor, "tool.execute.before", payload)
		if !ok || !strings.Contains(text, "MANDATORY") || !strings.Contains(text, "so graph query") {
			t.Fatalf("%s tool.execute.before should nudge graph query, ok=%v text=%q", vendor, ok, text)
		}
	}
	piStart, _, ok := steerTextFor("pi", "tool_execution_start", payload)
	if !ok || !strings.Contains(piStart, "so graph query") {
		t.Fatalf("pi tool_execution_start should nudge, ok=%v text=%q", ok, piStart)
	}
}

func TestOpenCodePiLifecycleSilent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	writeHookSession(t, root, "oc-life")
	payload, err := json.Marshal(map[string]any{
		"session_id": "oc-life",
		"cwd":        root,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range []string{"session.created", "session.end"} {
		if text, _, ok := steerTextFor("opencode", ev, payload); ok {
			t.Fatalf("opencode %s must stay silent, got %q", ev, text)
		}
	}
	if text, _, ok := steerTextFor("pi", "session_start", payload); ok {
		t.Fatalf("pi session_start must stay silent, got %q", text)
	}
}

func TestOpenCodePiGraphToolsNotNudged(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	writeHookSession(t, root, "graph-tool")
	payload, err := json.Marshal(map[string]any{
		"session_id": "graph-tool",
		"cwd":        root,
		"tool_name":  "graph_query",
	})
	if err != nil {
		t.Fatal(err)
	}
	if text, _, ok := steerTextFor("pi", "tool.execute.before", payload); ok {
		t.Fatalf("graph_* tools must not get grep nudge, got %q", text)
	}
}

func writeHookSession(t *testing.T, root, id string) {
	t.Helper()
	// Best-effort ingest so pack has content; empty store still fail-opens.
	_ = os.MkdirAll(filepath.Join(root, ".so", "sessions", id), 0o755)
}
