package hook

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/audit"
	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/guardrails"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/harvest"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/port"
	"github.com/ishanjainn/superopen/internal/retention"
)

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
	if !memoryOn && portExtra == "" {
		return
	}

	var packText string
	var sections any
	var charCount int
	if memoryOn {
		store := memory.NewStore(paths)
		pack, err := store.BuildSessionContext(12000, "", memory.ModePersistent)
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

func maybeHarvestOnSessionEnd(vendor, event, sessionID, cwd string) {
	root := findRepoRoot(cwd)
	paths := harness.Resolve(root)
	if !paths.Exists() {
		return
	}
	cfg, err := config.Load(paths.Config)
	if err != nil {
		cfg = config.Default()
	}

	if isSessionStartEvent(event) {
		_ = harvest.FlushPending(paths, cfg, sessionID)
		_, _ = retention.Prune(paths, cfg)
		return
	}

	if isSessionEndEvent(event) {
		if sessionID == "" {
			return
		}
		_ = harvest.ClearPending(paths, sessionID)
		_, _ = harvest.Run(paths, cfg, sessionID, harvest.TriggerSessionEnd)
		_ = harvest.MarkFinalizePending(paths, sessionID)
		_, _ = harvest.IdleSweep(paths, cfg)
		return
	}

	// Vendors without a real session-end hook (Codex Stop, and any Stop-only adapter).
	if isTurnBoundaryHarvestEvent(vendor, event) {
		if sessionID != "" {
			_ = harvest.MarkPending(paths, sessionID)
			_, _ = harvest.RunDebounced(paths, cfg, sessionID, harvest.TriggerTurnEnd)
			_ = harvest.MarkFinalizePending(paths, sessionID)
		}
		_, _ = harvest.IdleSweep(paths, cfg)
	}
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
	cmd, path := extractToolTargets(payload)
	dec, matcher, deny := decideGuardrail(eng, cmd, path)
	if !deny {
		return false
	}
	_ = audit.Append(paths, audit.Event{
		Action: "deny", Type: "policy", Key: dec.Rule, Detail: dec.Reason,
		Vendor: vendor, Attrs: map[string]string{"matcher": matcher, "command": truncate(cmd, 120), "path": truncate(path, 120)},
	})
	writeDeny(vendor, event, dec)
	return true
}

// decideGuardrail evaluates command/path against guardrails.
// Empty targets never deny (avoids zero-value Decision{Allow:false} false positives).
func decideGuardrail(eng guardrails.Engine, cmd, path string) (dec guardrails.Decision, matcher string, deny bool) {
	if cmd == "" && path == "" {
		return guardrails.Decision{Allow: true}, "", false
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
			"permission": "deny",
			"userMessage": reason,
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
	enc := json.NewEncoder(os.Stdout)
	return enc.Encode(v)
}

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

func extractToolTargets(payload []byte) (cmd, path string) {
	var m map[string]any
	if json.Unmarshal(payload, &m) != nil {
		return "", ""
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
	return cmd, path
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
	debounceFile := filepath.Join(paths.MemoryDir, "approval-mismatch-at")
	if data, err := os.ReadFile(debounceFile); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			parts := strings.SplitN(strings.TrimSpace(line), "\t", 2)
			if len(parts) != 2 || parts[0] != key {
				continue
			}
			if t, err := time.Parse(time.RFC3339, parts[1]); err == nil && time.Since(t) < time.Hour {
				return
			}
		}
	}
	_ = os.MkdirAll(paths.MemoryDir, 0o755)
	_ = os.WriteFile(debounceFile, []byte(key+"\t"+time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)

	_ = audit.Append(paths, audit.Event{
		Action:  "conflict_skip",
		Type:    "approval",
		Vendor:  vendor,
		Session: sessionID,
		Detail:  fmt.Sprintf("session=%s · policy=%s", mode, ceiling),
		Attrs:   map[string]string{"session_mode": mode, "policy_ceiling": ceiling, "event": event},
	})
}

