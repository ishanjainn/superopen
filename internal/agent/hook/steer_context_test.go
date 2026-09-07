package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/engine"
	"github.com/ishanjainn/superopen/internal/harvest"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/paths"
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

func TestBashLooksLikeReadAndListing(t *testing.T) {
	if !bashLooksLikeRead("sed -n '1,80p' django/db/models/query.py") {
		t.Fatal("sed -n should count as a file read")
	}
	if !bashLooksLikeRead("python3 -c \"print(open('django/db/models/query.py').read())\"") {
		t.Fatal("python3 -c should count as a file read")
	}
	if !bashLooksLikeListing("ls") || !bashLooksLikeListing("ls -la /work") {
		t.Fatal("ls should count as a listing")
	}
	if bashLooksLikeRead("python3 manage.py test") || bashLooksLikeListing("git status") {
		t.Fatal("unrelated commands must not match read/listing")
	}
	if !bashLooksLikeGraphQuery("/usr/local/bin/so graph query 'How does middleware work?'") {
		t.Fatal("graph query should match")
	}
	if bashLooksLikeGraphQuery("/usr/local/bin/so graph snippet pkg.Foo.bar") {
		t.Fatal("snippet must not look like graph query")
	}
}

func TestBashCatPreToolUseEmitsGraphNudge(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	writeHookSession(t, root, "bash-cat")
	writeHookSourceFile(t, root)
	payload, err := json.Marshal(map[string]any{
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": "sed -n '1,80p' pkg/foo.go"},
		"cwd":        root,
		"session_id": "bash-cat",
	})
	if err != nil {
		t.Fatal(err)
	}
	d, ok := steerDecisionFor("claude-code", "PreToolUse", "search", payload)
	if !ok || !strings.Contains(d.text, "graph query") {
		t.Fatalf("bash sed -n should get a graph nudge, ok=%v text=%q", ok, d.text)
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
		t.Fatalf("empty store SessionStart must stay silent, got %q", text)
	}
}

