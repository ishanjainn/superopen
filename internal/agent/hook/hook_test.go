package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/session"
)

func TestPeekContextUsesWorkspaceRootsOnlyWhenCwdMissing(t *testing.T) {
	got := peekContext([]byte(`{"cwd":"/tmp/tool-dir","workspace_roots":["/tmp/workspace"]}`))
	if got.CWD != "/tmp/tool-dir" {
		t.Fatalf("CWD = %q, want payload cwd (main-branch extraction)", got.CWD)
	}
	got = peekContext([]byte(`{"workspace_roots":["/tmp/workspace"]}`))
	if got.CWD != "/tmp/workspace" {
		t.Fatalf("CWD = %q, want workspace_roots fallback", got.CWD)
	}
}

// TestForeignParentID ensures Cursor's self-parent echo
// (parent_conversation_id == conversation.id) is not stamped as a
// real parent link. That echo previously flipped is_subagent on the
// parent chat and hid it from the Sessions list.
func TestForeignParentID(t *testing.T) {
	cases := []struct {
		name           string
		parent         string
		conversationID string
		sessionID      string
		want           string
	}{
		{name: "empty", parent: "", conversationID: "c1", sessionID: "s1", want: ""},
		{name: "foreign", parent: "parent-1", conversationID: "c1", sessionID: "s1", want: "parent-1"},
		{name: "self_eq_conversation", parent: "c1", conversationID: "c1", sessionID: "s1", want: ""},
		{name: "self_eq_session", parent: "s1", conversationID: "c1", sessionID: "s1", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := foreignParentID(tc.parent, tc.conversationID, tc.sessionID); got != tc.want {
				t.Fatalf("foreignParentID(%q,%q,%q) = %q, want %q",
					tc.parent, tc.conversationID, tc.sessionID, got, tc.want)
			}
		})
	}
}

// TestIsClaudeCodeVendor covers the small whitelist that drives the
// host-mismatch guard's left-hand side. The list is small but the
// downstream behaviour (emit vs. drop) is binary, so we want a
// regression test for the spellings actually shipped on plugin
// manifests today.
func TestIsClaudeCodeVendor(t *testing.T) {
	cases := []struct {
		vendor string
		want   bool
	}{
		{"cc", true},
		{"CC", true},
		{"  cc  ", true},
		{"claude-code", true},
		{"Claude-Code", true},
		{"claudecode", true},
		{"cursor", false},
		{"codex", false},
		{"", false},
		{"cc-cursor", false},
	}
	for _, tc := range cases {
		t.Run(tc.vendor, func(t *testing.T) {
			if got := isClaudeCodeVendor(tc.vendor); got != tc.want {
				t.Fatalf("isClaudeCodeVendor(%q) = %v, want %v", tc.vendor, got, tc.want)
			}
		})
	}
}

