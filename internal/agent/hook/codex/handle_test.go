package codex

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/agent/normalize"
	"github.com/ishanjainn/superopen/internal/agent/sessionstate"
)

// recordingEmitter is a normalize.Emitter implementation that captures
// every call so tests can assert on the order + content of emissions.
type recordingEmitter struct {
	sessions      []normalize.Session
	toolCalls     []normalize.ToolCall
	editDecisions []normalize.EditDecision
	llmTurns      []normalize.LLMTurn
	subagents     []normalize.Subagent
	events        []normalize.EventEmission
	gitCommits    []normalize.GitCommit
	gitPRs        []normalize.GitPullRequest
}

func (e *recordingEmitter) EmitSession(s normalize.Session) error {
	e.sessions = append(e.sessions, s)
	return nil
}
func (e *recordingEmitter) EmitToolCall(t normalize.ToolCall) error {
	e.toolCalls = append(e.toolCalls, t)
	return nil
}
func (e *recordingEmitter) EmitEditDecision(d normalize.EditDecision) error {
	e.editDecisions = append(e.editDecisions, d)
	return nil
}
func (e *recordingEmitter) EmitLLMTurn(t normalize.LLMTurn) error {
	e.llmTurns = append(e.llmTurns, t)
	return nil
}
func (e *recordingEmitter) EmitSubagent(s normalize.Subagent) error {
	e.subagents = append(e.subagents, s)
	return nil
}
func (e *recordingEmitter) EmitEvent(ev normalize.EventEmission) error {
	e.events = append(e.events, ev)
	return nil
}
func (e *recordingEmitter) EmitGitCommit(c normalize.GitCommit) error {
	e.gitCommits = append(e.gitCommits, c)
	return nil
}
func (e *recordingEmitter) EmitGitPullRequest(p normalize.GitPullRequest) error {
	e.gitPRs = append(e.gitPRs, p)
	return nil
}

// withIsolatedCache redirects sessionstate's on-disk cache to a tmp
// dir so tests don't leak state between runs.
func withIsolatedCache(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	if home := os.Getenv("HOME"); home != "" {
		t.Setenv("HOME", dir)
	}
	t.Setenv("USERPROFILE", dir)
	t.Setenv("LOCALAPPDATA", dir)
}

