package hook

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/audit"
	"github.com/ishanjainn/superopen/internal/coding/normalize"
	"github.com/ishanjainn/superopen/internal/coding/pricing"
	"github.com/ishanjainn/superopen/internal/coding/sessionstate"
	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/execx"
	"github.com/ishanjainn/superopen/internal/guardrails"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/harvest"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/port"
	"github.com/ishanjainn/superopen/internal/redact"
	"github.com/ishanjainn/superopen/internal/runtimestate"
	"github.com/ishanjainn/superopen/internal/session"
)

const dynamicMemorySessionTokens = 2000

// maybeInjectDynamicMemory performs bounded, local-only recall on model-visible
// prompt and file hooks. It is intentionally fail-open: any unsupported vendor,
// missing state, parse failure, or budget exhaustion returns no hook output.
func maybeInjectDynamicMemory(vendor, event, sessionID, cwd string, probe peekedContext, cached *sessionstate.State, emit normalize.Emitter) {
	if sessionID == "" || cached == nil {
		return
	}
	root := findRepoRoot(cwd)
	paths := harness.Resolve(root)
	if !paths.Exists() {
		return
	}
	if cfg, err := config.Load(paths.Config); err == nil && !cfg.MemoryEnabled() {
		return
	}

	e := strings.ToLower(strings.TrimSpace(event))
	isPrompt := isPromptRecallEvent(vendor, e)
	safeToolPath := redact.StringFull(probe.ToolPath)
	isFile := isFileRecallEvent(vendor, e) && strings.TrimSpace(safeToolPath) != ""
	observedFile := isFileObservationEvent(vendor, e) && strings.TrimSpace(safeToolPath) != ""
	if observedFile {
		cached.RecentPaths = appendRecentPath(cached.RecentPaths, repoPath(root, safeToolPath), 32)
	}
	if !isPrompt && !isFile {
		if observedFile {
			sessionstate.Save(sessionID, vendor, cached)
		}
		return
	}
	remaining := dynamicMemorySessionTokens - int(cached.MemoryTokens)
	if remaining <= 0 {
		return
	}
	maxTokens := memory.DefaultTurnTokens
	queryText := dynamicMemoryQueryText(probe)
	queryPaths := append([]string(nil), cached.RecentPaths...)
	if isFile {
		maxTokens = memory.DefaultFileTokens
		queryPaths = []string{repoPath(root, safeToolPath)}
	}
	promptHash := ""
	turnID := cached.MemoryTurnID
	turnTokens := cached.MemoryTurnTokens
	if isPrompt {
		promptHash = fmt.Sprintf("%x", sha256.Sum256([]byte(queryText)))
		if promptHash == cached.LastPromptHash {
			return
		}
		turnID = nonEmptyTurnID(probe.TurnID, promptHash)
		turnTokens = 0
	}
	turnRemaining := memory.DefaultTurnTokens - int(turnTokens)
	if maxTokens > turnRemaining {
		maxTokens = turnRemaining
	}
	if maxTokens > remaining {
		maxTokens = remaining
	}
	if maxTokens <= 0 {
		sessionstate.Save(sessionID, vendor, cached)
		return
	}
	if cached.MemorySeen == nil {
		cached.MemorySeen = map[string]string{}
	}
	hits, err := memory.NewStore(paths).Retrieve(memory.RetrievalQuery{
		Text: queryText, Vendor: vendor, Paths: queryPaths, Seen: cached.MemorySeen,
		Branch: cached.Branch, Worktree: root,
		MaxTokens: maxTokens, MaxResults: memory.DefaultTurnHits, FileOnly: isFile,
	})
	if err != nil || len(hits) == 0 {
		sessionstate.Save(sessionID, vendor, cached)
		return
	}
	// Cursor exposes this prompt boundary for observation but not same-turn
	// model context. Ranking occurs for diagnostics, while no retrieval event,
	// token use, or injection claim is recorded.
	if vendor == "cursor" {
		sessionstate.Save(sessionID, vendor, cached)
		return
	}
	text := memory.FormatRetrieval(hits)
	if text == "" {
		return
	}
	// Delivery must succeed before Superopen claims an injection or consumes
	// deduplication/token budget. A broken stdout remains fail-open and the
	// next supported hook may retry the same memory.
	if err := writeDynamicContext(vendor, event, probe, text, isFile); err != nil {
		if observedFile {
			sessionstate.Save(sessionID, vendor, cached)
		}
		return
	}
	var used int64
	var ids, reasons, targets []string
	for _, hit := range hits {
		cached.MemorySeen[hit.Fingerprint] = hit.ContentID
		used += hit.EstimatedTokens
		ids = append(ids, hit.Fingerprint)
		reasons = append(reasons, strings.Join(hit.Reasons, "+"))
		if hit.TargetPath != "" {
			targets = append(targets, hit.TargetPath)
		}
	}
	if isPrompt {
		cached.LastPromptHash = promptHash
		cached.MemoryTurnID = turnID
		cached.MemoryTurnTokens = 0
	}
	cached.MemoryTokens += used
	cached.MemoryTurnTokens += used
	sessionstate.Save(sessionID, vendor, cached)
	_ = emit.EmitEvent(normalize.EventEmission{SessionID: sessionID, Name: "superopen.memory.retrieved", At: time.Now(), Attrs: map[string]any{
		"coding_agent.client": vendor, "superopen.memory.pattern_ids": ids,
		"superopen.memory.scores": retrievalScores(hits), "superopen.memory.reasons": reasons,
		"superopen.memory.estimated_tokens": used, "superopen.memory.target_paths": targets,
		"superopen.memory.turn_id": nonEmptyTurnID(probe.TurnID, turnID), "superopen.memory.delivery": deliveryFor(vendor, isFile),
	}})
}