// TestIsRealClaudeCodeInvocation pins down the rule that drives the
// host-mismatch guard's right-hand side: only `CLAUDECODE=1` is
// authoritative. Cursor 3.4+ honours the Claude Code plugin spec and
// fires our --vendor=cc hook for Cursor's own agent turns, mirroring
// Anthropic's `CLAUDE_*` env envelope but NOT setting `CLAUDECODE=1`
// - the env captured here matches a real production masquerade.
//
// Reverting the rule (e.g. accepting `CLAUDE_PROJECT_DIR` as a
// positive marker) reintroduces duplicate sessions on the Coding
// Agents UI; this test is the trip-wire.
func TestIsRealClaudeCodeInvocation(t *testing.T) {
	// Variables we touch across cases. We snapshot+restore them per
	// test so the suite remains hermetic when run with -run or in
	// parallel-discovery mode.
	managed := []string{
		"CLAUDECODE",
		"CLAUDE_PROJECT_DIR",
		"CLAUDE_PLUGIN_ROOT",
		"CLAUDE_SESSION_ID",
		"CURSOR_VERSION",
		"CURSOR_PLUGIN_ROOT",
		"CURSOR_USER_EMAIL",
		"CURSOR_LAYOUT",
		"CURSOR_EXTENSION_HOST_ROLE",
		"VSCODE_IPC_HOOK",
	}

	cases := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{
			name: "real_claude_code_minimal",
			env: map[string]string{
				"CLAUDECODE": "1",
			},
			want: true,
		},
		{
			name: "real_claude_code_full_envelope",
			env: map[string]string{
				"CLAUDECODE":         "1",
				"CLAUDE_PROJECT_DIR": "/Users/me/repo",
				"CLAUDE_PLUGIN_ROOT": "/Users/me/.claude/plugins/cache/superopen/superopen-cc/0.1.0",
				"CLAUDE_SESSION_ID":  "8f3a...",
			},
			want: true,
		},
		{
			name: "real_claude_code_inside_cursor_terminal",
			env: map[string]string{
				// User opens a Cursor terminal and runs `claude`.
				// The shell inherits CURSOR_* from the IDE but
				// claude itself still sets CLAUDECODE=1. The
				// guard must let this through.
				"CLAUDECODE":      "1",
				"CURSOR_VERSION":  "3.4.17",
				"VSCODE_IPC_HOOK": "/Users/me/Library/Application Support/Cursor/3.4.-main.sock",
			},
			want: true,
		},
		{
			name: "cursor_compat_shim_masquerade",
			env: map[string]string{
				// Captured verbatim from Cursor 3.4.17 invoking
				// our cc hook through its compat shim. Note the
				// absence of CLAUDECODE=1 - that's the single
				// signal we anchor on.
				"CLAUDE_PROJECT_DIR":         "/Users/me/repo",
				"CLAUDE_PLUGIN_ROOT":         "/Users/me/.claude/plugins/cache/superopen/superopen-cc/0.1.0",
				"CURSOR_VERSION":             "3.4.17",
				"CURSOR_PLUGIN_ROOT":         "/Users/me/.claude/plugins/cache/superopen/superopen-cc/0.1.0",
				"CURSOR_USER_EMAIL":          "user@example.com",
				"CURSOR_LAYOUT":              "unifiedAgent",
				"CURSOR_EXTENSION_HOST_ROLE": "always-local",
				"VSCODE_IPC_HOOK":            "/Users/me/Library/Application Support/Cursor/3.4.-main.sock",
			},
			want: false,
		},
		{
			name: "claude_project_dir_alone_is_not_enough",
			env: map[string]string{
				// Pre-fix the guard treated this as positive,
				// which is what let the masquerade through.
				"CLAUDE_PROJECT_DIR": "/Users/me/repo",
			},
			want: false,
		},
		{
			name: "empty_env",
			env:  map[string]string{},
			want: false,
		},
		{
			name: "claudecode_zero_is_not_one",
			env:  map[string]string{"CLAUDECODE": "0"},
			want: false,
		},
		{
			name: "claudecode_whitespace_only_is_not_one",
			env:  map[string]string{"CLAUDECODE": "  "},
			want: false,
		},
		{
			name: "claudecode_with_padding_is_accepted",
			// TrimSpace exists so users sourcing env from a file
			// with stray whitespace don't fail-closed silently.
			env:  map[string]string{"CLAUDECODE": " 1 "},
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, k := range managed {
				// t.Setenv handles restoration on test exit.
				// We Setenv first so the cleanup hook is
				// registered, then Unsetenv to give the guard
				// strict-absence semantics during the test.
				t.Setenv(k, "")
				if err := os.Unsetenv(k); err != nil {
					t.Fatalf("unset %s: %v", k, err)
				}
			}
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if got := isRealClaudeCodeInvocation(); got != tc.want {
				t.Fatalf("isRealClaudeCodeInvocation() = %v, want %v (env=%v)", got, tc.want, tc.env)
			}
		})
	}
}

func TestShouldFinalizeOnlySessionEnd(t *testing.T) {
	if !shouldFinalize("sessionEnd") || !shouldFinalize("SessionEnd") {
		t.Fatal("sessionEnd/SessionEnd must finalize")
	}
	if !shouldFinalize("session.end") || !shouldFinalize("session.deleted") {
		t.Fatal("OpenCode session close must finalize")
	}
	if !shouldFinalize("session_shutdown") || !shouldFinalize("agent_end") {
		t.Fatal("Pi session close must finalize")
	}
	for _, ev := range []string{"stop", "Stop", "sessionStart", "SessionStart", "beforeSubmitPrompt", "UserPromptSubmit", "subagentStop", "SubagentStop"} {
		if shouldFinalize(ev) {
			t.Fatalf("%s must not finalize parent", ev)
		}
	}
	if !shouldFinalizeNested("subagentStop") || !shouldFinalizeNested("SubagentStop") {
		t.Fatal("subagentStop must finalize nested")
	}
	if shouldFinalizeNested("sessionEnd") || shouldFinalizeNested("Stop") {
		t.Fatal("sessionEnd/Stop must not use nested finalize")
	}
}