func TestCodexEndToEndOneTurn(t *testing.T) {
	withIsolatedCache(t)

	em := &recordingEmitter{}
	in := func(event string, payload any) normalize.Input {
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("payload marshal: %v", err)
		}
		return normalize.Input{
			Vendor:         "codex",
			Event:          event,
			Payload:        body,
			ContentCapture: "full",
			Emit:           em,
		}
	}

	sid := "cdx-session-1"
	turn := "turn-1"
	now := time.Now().UTC().Format(time.RFC3339Nano)

	if err := handle(context.Background(), in("SessionStart", map[string]any{
		"hook_event_name": "SessionStart",
		"session_id":      sid,
		"cwd":             "/tmp/work",
		"model":           "gpt-5",
		"source":          "startup",
		"timestamp":       now,
	})); err != nil {
		t.Fatalf("SessionStart: %v", err)
	}
	if err := handle(context.Background(), in("UserPromptSubmit", map[string]any{
		"hook_event_name": "UserPromptSubmit",
		"session_id":      sid,
		"turn_id":         turn,
		"prompt":          "refactor sessionstate",
		"timestamp":       now,
	})); err != nil {
		t.Fatalf("UserPromptSubmit: %v", err)
	}
	if err := handle(context.Background(), in("PostToolUse", map[string]any{
		"hook_event_name":  "PostToolUse",
		"session_id":       sid,
		"turn_id":          turn,
		"tool_name":        "shell",
		"tool_use_id":      "call_1",
		"tool_input":       json.RawMessage(`{"command":"ls -la"}`),
		"tool_response":    json.RawMessage(`{"stdout":"foo bar","exit_code":0}`),
		"tool_duration_ms": 25.0,
		"status":           "completed",
		"timestamp":        now,
	})); err != nil {
		t.Fatalf("PostToolUse: %v", err)
	}
	lastMsg := "Refactored, see diff."
	if err := handle(context.Background(), in("Stop", map[string]any{
		"hook_event_name":        "Stop",
		"session_id":             sid,
		"turn_id":                turn,
		"last_assistant_message": lastMsg,
		"timestamp":              now,
	})); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Sessions: SessionStart → 1, Stop → 1 (Codex has no SessionEnd
	// event, so Stop re-emits an active session-root snapshot). The deterministic span IDs in the otlp
	// emitter dedupe these into a single `otel_traces` row.
	if len(em.sessions) != 2 {
		t.Fatalf("expected 2 sessions (start + stop), got %d", len(em.sessions))
	}
	if em.sessions[0].Vendor != "codex" {
		t.Errorf("session vendor: got %q, want codex", em.sessions[0].Vendor)
	}
	if em.sessions[1].Outcome != "" {
		t.Errorf("stop session outcome: got %q, want active/unfinalized", em.sessions[1].Outcome)
	}
	// Tool calls: PostToolUse → 1
	if len(em.toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(em.toolCalls))
	}
	tc := em.toolCalls[0]
	if tc.ToolName != "shell" {
		t.Errorf("tool name: got %q, want shell", tc.ToolName)
	}
	if tc.Command != "ls -la" {
		t.Errorf("tool command (full mode): got %q, want %q", tc.Command, "ls -la")
	}
	// LLM turns: Stop → 1
	if len(em.llmTurns) != 1 {
		t.Fatalf("expected 1 llm turn, got %d", len(em.llmTurns))
	}
	llt := em.llmTurns[0]
	if llt.Prompt != "refactor sessionstate" {
		t.Errorf("llm.prompt: got %q", llt.Prompt)
	}
	if llt.Response != lastMsg {
		t.Errorf("llm.response: got %q want %q", llt.Response, lastMsg)
	}
	foundPrompt := false
	for _, ev := range em.events {
		if ev.Name != "coding_agent.user_prompt.submit" {
			continue
		}
		if p, _ := ev.Attrs["gen_ai.prompt"].(string); p == "refactor sessionstate" {
			foundPrompt = true
		}
	}
	if !foundPrompt {
		t.Fatal("UserPromptSubmit must stamp gen_ai.prompt for ingest")
	}
	// Tool calls + tool results are NOT folded onto the LLM-turn
	// span - they live on the dedicated `coding_agent.tool.call`
	// span (asserted above via em.toolCalls). Re-encoding them in
	// the turn's messages JSON used to balloon `gen_ai.input.messages`
	// past the 16 KB cap, which broke the chat view's JSON parser.
}