func dynamicMemoryQueryText(probe peekedContext) string {
	text := probe.Prompt
	if probe.TransformedPrompt != "" {
		text = probe.TransformedPrompt
	}
	return redact.StringFull(text)
}

func nonEmptyTurnID(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "unresolved"
}

func isPromptRecallEvent(vendor, event string) bool {
	switch vendor {
	case "claude-code", "codex":
		return event == "userpromptsubmit" || event == "user_prompt_submit"
	case "gemini":
		return event == "beforeagent"
	case "opencode":
		return strings.Contains(event, "message.updated") || event == "chat.message"
	case "pi":
		return event == "before_agent_start" || event == "turn_start"
	case "copilot-cli":
		return event == "userprompttransformed"
	case "cursor":
		return event == "beforesubmitprompt"
	}
	return false
}

func isFileRecallEvent(vendor, event string) bool {
	switch vendor {
	case "claude-code", "codex":
		return event == "pretooluse"
	case "gemini":
		return event == "beforetool"
	case "copilot-cli":
		return event == "posttooluse"
	case "opencode":
		return strings.Contains(event, "tool.execute.before")
	}
	return false
}

func isFileObservationEvent(vendor, event string) bool {
	if isFileRecallEvent(vendor, event) {
		return true
	}
	switch vendor {
	case "cursor":
		return event == "pretooluse" || event == "beforereadfile"
	case "pi":
		return strings.Contains(event, "tool_execution_start") || strings.Contains(event, "tool.execute.before")
	}
	return false
}

func writeDynamicContext(vendor, event string, probe peekedContext, text string, file bool) error {
	switch vendor {
	case "opencode", "pi":
		return writeHookJSON(map[string]any{"inject_context": text, "additional_context": text})
	case "copilot-cli":
		if file {
			return writeHookJSON(map[string]any{"additionalContext": text})
		}
		base := probe.TransformedPrompt
		if base == "" {
			base = probe.Prompt
		}
		return writeHookJSON(map[string]any{"modifiedTransformedPrompt": base + "\n\n" + text})
	default:
		return writeHookJSON(map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": event, "additionalContext": text, "permissionDecision": "allow"}, "additionalContext": text, "additional_context": text})
	}
}

func retrievalScores(hits []memory.RetrievalHit) []string {
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, fmt.Sprintf("%.3f", h.Score))
	}
	return out
}
func deliveryFor(vendor string, file bool) string {
	if file {
		return vendor + ":file"
	}
	return vendor + ":prompt"
}
func appendRecentPath(paths []string, path string, limit int) []string {
	if path == "" {
		return paths
	}
	out := []string{path}
	for _, p := range paths {
		if p != path {
			out = append(out, p)
		}
		if len(out) >= limit {
			break
		}
	}
	return out
}
func repoPath(root, path string) string {
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		return filepath.ToSlash(filepath.Clean(path))
	}
	rel, err := filepath.Rel(root, path)
	if err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