func TestShouldIngestPromptNotStop(t *testing.T) {
	for _, ev := range []string{"beforeSubmitPrompt", "UserPromptSubmit", "userPromptSubmitted"} {
		if !shouldIngestPrompt(ev) {
			t.Fatalf("%s must ingest this session", ev)
		}
	}
	if shouldIngestPrompt("stop") || shouldIngestPrompt("Stop") || shouldIngestPrompt("sessionEnd") {
		t.Fatal("stop/sessionEnd must not per-prompt ingest")
	}
	if !shouldIngestBackfill("sessionStart") || !shouldIngestBackfill("SessionStart") {
		t.Fatal("SessionStart must backfill")
	}
	if shouldIngestBackfill("stop") {
		t.Fatal("stop must not backfill")
	}
}

func TestNestedChildSessionIDSkipsParent(t *testing.T) {
	parent := "aaaaaaaa-1111-2222-3333-444444444444"
	child := "bbbbbbbb-1111-2222-3333-444444444444"
	payload, _ := json.Marshal(map[string]any{
		"conversation_id": parent,
		"subagent_id":     child,
	})
	if got := nestedChildSessionID(payload); got != child {
		t.Fatalf("nestedChildSessionID=%q want %q", got, child)
	}
	self, _ := json.Marshal(map[string]any{
		"conversation_id": parent,
		"subagent_id":     parent,
	})
	if got := nestedChildSessionID(self); got != "" {
		t.Fatalf("self subagent_id must skip, got %q", got)
	}
}

func TestMaybeFinalizeNestedSpawnsChildNotParent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".so"), 0o755); err != nil {
		t.Fatal(err)
	}
	parent := "aaaaaaaa-1111-2222-3333-444444444444"
	child := "bbbbbbbb-1111-2222-3333-444444444444"
	payload, _ := json.Marshal(map[string]any{
		"cwd":             root,
		"conversation_id": parent,
		"subagent_id":     child,
	})
	var got []string
	prev := spawnSessionFinalize
	spawnSessionFinalize = func(_, id string) { got = append(got, id) }
	t.Cleanup(func() { spawnSessionFinalize = prev })

	maybeFinalizeNested("subagentStop", payload)
	if len(got) != 1 || got[0] != child {
		t.Fatalf("finalize ids=%v want [%s]", got, child)
	}
	maybeFinalizeNested("Stop", payload)
	if len(got) != 1 {
		t.Fatalf("Stop must not nested-finalize, got %v", got)
	}
}

func TestMaybeFinalizeSessionCascadesActiveChildren(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	layout := paths.Resolve(root)
	if err := layout.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	store := session.NewStore(layout)
	parent := "parent-chat"
	child := "child-chat"
	now := time.Now().UTC()
	if err := store.Start(session.Meta{ID: parent, Vendor: "cursor", Status: session.StatusActive, StartedAt: now, PromptPreview: "parent"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Start(session.Meta{
		ID: child, Vendor: "cursor", Status: session.StatusActive, StartedAt: now,
		ParentID: parent, IsSubagent: true, PromptPreview: "child work",
	}); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{
		"cwd":             root,
		"conversation_id": parent,
	})
	var got []string
	prev := spawnSessionFinalize
	spawnSessionFinalize = func(_, id string) { got = append(got, id) }
	t.Cleanup(func() { spawnSessionFinalize = prev })

	maybeFinalizeSession("sessionEnd", payload)
	want := map[string]bool{parent: false, child: false}
	for _, id := range got {
		want[id] = true
	}
	if !want[parent] || !want[child] {
		t.Fatalf("finalize ids=%v want parent+child", got)
	}
}