func TestSessionStartHarvestOneLiner(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	writeHookSession(t, root, "harvest-session")
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# Agents\n\nFollow graph-first search.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diff := `--- a/AGENTS.md
+++ b/AGENTS.md
@@ -1,3 +1,4 @@
 # Agents
 
 Follow graph-first search.
+Prefer so harvest review for playbook patches.
`
	if _, err := harvest.Propose(root, harvest.ProposeInput{
		Title: "add harvest line", Target: "AGENTS.md", Reason: "missing pointer",
		Kind: harvest.KindImprove, Diff: diff,
	}); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"session_id": "harvest-session",
		"cwd":        root,
	})
	if err != nil {
		t.Fatal(err)
	}
	text, _, ok := steerTextFor("claude-code", "SessionStart", payload)
	if ok {
		t.Fatalf("SessionStart must not inject harvest: %q", text)
	}
	if extra, _, ok := steerTextFor("claude-code", "Stop", payload); ok {
		t.Fatalf("Stop must stay silent, got %q", extra)
	}
}

func TestSessionStartPendingHarvestOneLiner(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	writeHookSession(t, root, "pending-session")
	writeHookSourceFile(t, root)
	store, err := harvest.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.InsertRun("old-cursor-sess", harvest.StatusPending, "", "await-live"); err != nil {
		t.Fatal(err)
	}
	store.Close()
	payload, err := json.Marshal(map[string]any{
		"session_id": "pending-session",
		"cwd":        root,
	})
	if err != nil {
		t.Fatal(err)
	}
	text, _, ok := steerTextFor("cursor", "sessionStart", payload)
	if !ok || !strings.Contains(text, "HARVEST pending") || !paths.MentionsCommand(text, "harvest propose") {
		t.Fatalf("pending harvest one-liner, got ok=%v %q", ok, text)
	}
	if strings.Contains(text, "review") {
		t.Fatalf("must not inject OPEN review: %q", text)
	}
}

func TestPromptSubmitPendingHarvestOnCodePrompt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	writeHookSession(t, root, "code-harvest")
	writeHookSourceFile(t, root)
	store, err := harvest.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.InsertRun("old-cursor-sess", harvest.StatusPending, "", harvest.SkipAwaitLive); err != nil {
		t.Fatal(err)
	}
	store.Close()
	payload, err := json.Marshal(map[string]any{
		"session_id": "code-harvest",
		"cwd":        root,
		"prompt":     "Where is the plugin's App plugin registered?",
	})
	if err != nil {
		t.Fatal(err)
	}
	text, _, ok := steerTextFor("cursor", "beforeSubmitPrompt", payload)
	if !ok || !strings.Contains(text, "HARVEST pending") || !paths.MentionsCommand(text, "harvest propose") {
		t.Fatalf("code prompt must still inject live harvest, got ok=%v %q", ok, text)
	}
	if strings.Contains(text, "review") {
		t.Fatalf("must not inject OPEN review: %q", text)
	}
	second, _, ok := steerTextFor("cursor", "beforeSubmitPrompt", payload)
	if ok && strings.Contains(second, "HARVEST pending") {
		t.Fatalf("second prompt-submit must not re-nag harvest: %q", second)
	}
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
		t.Fatalf("empty store sessionStart must stay silent, got %q", first)
	}
}

func TestSessionStartIndexGraphFirstNoBodies(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	id := "mem-index-session"
	writeHookSession(t, root, id)
	body := "UNIQUE_BODY_PHRASE_DO_NOT_INJECT xyzzy-memory-body"
	store, err := memory.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Capture(memory.CaptureInput{
		Kind:  memory.KindSession,
		Title: "JWT expiry is 15m",
		Text:  body,
		Topic: memory.ObservationDecision,
	}); err != nil {
		t.Fatal(err)
	}
	store.Close()

	payload, err := json.Marshal(map[string]any{"session_id": id, "cwd": root})
	if err != nil {
		t.Fatal(err)
	}
	vendors := []struct {
		vendor, event string
	}{
		{"claude-code", "SessionStart"},
		{"cursor", "sessionStart"},
		{"codex", "SessionStart"},
		{"gemini", "sessionStart"},
		{"copilot-cli", "sessionStart"},
		{"opencode", "session.created"},
		{"pi", "session_start"},
	}
	for _, tc := range vendors {
		text, _, ok := steerTextFor(tc.vendor, tc.event, payload)
		if !ok {
			t.Fatalf("%s %s: expected SessionStart index", tc.vendor, tc.event)
		}
		if !strings.Contains(text, "memories in this workspace") {
			t.Fatalf("%s %s: must say memories exist: %q", tc.vendor, tc.event, text)
		}
		if !strings.Contains(text, "memory recall") {
			t.Fatalf("%s %s: must give recall command: %q", tc.vendor, tc.event, text)
		}
		if !strings.Contains(text, "shell") {
			t.Fatalf("%s %s: must say run in your shell: %q", tc.vendor, tc.event, text)
		}
		if !strings.Contains(text, ".so/") && !strings.Contains(text, "MEMORY.md") {
			t.Fatalf("%s %s: must name the workspace store: %q", tc.vendor, tc.event, text)
		}
		if strings.Contains(text, body) {
			t.Fatalf("%s %s: must not inject episode bodies: %q", tc.vendor, tc.event, text)
		}
		if memory.EstimateTokens(text) > 350 {
			t.Fatalf("%s %s: index %d tokens over 350: %q", tc.vendor, tc.event, memory.EstimateTokens(text), text)
		}
	}
	if _, _, ok := steerTextFor("codex", "PreToolUse", []byte(`{"tool_name":"Grep","tool_input":{"pattern":"HandleRequest"},"cwd":"`+root+`","session_id":"`+id+`"}`)); ok {
		t.Fatal("Codex PreToolUse must stay silent")
	}
}

func TestCodeRepoSessionStartIsGraphFirst(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	writeHookSession(t, root, "code-session")
	writeHookSourceFile(t, root)
	payload, err := json.Marshal(map[string]any{"session_id": "code-session", "cwd": root})
	if err != nil {
		t.Fatal(err)
	}
	text, _, ok := steerTextFor("claude-code", "SessionStart", payload)
	if !ok || !strings.Contains(text, "graph query") {
		t.Fatalf("code repo SessionStart must be graph-first, got ok=%v text=%q", ok, text)
	}
	if strings.Contains(text, "memories in this workspace") {
		t.Fatalf("code repo SessionStart must not preach memory: %q", text)
	}
}

func TestSubagentStartInjectsHookReminder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	writeHookSession(t, root, "steer-subagent-session")
	writeHookSourceFile(t, root)
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
	if !paths.MentionsCommand(text, "graph query") {
		t.Fatalf("SubagentStart text missing graph reminder: %q", text)
	}
	cursorText, cursorEv, cursorOK := steerTextFor("cursor", "subagentStart", payload)
	if !cursorOK || cursorEv != "subagentStart" || !paths.MentionsCommand(cursorText, "graph query") {
		t.Fatalf("cursor subagentStart must inject the same reminder, got ok=%v ev=%q text=%q", cursorOK, cursorEv, cursorText)
	}
	if text, _, ok := steerTextFor("claude-code", "SubagentStart", payload); ok {
		t.Fatalf("second SubagentStart in the same session must stay silent, got %q", text)
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
	if ok && strings.Contains(text, "so memory search") {
		t.Fatalf("preCompact must not CTA search: %q", text)
	}
}

func TestGrepPreToolUseEmitsGraphNudge(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	writeHookSession(t, root, "grep-session")
	writeHookSourceFile(t, root)
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
	if !ok || !paths.MentionsCommand(text, "graph query") {
		t.Fatalf("grep should get a graph-first nudge, got %q", text)
	}
	if strings.Contains(text, "MANDATORY") {
		t.Fatalf("grep nudge must not say MANDATORY, got %q", text)
	}
	if strings.Contains(text, "Superopen graph:") || strings.Contains(text, "hit(s) for") {
		t.Fatalf("grep must not receive ExploreAugment hit lists, got %q", text)
	}
	if strings.Contains(text, "so graph search") {
		t.Fatalf("search nudge must not list so graph search (spray menu), got %q", text)
	}
}

func TestCursorPreToolUseGrepInfersSearch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	writeHookSession(t, root, "grep-session")
	writeHookSourceFile(t, root)
	payload, err := json.Marshal(map[string]any{
		"tool_name":  "Grep",
		"tool_input": map[string]any{"pattern": "HandleRequest"},
		"cwd":        root,
		"session_id": "grep-session",
	})
	if err != nil {
		t.Fatal(err)
	}
	text, _, ok := steerTextFor("cursor", "preToolUse", payload)
	if !ok || !paths.MentionsCommand(text, "graph query") {
		t.Fatalf("cursor Grep without --kind should get search nudge, got %q", text)
	}
	if strings.Contains(text, "MANDATORY") {
		t.Fatalf("cursor nudge must not say MANDATORY, got %q", text)
	}
	if !strings.Contains(text, ".so/") {
		t.Fatalf("nudge should mention .so/: %q", text)
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
	writeHookSourceFile(t, root)
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
	if !paths.MentionsCommand(second.text, "graph query") {
		t.Fatalf("second Read should still carry the graph nudge, got %q", second.text)
	}
	if strings.Contains(second.text, "MANDATORY") {
		t.Fatalf("Read nudge must not say MANDATORY, got %q", second.text)
	}
}

func TestQueryStampFreshReadOverflowsToSnippetOnce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".so", "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".so", "db", "so.db"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeHookSession(t, root, "overflow-session")
	writeHookSourceFile(t, root)
	py := filepath.Join(root, "django", "db", "models", "query.py")
	if err := os.MkdirAll(filepath.Dir(py), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(py, []byte("class QuerySet:\n    pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	engine.RecordQueryStampFor(root, "overflow-session")
	readPayload, err := json.Marshal(map[string]any{
		"tool_name":  "Read",
		"tool_input": map[string]any{"file_path": py},
		"cwd":        root,
		"session_id": "overflow-session",
	})
	if err != nil {
		t.Fatal(err)
	}
	d, ok := steerDecisionFor("claude-code", "PreToolUse", "read", readPayload)
	if !ok || d.deny || !paths.MentionsCommand(d.text, "graph snippet") {
		t.Fatalf("after query, source Read must overflow to snippet once, got ok=%v deny=%v text=%q", ok, d.deny, d.text)
	}
	if _, ok := steerDecisionFor("claude-code", "PreToolUse", "read", readPayload); ok {
		t.Fatal("snippet overflow must fire once per session")
	}
	grepPayload, err := json.Marshal(map[string]any{
		"tool_name":  "Grep",
		"tool_input": map[string]any{"pattern": "QuerySet"},
		"cwd":        root,
		"session_id": "overflow-session",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := steerDecisionFor("claude-code", "PreToolUse", "search", grepPayload); ok {
		t.Fatal("fresh query stamp must skip Grep/search nudges")
	}
}

func TestQueryRepeatOverflowOnceAfterStamp(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".so", "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".so", "db", "so.db"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeHookSession(t, root, "repeat-session")
	writeHookSourceFile(t, root)
	queryPayload, err := json.Marshal(map[string]any{
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": "/usr/local/bin/so graph query 'How does middleware process a request?'"},
		"cwd":        root,
		"session_id": "repeat-session",
	})
	if err != nil {
		t.Fatal(err)
	}
	if d, ok := steerDecisionFor("claude-code", "PreToolUse", "search", queryPayload); ok {
		t.Fatalf("first graph query must run without overflow, got %q", d.text)
	}
	d, ok := steerDecisionFor("claude-code", "PreToolUse", "search", queryPayload)
	if !ok || d.deny || !paths.MentionsCommand(d.text, "graph snippet") || !strings.Contains(d.text, "TRUNCATED") {
		t.Fatalf("second graph query must overflow to snippet (no deny), got ok=%v deny=%v text=%q", ok, d.deny, d.text)
	}
	if _, ok := steerDecisionFor("claude-code", "PreToolUse", "search", queryPayload); ok {
		t.Fatal("query-repeat overflow must fire once per session")
	}
	snippetPayload, err := json.Marshal(map[string]any{
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": "/usr/local/bin/so graph snippet pkg.Site.register"},
		"cwd":        root,
		"session_id": "repeat-session",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := steerDecisionFor("claude-code", "PreToolUse", "search", snippetPayload); ok {
		t.Fatal("graph snippet after a query must stay silent")
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
	writeHookSourceFile(t, root)
	engine.RecordQueryStampFor(root, "stamped-session")
	readPayload, err := json.Marshal(map[string]any{
		"tool_name":  "Read",
		"tool_input": map[string]any{"file_path": filepath.Join(root, "internal", "api", "handler.go")},
		"cwd":        root,
		"session_id": "stamped-session",
	})
	if err != nil {
		t.Fatal(err)
	}
	d, ok := steerDecisionFor("claude-code", "PreToolUse", "read", readPayload)
	if !ok || d.deny || !paths.MentionsCommand(d.text, "graph snippet") {
		t.Fatalf("fresh stamp Read must overflow to snippet (no deny), got ok=%v deny=%v text=%q", ok, d.deny, d.text)
	}
	if _, ok := steerDecisionFor("claude-code", "PreToolUse", "read", readPayload); ok {
		t.Fatal("snippet overflow must fire once per session")
	}
	grepPayload, err := json.Marshal(map[string]any{
		"tool_name":  "Grep",
		"tool_input": map[string]any{"pattern": "QuerySet"},
		"cwd":        root,
		"session_id": "stamped-session",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := steerDecisionFor("claude-code", "PreToolUse", "search", grepPayload); ok {
		t.Fatal("fresh query stamp must skip Grep/search nudges")
	}
}

func TestEvalDoesNotEnableHookStrict(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SUPEROPEN_HOOK_STRICT", "")
	root := t.TempDir()
	writeHookSession(t, root, "nonstrict")
	writeHookSourceFile(t, root)
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

func TestPriorWorkCueInjectsIndex(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	id := "cue-session"
	writeHookSession(t, root, id)
	store, err := memory.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Capture(memory.CaptureInput{
		Kind:  memory.KindSession,
		Title: "JWT expiry is 15m",
		Text:  "UNIQUE_CUE_BODY",
		Topic: memory.ObservationDecision,
	}); err != nil {
		t.Fatal(err)
	}
	store.Close()
	payload, err := json.Marshal(map[string]any{
		"session_id": id,
		"cwd":        root,
		"prompt":     "what did we decide last time about JWT?",
	})
	if err != nil {
		t.Fatal(err)
	}
	text, _, ok := steerTextFor("claude-code", "UserPromptSubmit", payload)
	if !ok || !strings.Contains(text, "JWT expiry") {
		t.Fatalf("cue should inject index lines, got ok=%v text=%q", ok, text)
	}
	if !strings.Contains(text, "import ids") || !strings.Contains(text, "cite both") || !strings.Contains(text, "second cue") {
		t.Fatalf("cue pack must include diary framing, got %q", text)
	}
	if !strings.Contains(text, "UNIQUE_CUE_BODY") {
		t.Fatalf("cue should inject matching recalled body, got %q", text)
	}
	if !strings.Contains(text, "past sessions") {
		t.Fatalf("cue pack must frame ownership, got %q", text)
	}
	if !strings.Contains(text, "--full") {
		t.Fatalf("cue pack must point at memory get --full, got %q", text)
	}
	cursorText, _, cursorOK := steerTextFor("cursor", "beforeSubmitPrompt", payload)
	if !cursorOK || !strings.Contains(cursorText, "UNIQUE_CUE_BODY") {
		t.Fatalf("cursor beforeSubmitPrompt cue failed: ok=%v text=%q", cursorOK, cursorText)
	}
	copilotText, _, copilotOK := steerTextFor("copilot-cli", "userPromptSubmitted", payload)
	if !copilotOK || !strings.Contains(copilotText, "UNIQUE_CUE_BODY") {
		t.Fatalf("copilot-cli userPromptSubmitted cue failed: ok=%v text=%q", copilotOK, copilotText)
	}
	codexText, _, codexOK := steerTextFor("codex", "UserPromptSubmit", payload)
	if !codexOK || !strings.Contains(codexText, "UNIQUE_CUE_BODY") {
		t.Fatalf("codex UserPromptSubmit cue failed: ok=%v text=%q", codexOK, codexText)
	}
	geminiText, _, geminiOK := steerTextFor("gemini", "BeforeAgent", payload)
	if !geminiOK || !strings.Contains(geminiText, "UNIQUE_CUE_BODY") {
		t.Fatalf("gemini BeforeAgent cue failed: ok=%v text=%q", geminiOK, geminiText)
	}
	piText, _, piOK := steerTextFor("pi", "before_agent_start", payload)
	if !piOK || !strings.Contains(piText, "UNIQUE_CUE_BODY") {
		t.Fatalf("pi before_agent_start cue failed: ok=%v text=%q", piOK, piText)
	}
	if text, _, ok := steerTextFor("opencode", "session.created", payload); ok && strings.Contains(text, "UNIQUE_CUE_BODY") {
		t.Fatalf("opencode has no prompt-submit event; must not inject bodies on session.created, got %q", text)
	}
}

func TestCodePromptDoesNotInjectMemoryPack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	writeHookSession(t, root, "code-prompt")
	writeHookSourceFile(t, root)
	store, err := memory.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Capture(memory.CaptureInput{
		Kind: memory.KindSession, Title: "JWT expiry is 15m", Text: "UNIQUE_CODE_PROMPT_BODY",
	}); err != nil {
		t.Fatal(err)
	}
	store.Close()
	payload, err := json.Marshal(map[string]any{
		"session_id": "code-prompt",
		"cwd":        root,
		"prompt":     "how does dashboard provisioning work",
	})
	if err != nil {
		t.Fatal(err)
	}
	if text, _, ok := steerTextFor("claude-code", "UserPromptSubmit", payload); ok {
		t.Fatalf("code prompt must not inject memory bodies, got %q", text)
	}
}

func TestSkillReadIsNotGraphGate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	writeHookSession(t, root, "skill-read")
	payload, err := json.Marshal(map[string]any{
		"tool_name":  "Read",
		"tool_input": map[string]any{"file_path": root + "/.claude/skills/so/SKILL.md"},
		"cwd":        root,
		"session_id": "skill-read",
	})
	if err != nil {
		t.Fatal(err)
	}
	if text, _, ok := steerTextFor("claude-code", "PreToolUse", payload); ok {
		t.Fatalf("skill Read must not count as skipped graph, got %q", text)
	}
	if d, ok := steerDecisionFor("cursor", "beforeReadFile", "read", payload); ok && d.text != "" {
		t.Fatalf("cursor skill beforeReadFile must stay silent, got %#v", d)
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
	writeHookSourceFile(t, root)
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
		if !ok || !paths.MentionsCommand(text, "graph query") {
			t.Fatalf("%s tool.execute.before should nudge graph query, ok=%v text=%q", vendor, ok, text)
		}
		if strings.Contains(text, "MANDATORY") || strings.Contains(text, `"`) {
			t.Fatalf("%s nudge must be one line without MANDATORY or double quotes, got %q", vendor, text)
		}
	}
	piPayload, err := json.Marshal(map[string]any{
		"session_id": "oc-pi-session-2",
		"cwd":        root,
		"tool_name":  "bash",
		"command":    "grep Foo pkg",
	})
	if err != nil {
		t.Fatal(err)
	}
	piStart, _, ok := steerTextFor("pi", "tool_execution_start", piPayload)
	if !ok || !paths.MentionsCommand(piStart, "graph query") {
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

func TestMemoryPromptSkipsSearchNudge(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	id := "diary-qa"
	writeHookSession(t, root, id)
	store, err := memory.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Capture(memory.CaptureInput{
		Kind: memory.KindSession, Title: "Caroline raced Tuesday", Text: "Caroline raced on Tuesday",
	}); err != nil {
		t.Fatal(err)
	}
	store.Close()
	submit, err := json.Marshal(map[string]any{
		"session_id": id, "cwd": root, "prompt": "Who is Caroline?",
	})
	if err != nil {
		t.Fatal(err)
	}
	text, _, ok := steerTextFor("claude-code", "UserPromptSubmit", submit)
	if !ok || !strings.Contains(text, "memory recall") {
		t.Fatalf("personal prompt should inject recall, ok=%v text=%q", ok, text)
	}
	if strings.Contains(text, "graph query") {
		t.Fatalf("personal prompt must not mandate graph: %q", text)
	}
	grep, err := json.Marshal(map[string]any{
		"tool_name": "Grep", "tool_input": map[string]any{"pattern": "Caroline"},
		"cwd": root, "session_id": id,
	})
	if err != nil {
		t.Fatal(err)
	}
	nudge, _, ok := steerTextFor("claude-code", "PreToolUse", grep)
	if !ok || !strings.Contains(nudge, "memory recall") {
		t.Fatalf("memory prompt must not get SearchNudge, got ok=%v text=%q", ok, nudge)
	}
	if strings.Contains(nudge, "graph query") && !strings.Contains(nudge, "Skip Grep and graph query") {
		t.Fatalf("memory PreToolUse must not mandate graph: %q", nudge)
	}
	second, _, ok := steerTextFor("claude-code", "UserPromptSubmit", submit)
	if !ok || !strings.Contains(second, "Caroline raced") {
		t.Fatalf("second UserPromptSubmit should re-inject matching bodies, ok=%v text=%q", ok, second)
	}
}

func TestEmptyStorePreToolUseSilent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	writeHookSession(t, root, "empty-store")
	payload, err := json.Marshal(map[string]any{
		"tool_name": "Grep", "tool_input": map[string]any{"pattern": "Handler"},
		"cwd": root, "session_id": "empty-store",
	})
	if err != nil {
		t.Fatal(err)
	}
	if text, _, ok := steerTextFor("claude-code", "PreToolUse", payload); ok {
		t.Fatalf("empty graph+memory must stay silent, got %q", text)
	}
}