// maybeInjectMemory injects Active Context and any pending Port resume on SessionStart.
func maybeInjectMemory(vendor, event, sessionID, cwd string) {
	if !isSessionStartEvent(event) {
		return
	}
	root := findRepoRoot(cwd)
	paths := harness.Resolve(root)
	if !paths.Exists() {
		return
	}

	// Port resume is independent of memory.enabled - always inject when armed.
	portExtra := port.ConsumePendingResume(root)

	memoryOn := true
	if cfg, err := config.Load(paths.Config); err == nil {
		memoryOn = cfg.MemoryEnabled()
	}
	reviewExtra := session.NewStore(paths).PreviousReviewContext(vendor, sessionID)
	if !memoryOn && portExtra == "" && reviewExtra == "" {
		return
	}

	var packText string
	var sections any
	var charCount int
	if memoryOn {
		store := memory.NewStore(paths)
		pack, err := store.BuildSessionContextForVendor(1500, "", memory.ModePersistent, vendor)
		if err == nil {
			packText = pack.Text
			sections = pack.Sections
			charCount = pack.CharCount
		}
	}
	if portExtra != "" {
		if packText != "" {
			packText = packText + "\n\n## Ported conversation resume\n\n" + portExtra
		} else {
			packText = "## Ported conversation resume\n\n" + portExtra
		}
		charCount = len(packText)
	}
	if reviewExtra != "" {
		if packText != "" {
			packText += "\n\n" + reviewExtra
		} else {
			packText = reviewExtra
		}
		charCount = len(packText)
	}
	packText = capEstimatedTokens(packText, 1500)
	charCount = len(packText)
	if strings.TrimSpace(packText) == "" {
		return
	}

	_ = audit.Append(paths, audit.Event{
		Action:  "session.inject_active_context",
		Type:    "session",
		Vendor:  vendor,
		Session: sessionID,
		Detail:  fmt.Sprintf("chars=%d sections=%v port_resume=%v", charCount, sections, portExtra != ""),
	})
	switch {
	case isClaudeCodeVendor(vendor) || vendor == "codex":
		_ = writeHookJSON(map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName":     "SessionStart",
				"additionalContext": packText,
			},
		})
	case vendor == "cursor":
		_ = writeHookJSON(map[string]any{
			"additional_context": packText,
		})
	case vendor == "opencode" || vendor == "pi":
		// Plugins parse inject_context / additional_context from stdout.
		_ = writeHookJSON(map[string]any{
			"inject_context":     packText,
			"additional_context": packText,
		})
	case vendor == "gemini", vendor == "google_gemini":
		// Gemini CLI SessionStart: additionalContext (Claude-shaped) + context string.
		_ = writeHookJSON(map[string]any{
			"additionalContext": packText,
			"context":           packText,
			"hookSpecificOutput": map[string]any{
				"hookEventName":     "SessionStart",
				"additionalContext": packText,
			},
		})
	case vendor == "copilot-cli", vendor == "copilot":
		_ = writeHookJSON(map[string]any{
			"additionalContext":  packText,
			"additional_context": packText,
			"context":            packText,
		})
	}
}