func TestEffectiveToolNameUnwrapsCodeMode(t *testing.T) {
	tests := []struct {
		code string
		want string
	}{
		{`const r = await tools.exec_command({cmd:"go test ./..."})`, "Bash"},
		{`text(await tools.apply_patch(patch))`, "apply_patch"},
		{`await tools.update_plan({plan: []})`, "Update plan"},
		{`await tools.node_repl({code:"..."})`, "Browser"},
	}
	for _, tt := range tests {
		raw, _ := json.Marshal(map[string]string{"code": tt.code})
		if got := effectiveToolName(codexPayload{ToolName: "exec", ToolInput: raw}); got != tt.want {
			t.Errorf("effectiveToolName(%q)=%q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestApplyPatchBodySupportsCodexHookCommand(t *testing.T) {
	patch := "*** Begin Patch\n*** Update File: web/src/app.tsx\n-old\n+new\n*** End Patch"
	raw, _ := json.Marshal(map[string]string{"command": patch})
	if got := applyPatchBody(raw); got != patch {
		t.Fatalf("applyPatchBody(command) = %q, want %q", got, patch)
	}
}

func TestApplyPatchBodyUnwrapsCodeMode(t *testing.T) {
	patch := "*** Begin Patch\n*** Update File: web/src/app.tsx\n-old\n+new\n*** End Patch"
	codeBytes, _ := json.Marshal(patch)
	code := "const patch = " + string(codeBytes) + ";\ntext(await tools.apply_patch(patch));\n"
	raw, _ := json.Marshal(map[string]string{"code": code})
	if got := applyPatchBody(raw); got != patch {
		t.Fatalf("applyPatchBody(code mode) = %q, want %q", got, patch)
	}
}

func TestCodeModeApplyPatchEmitsFileDecisions(t *testing.T) {
	withIsolatedCache(t)
	em := &recordingEmitter{}
	patch := "*** Begin Patch\n*** Update File: web/src/app.tsx\n-old\n+new\n*** Add File: docs/new.md\n+hello\n*** End Patch"
	encoded, _ := json.Marshal(patch)
	code := "const patch = " + string(encoded) + ";\ntext(await tools.apply_patch(patch));\n"
	payload, _ := json.Marshal(map[string]any{
		"hook_event_name": "PostToolUse",
		"session_id":      "session-map-edits",
		"turn_id":         "turn-map-edits",
		"tool_name":       "exec",
		"tool_use_id":     "call-map-edits",
		"tool_input":      map[string]string{"code": code},
		"status":          "completed",
	})
	if err := handle(context.Background(), normalize.Input{
		Vendor: "codex", Event: "PostToolUse", Payload: payload,
		ContentCapture: "full", Emit: em,
	}); err != nil {
		t.Fatal(err)
	}
	if len(em.editDecisions) != 2 {
		t.Fatalf("edit decisions = %d, want 2: %+v", len(em.editDecisions), em.editDecisions)
	}
	got := []string{em.editDecisions[0].FilePath, em.editDecisions[1].FilePath}
	if !strings.Contains(strings.Join(got, ","), "web/src/app.tsx") || !strings.Contains(strings.Join(got, ","), "docs/new.md") {
		t.Fatalf("edit paths = %v", got)
	}
}

// TestCodexTokenSnapshotFromRollout verifies that the per-turn token
// delta is computed correctly from a synthetic rollout.jsonl. This is
// the contract that downstream USD cost rollups depend on.
func TestCodexTokenSnapshotFromRollout(t *testing.T) {
	dir := t.TempDir()
	rollout := filepath.Join(dir, "rollout-2026-05-test.jsonl")

	lines := []map[string]any{
		{"type": "session_meta", "payload": map[string]any{"id": "cdx-tok-1"}},
		// turn A starts - baseline = 0 input/0 output (first turn)
		{"type": "turn_context", "payload": map[string]any{"turn_id": "turn-A"}},
		// pre-model snapshot
		{"type": "event_msg", "payload": map[string]any{
			"type": "token_count",
			"info": map[string]any{
				"total_token_usage":    map[string]any{"input_tokens": 100, "output_tokens": 0, "total_tokens": 100},
				"model_context_window": 200000,
			},
		}},
		// model activity → next token_count is final for this turn
		{"type": "response_item", "payload": map[string]any{
			"type": "reasoning",
			"summary": []map[string]any{
				{"type": "summary_text", "text": "Attribute the turn usage."},
			},
		}},
		{"type": "response_item", "payload": map[string]any{"type": "message", "role": "assistant"}},
		{"type": "event_msg", "payload": map[string]any{
			"type": "token_count",
			"info": map[string]any{
				"total_token_usage": map[string]any{"input_tokens": 1500, "output_tokens": 400, "cached_input_tokens": 800, "reasoning_output_tokens": 150, "total_tokens": 1900},
			},
		}},
	}
	var buf []byte
	for _, l := range lines {
		b, err := json.Marshal(l)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		buf = append(buf, b...)
		buf = append(buf, '\n')
	}
	if err := os.WriteFile(rollout, buf, 0o600); err != nil {
		t.Fatalf("write rollout: %v", err)
	}
	snap, ok := readTokenUsageForTurn(rollout, "turn-A")
	if !ok {
		t.Fatalf("expected token snapshot, got none")
	}
	// Per-turn delta = final cumulative - pre-model baseline. The
	// pre-model snapshot was 100 input / 0 output, so the delta we
	// attribute to this turn's model activity is 1400 / 400 /
	// 800-cached / 150-reasoning. Crucially the BASELINE (100
	// cumulative input before the turn started thinking) belongs to
	// system/dialog overhead, not this turn.
	if snap.TurnUsage.InputTokens != 1400 {
		t.Errorf("input_tokens: got %d want 1400", snap.TurnUsage.InputTokens)
	}
	if snap.TurnUsage.OutputTokens != 400 {
		t.Errorf("output_tokens: got %d want 400", snap.TurnUsage.OutputTokens)
	}
	if snap.TurnUsage.CachedInputTokens != 800 {
		t.Errorf("cached_input_tokens: got %d want 800", snap.TurnUsage.CachedInputTokens)
	}
	if snap.TurnUsage.ReasoningOutputTokens != 150 {
		t.Errorf("reasoning_output_tokens: got %d want 150", snap.TurnUsage.ReasoningOutputTokens)
	}
	if snap.TotalUsage.InputTokens != 1500 {
		t.Errorf("total cumulative input: got %d want 1500", snap.TotalUsage.InputTokens)
	}
	if !snap.ReasoningObserved || snap.ReasoningSummary != "Attribute the turn usage." {
		t.Errorf("reasoning summary: got %q observed=%v", snap.ReasoningSummary, snap.ReasoningObserved)
	}
}

func TestCodexReasoningSummaryFromRollout(t *testing.T) {
	dir := t.TempDir()
	rollout := filepath.Join(dir, "rollout-reasoning.jsonl")
	lines := []map[string]any{
		{"type": "turn_context", "payload": map[string]any{"turn_id": "turn-A"}},
		{"type": "response_item", "payload": map[string]any{
			"type":              "reasoning",
			"summary":           []map[string]any{{"type": "summary_text", "text": "Inspect the adapter."}},
			"encrypted_content": "must-never-be-exported",
		}},
		{"type": "response_item", "payload": map[string]any{
			"item": map[string]any{
				"type":              "reasoning",
				"summary":           []map[string]any{{"type": "summary_text", "text": "Wire the public summary."}},
				"encrypted_content": "also-private",
			},
		}},
		{"type": "turn_context", "payload": map[string]any{"turn_id": "turn-B"}},
		{"type": "response_item", "payload": map[string]any{
			"type": "reasoning", "summary": []map[string]any{{"text": "wrong turn"}},
		}},
	}
	var buf []byte
	for _, line := range lines {
		raw, err := json.Marshal(line)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		buf = append(buf, raw...)
		buf = append(buf, '\n')
	}
	if err := os.WriteFile(rollout, buf, 0o600); err != nil {
		t.Fatalf("write rollout: %v", err)
	}

	summary, observed := readReasoningSummaryForTurn(rollout, "turn-A")
	if !observed {
		t.Fatal("expected reasoning item to be observed")
	}
	if want := "Inspect the adapter.\n\nWire the public summary."; summary != want {
		t.Fatalf("summary: got %q want %q", summary, want)
	}
	if strings.Contains(summary, "private") || strings.Contains(summary, "wrong turn") {
		t.Fatalf("summary leaked excluded rollout content: %q", summary)
	}
}

func TestCodexReasoningObservedWithEmptySummary(t *testing.T) {
	dir := t.TempDir()
	rollout := filepath.Join(dir, "rollout-empty-reasoning.jsonl")
	body := []byte("{\"type\":\"turn_context\",\"payload\":{\"turn_id\":\"turn-A\"}}\n" +
		"{\"type\":\"response_item\",\"payload\":{\"type\":\"reasoning\",\"summary\":[],\"encrypted_content\":\"opaque\"}}\n")
	if err := os.WriteFile(rollout, body, 0o600); err != nil {
		t.Fatalf("write rollout: %v", err)
	}
	summary, observed := readReasoningSummaryForTurn(rollout, "turn-A")
	if !observed || summary != "" {
		t.Fatalf("got summary=%q observed=%v, want empty and observed", summary, observed)
	}
}

func TestCodexStopEmitsPublicReasoningSummary(t *testing.T) {
	withIsolatedCache(t)
	dir := t.TempDir()
	rollout := filepath.Join(dir, "rollout-stop-reasoning.jsonl")
	body := []byte("{\"type\":\"turn_context\",\"payload\":{\"turn_id\":\"turn-A\"}}\n" +
		"{\"type\":\"response_item\",\"payload\":{\"type\":\"reasoning\",\"summary\":[{\"type\":\"summary_text\",\"text\":\"Check the rollout parser.\"}],\"encrypted_content\":\"opaque\"}}\n")
	if err := os.WriteFile(rollout, body, 0o600); err != nil {
		t.Fatalf("write rollout: %v", err)
	}

	em := &recordingEmitter{}
	payload, err := json.Marshal(map[string]any{
		"hook_event_name":  "Stop",
		"session_id":       "session-A",
		"turn_id":          "turn-A",
		"transcript_path":  rollout,
		"timestamp":        time.Now().UTC().Format(time.RFC3339Nano),
		"stop_hook_active": true,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := handle(context.Background(), normalize.Input{
		Vendor:         "codex",
		Event:          "Stop",
		Payload:        payload,
		ContentCapture: "full",
		Emit:           em,
	}); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if len(em.llmTurns) != 1 {
		t.Fatalf("expected one LLM turn, got %d", len(em.llmTurns))
	}
	turn := em.llmTurns[0]
	if turn.ThoughtText != "Check the rollout parser." {
		t.Fatalf("thought text: got %q", turn.ThoughtText)
	}
	if got := turn.Extras["coding_agent.llm.reasoning.summary.available"]; got != "true" {
		t.Fatalf("summary availability: got %q want true", got)
	}
}

// TestCodexSubagentLinkFromSessionMeta verifies that a `session_meta`
// block with subagent fields is cached into sessionstate so subsequent
// spans inherit `coding_agent.agent.parent_id`.
func TestCodexSubagentLinkFromSessionMeta(t *testing.T) {
	withIsolatedCache(t)

	dir := t.TempDir()
	rollout := filepath.Join(dir, "rollout-2026-05-sub.jsonl")
	body := map[string]any{
		"type": "session_meta",
		"payload": map[string]any{
			"id":                "cdx-child",
			"thread_source":     "subagent",
			"parent_session_id": "cdx-parent",
			"agent_role":        "code-reviewer",
			"agent_nickname":    "reviewer-1",
			"agent_depth":       2,
		},
	}
	raw, _ := json.Marshal(body)
	if err := os.WriteFile(rollout, append(raw, '\n'), 0o600); err != nil {
		t.Fatalf("write rollout: %v", err)
	}
	em := &recordingEmitter{}
	body2, _ := json.Marshal(map[string]any{
		"hook_event_name": "SessionStart",
		"session_id":      "cdx-child",
		"transcript_path": rollout,
		"timestamp":       time.Now().UTC().Format(time.RFC3339Nano),
		"cwd":             "/tmp",
	})
	if err := handle(context.Background(), normalize.Input{
		Vendor:         "codex",
		Event:          "SessionStart",
		Payload:        body2,
		ContentCapture: "full",
		Emit:           em,
	}); err != nil {
		t.Fatalf("SessionStart: %v", err)
	}
	st := sessionstate.Load("cdx-child", "codex")
	if st == nil || st.CodexSubagent == nil {
		t.Fatalf("expected subagent link, got %+v", st)
	}
	if st.CodexSubagent.ParentSessionID != "cdx-parent" {
		t.Errorf("parent_session_id: got %q want cdx-parent", st.CodexSubagent.ParentSessionID)
	}
	if st.CodexSubagent.AgentRole != "code-reviewer" {
		t.Errorf("agent_role: got %q", st.CodexSubagent.AgentRole)
	}

	// The session-root span emitted on SessionStart must carry the
	// parent linkage. Without this, the Sessions list shows the
	// subagent as a standalone row instead of folding it under the
	// parent's chat.
	if len(em.sessions) == 0 {
		t.Fatalf("expected SessionStart to emit a session span")
	}
	gotParent := em.sessions[0].Extras["coding_agent.agent.parent_id"]
	if gotParent != "cdx-parent" {
		t.Errorf("SessionStart extras parent_id: got %q want cdx-parent", gotParent)
	}

	// A later Stop event in the same subagent session must re-stamp
	// the parent_id on the llm.turn span - long subagents accumulate
	// many turns and the UI's chat_id fold relies on the resource
	// attr being present on *every* span this session emits, not just
	// the root.
	turn := "turn-sub-1"
	stopBody, _ := json.Marshal(map[string]any{
		"hook_event_name":        "Stop",
		"session_id":             "cdx-child",
		"turn_id":                turn,
		"transcript_path":        rollout,
		"last_assistant_message": "ok",
		"timestamp":              time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err := handle(context.Background(), normalize.Input{
		Vendor:         "codex",
		Event:          "Stop",
		Payload:        stopBody,
		ContentCapture: "full",
		Emit:           em,
	}); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if len(em.llmTurns) == 0 {
		t.Fatalf("expected Stop to emit an llm.turn")
	}
	turnParent := em.llmTurns[0].Extras["coding_agent.agent.parent_id"]
	if turnParent != "cdx-parent" {
		t.Errorf("llm.turn extras parent_id: got %q want cdx-parent", turnParent)
	}
}