func TestSoBashSkipsNudge(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	writeHookSession(t, root, "so-bash")
	writeHookSourceFile(t, root)
	payload, err := json.Marshal(map[string]any{
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": "so graph query \"how does auth work\""},
		"cwd":        root,
		"session_id": "so-bash",
	})
	if err != nil {
		t.Fatal(err)
	}
	if text, _, ok := steerTextFor("claude-code", "PreToolUse", payload); ok {
		t.Fatalf("so Bash must not be nagged, got %q", text)
	}
}

func TestEmptySessionIDUsesRootStampOnce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	writeHookSession(t, root, "no-sid")
	writeHookSourceFile(t, root)
	payload, err := json.Marshal(map[string]any{
		"tool_name":  "Grep",
		"tool_input": map[string]any{"pattern": "HandleRequest"},
		"cwd":        root,
	})
	if err != nil {
		t.Fatal(err)
	}
	text, _, ok := steerTextFor("claude-code", "PreToolUse", payload)
	if !ok || !paths.MentionsCommand(text, "graph query") {
		t.Fatalf("first grep without session id should nudge, ok=%v text=%q", ok, text)
	}
	if text, _, ok := steerTextFor("claude-code", "PreToolUse", payload); ok {
		t.Fatalf("second grep without session id must use the root stamp, got %q", text)
	}
}