func capEstimatedTokens(text string, limit int64) string {
	if pricing.EstimateTokens(text) <= limit {
		return text
	}
	runes := []rune(text)
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if pricing.EstimateTokens(string(runes[:mid])+"\n…[startup context truncated]") <= limit {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return string(runes[:lo]) + "\n…[startup context truncated]"
}

func maybeHarvestOnSessionEnd(vendor, event, sessionID, cwd string) {
	root := findRepoRoot(cwd)
	paths := harness.Resolve(root)
	if !paths.Exists() {
		return
	}
	if isSessionStartEvent(event) {
		// A new same-vendor session only schedules the immediately preceding
		// pending review. The hook never waits for evaluation or backlog work.
		if previous := harvest.PendingVendor(paths, sessionID, vendor); previous != "" {
			scheduleFinalize(root, previous)
		}
		return
	}

	if isSessionEndEvent(event) {
		if sessionID == "" {
			return
		}
		_ = harvest.MarkPending(paths, sessionID, vendor)
		// Detached post-session pipeline: materialize → graph → review → apply.
		scheduleFinalize(root, sessionID)
		return
	}

	// Vendors without a real session-end hook (Codex Stop, and any Stop-only adapter).
	if isTurnBoundaryHarvestEvent(vendor, event) {
		if sessionID != "" {
			_ = harvest.MarkPending(paths, sessionID, vendor)
		}
		// Stop is only a marker. Finalize runs on explicit close, idle handling,
		// or the next different same-vendor SessionStart.
	}
}

// scheduleFinalize kicks off so sessions finalize in the background so the
// agent hook can return immediately. Failures are swallowed (fail-soft).
func scheduleFinalize(repoRoot, sessionID string) {
	if strings.TrimSpace(sessionID) == "" {
		execx.SpawnSO(repoRoot, "sessions", "finalize")
		return
	}
	execx.SpawnSO(repoRoot, "sessions", "finalize", sessionID)
}

func isSessionEndEvent(event string) bool {
	e := strings.ToLower(strings.TrimSpace(event))
	switch e {
	case "sessionend", "session_end", "session.shutdown", "session_shutdown",
		"dispose", "agent_end", "session.idle", "session.deleted", "session_deleted":
		return true
	default:
		return strings.Contains(e, "session.end") || strings.Contains(e, "session_end")
	}
}

// isTurnBoundaryHarvestEvent is the Codex-class fallback when the vendor
// never emits SessionEnd. Stop fires after every assistant turn - harvest
// is debounced / pending-flushed rather than run raw on every Stop.
func isTurnBoundaryHarvestEvent(vendor, event string) bool {
	e := strings.ToLower(strings.TrimSpace(event))
	if e != "stop" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(vendor)) {
	case "codex", "gemini", "copilot-cli", "copilot":
		return true
	default:
		// Claude/Cursor have SessionEnd; their Stop must not harvest.
		return false
	}
}

// maybeEnforceGuardrails returns true if the hook already wrote a deny response.
func maybeEnforceGuardrails(vendor, event string, payload []byte, cwd string) bool {
	if !isToolGateEvent(event) {
		return false
	}
	root := findRepoRoot(cwd)
	paths := harness.Resolve(root)
	if !paths.Exists() {
		return false
	}
	if cfg, err := config.Load(paths.Config); err == nil && !cfg.GuardrailsEnabled() {
		return false
	}
	eng, err := guardrails.Load(paths)
	if err != nil {
		return false
	}
	tool, cmd, path := extractToolTargets(payload)
	dec, matcher, deny := decideGuardrail(eng, tool, cmd, path)
	if !deny {
		return false
	}
	_ = audit.Append(paths, audit.Event{
		Action: "deny", Type: "policy", Key: dec.Rule, Detail: dec.Reason,
		Vendor: vendor, Attrs: map[string]string{"matcher": matcher, "tool": truncate(tool, 120), "command": truncate(cmd, 120), "path": truncate(path, 120)},
	})
	writeDeny(vendor, event, dec)
	return true
}

// decideGuardrail evaluates command/path against guardrails.
// Empty targets never deny (avoids zero-value Decision{Allow:false} false positives).
func decideGuardrail(eng guardrails.Engine, tool, cmd, path string) (dec guardrails.Decision, matcher string, deny bool) {
	if tool == "" && cmd == "" && path == "" {
		return guardrails.Decision{Allow: true}, "", false
	}
	if tool != "" {
		dec = eng.CheckTool(tool)
		if !dec.Allow {
			return dec, "tool", true
		}
	}
	if cmd != "" {
		dec = eng.CheckCommand(cmd)
		if !dec.Allow {
			return dec, "command", true
		}
	}
	if path != "" {
		dec = eng.CheckPath(path)
		if !dec.Allow {
			return dec, "path", true
		}
	}
	return guardrails.Decision{Allow: true}, "", false
}

func writeDeny(vendor, event string, dec guardrails.Decision) {
	reason := dec.Reason + ": " + dec.Rule
	switch {
	case isClaudeCodeVendor(vendor):
		_ = writeHookJSON(map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName":            "PreToolUse",
				"permissionDecision":       "deny",
				"permissionDecisionReason": reason,
			},
		})
	case vendor == "cursor":
		// Cursor shell/file hooks: permission:"deny"
		_ = writeHookJSON(map[string]any{
			"permission":   "deny",
			"userMessage":  reason,
			"agentMessage": reason,
		})
	default:
		_ = writeHookJSON(map[string]any{
			"decision": "deny",
			"reason":   reason,
		})
	}
}