func writeHookSourceFile(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCapturePromptNudgesCaptureNotRecall(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	id := "capture-sess"
	writeHookSession(t, root, id)
	submit, err := json.Marshal(map[string]any{
		"session_id": id, "cwd": root, "prompt": "remember this: login timeout is 30s",
	})
	if err != nil {
		t.Fatal(err)
	}
	text, _, ok := steerTextFor("claude-code", "UserPromptSubmit", submit)
	if !ok || !strings.Contains(text, "memory capture") {
		t.Fatalf("remember-this should inject capture, ok=%v text=%q", ok, text)
	}
	if strings.Contains(text, "memory recall") {
		t.Fatalf("remember-this must not inject recall, text=%q", text)
	}
	grep, err := json.Marshal(map[string]any{
		"tool_name": "Grep", "tool_input": map[string]any{"pattern": "timeout"},
		"cwd": root, "session_id": id,
	})
	if err != nil {
		t.Fatal(err)
	}
	nudge, _, ok := steerTextFor("claude-code", "PreToolUse", grep)
	if !ok || !strings.Contains(nudge, "memory capture") {
		t.Fatalf("capture PreToolUse should nudge capture, ok=%v text=%q", ok, nudge)
	}
	if strings.Contains(nudge, "memory recall") {
		t.Fatalf("capture PreToolUse must not inject recall, text=%q", nudge)
	}

	askID := "recall-sess"
	writeHookSession(t, root, askID)
	ask, err := json.Marshal(map[string]any{
		"session_id": askID, "cwd": root, "prompt": "do you remember where we left the login timeout",
	})
	if err != nil {
		t.Fatal(err)
	}
	askText, _, askOK := steerTextFor("claude-code", "UserPromptSubmit", ask)
	if askOK && strings.Contains(askText, "memory capture") {
		t.Fatalf("recall question must not inject capture, text=%q", askText)
	}
}

func writeHookSession(t *testing.T, root, id string) {
	t.Helper()
	_ = os.MkdirAll(filepath.Join(root, ".so", "sessions", id), 0o755)
}