func writeHookJSON(v any) error {
	enc := json.NewEncoder(hookJSONOutput())
	return enc.Encode(v)
}

var hookJSONOutput = func() io.Writer { return os.Stdout }

func isSessionStartEvent(event string) bool {
	e := strings.ToLower(event)
	return e == "sessionstart" || e == "session_start" || e == "session.created" || strings.Contains(e, "session.created")
}

func isToolGateEvent(event string) bool {
	e := strings.ToLower(event)
	switch e {
	case "pretooluse", "pre_tool_use", "beforeshellexecution", "before_shell_execution",
		"beforereadfile", "before_read_file", "afterfileedit", "after_file_edit",
		"tool.execute.before", "beforetool", "before_tool", "tool_start":
		return true
	default:
		return false
	}
}

var nestedToolCallRE = regexp.MustCompile(`tools\.([A-Za-z0-9_]+)\s*\(`)

func extractToolTargets(payload []byte) (tool, cmd, path string) {
	var m map[string]any
	if json.Unmarshal(payload, &m) != nil {
		return "", "", ""
	}
	for _, key := range []string{"tool_name", "toolName", "tool", "name"} {
		if value, ok := m[key].(string); ok && strings.TrimSpace(value) != "" {
			tool = value
			break
		}
	}
	// Common shapes across vendors
	if v, ok := m["tool_input"].(map[string]any); ok {
		if c, ok := v["command"].(string); ok {
			cmd = c
		}
		if p, ok := v["file_path"].(string); ok {
			path = p
		}
		if p, ok := v["path"].(string); ok && path == "" {
			path = p
		}
		// Codex code mode exposes nested tools through a generic exec hook.
		// Recover the actual callable name so denied_tools can enforce it.
		if code, ok := v["code"].(string); ok {
			if match := nestedToolCallRE.FindStringSubmatch(code); len(match) == 2 {
				tool = match[1]
			}
		}
	}
	if c, ok := m["command"].(string); ok && cmd == "" {
		cmd = c
	}
	if p, ok := m["file_path"].(string); ok && path == "" {
		path = p
	}
	if p, ok := m["path"].(string); ok && path == "" {
		path = p
	}
	// Cursor beforeShellExecution
	if p, ok := m["command"].(string); ok && cmd == "" {
		cmd = p
	}
	return tool, cmd, path
}

func findRepoRoot(cwd string) string {
	start := cwd
	if start == "" {
		start, _ = os.Getwd()
	}
	root, err := harness.FindRoot(start)
	if err != nil {
		return start
	}
	return root
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
func maybeAuditApproval(vendor, event, permissionMode, sessionID, cwd string) {
	if permissionMode == "" {
		return
	}
	root := findRepoRoot(cwd)
	paths := harness.Resolve(root)
	if !paths.Exists() {
		return
	}
	eng, err := guardrails.Load(paths)
	if err != nil {
		return
	}
	ceiling := strings.ToLower(eng.Approval())
	mode := strings.ToLower(permissionMode)
	// Ordinal: yolo < auto < interactive (interactive = tightest)
	ord := map[string]int{"yolo": 0, "never": 0, "auto": 1, "on-failure": 1, "interactive": 2, "on-request": 2, "untrusted": 2}
	if ord[mode] >= ord[ceiling] {
		return
	}
	// Debounce: ≤1 event per session+mode+ceiling per hour.
	key := sessionID + "|" + mode + "|" + ceiling
	if key == "||"+ceiling || strings.Trim(key, "|") == ceiling {
		key = vendor + "|" + mode + "|" + ceiling
	}
	record, err := runtimestate.TouchIfStale(root, "approval_mismatch:"+key, time.Hour)
	if err != nil || !record {
		return
	}

	_ = audit.Append(paths, audit.Event{
		Action:  "conflict_skip",
		Type:    "approval",
		Vendor:  vendor,
		Session: sessionID,
		Detail:  fmt.Sprintf("session=%s · policy=%s", mode, ceiling),
		Attrs:   map[string]string{"session_mode": mode, "policy_ceiling": ceiling, "event": event},
	})
}
