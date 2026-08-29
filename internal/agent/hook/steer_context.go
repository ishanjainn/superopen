package hook

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/agent/sessionstate"
	"github.com/ishanjainn/superopen/internal/agent/steer"
	"github.com/ishanjainn/superopen/internal/graph/engine"
	"github.com/ishanjainn/superopen/internal/harvest"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/paths"
)

const (
	// augmentMinTermLen skips terms too short to rank meaningfully.
	augmentMinTermLen = 3
)

// emitSteerContext writes vendor-specific additionalContext when the event
// supports it. Unknown protocols get no stdout (fail-open).
func emitSteerContext(vendor, event, kind string, payload []byte) {
	decision, ok := steerDecisionFor(vendor, event, kind, payload)
	if !ok {
		return
	}
	var body any
	switch vendor {
	case "claude-code":
		hso := map[string]any{
			"hookEventName": decision.hookEvent,
		}
		if decision.deny {
			hso["permissionDecision"] = "deny"
			hso["permissionDecisionReason"] = decision.text
		} else if decision.text != "" {
			hso["additionalContext"] = decision.text
		} else {
			return
		}
		body = map[string]any{"hookSpecificOutput": hso}
	case "cursor":
		if decision.deny || decision.text == "" {
			// Cursor has no PreToolUse deny contract matching Claude.
			if decision.text == "" {
				return
			}
		}
		body = map[string]any{"additional_context": decision.text}
	case "codex":
		// Codex Desktop rejects additionalContext on PreToolUse.
		if event == "PreToolUse" {
			return
		}
		if decision.text == "" {
			return
		}
		body = map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName":     decision.hookEvent,
				"additionalContext": decision.text,
			},
		}
	case "gemini", "copilot-cli", "opencode", "pi":
		if decision.text == "" {
			return
		}
		body = map[string]any{"additionalContext": decision.text}
	default:
		return
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(body); err != nil {
		fmt.Fprintf(os.Stderr, "so hook: steer stdout: %v\n", err)
	}
}

type steerDecision struct {
	text      string
	hookEvent string
	deny      bool
}

func steerTextFor(vendor, event string, payload []byte) (text, hookEvent string, ok bool) {
	d, ok := steerDecisionFor(vendor, event, "", payload)
	if !ok {
		return "", "", false
	}
	return d.text, d.hookEvent, true
}

func steerDecisionFor(vendor, event, kind string, payload []byte) (steerDecision, bool) {
	if !managedFromPayload(payload) {
		return steerDecision{}, false
	}
	ev := strings.TrimSpace(event)
	var sessionEvent, toolEvent bool
	var compactEvent bool
	switch vendor {
	case "claude-code", "codex":
		sessionEvent = ev == "SessionStart" || ev == "UserPromptSubmit" || ev == "SubagentStart"
		toolEvent = ev == "PreToolUse"
	case "cursor":
		sessionEvent = ev == "sessionStart" || ev == "beforeSubmitPrompt" || ev == "subagentStart"
		toolEvent = ev == "preToolUse" || ev == "beforeReadFile"
		compactEvent = ev == "preCompact"
	case "gemini":
		lower := strings.ToLower(ev)
		sessionEvent = lower == "sessionstart" || lower == "beforeagent"
		toolEvent = lower == "beforetool"
	case "copilot-cli":
		sessionEvent = ev == "sessionStart" || ev == "userPromptSubmitted"
		toolEvent = ev == "preToolUse"
	case "opencode":
		lower := strings.ToLower(ev)
		sessionEvent = strings.Contains(lower, "session.created") || strings.Contains(lower, "session.end") ||
			strings.Contains(lower, "session.deleted") || strings.Contains(lower, "session.idle") ||
			lower == "sessionstart" || lower == "session_start"
		toolEvent = strings.Contains(lower, "tool.execute.before") || lower == "pretooluse"
	case "pi":
		lower := strings.ToLower(ev)
		sessionEvent = lower == "session_start" || lower == "sessionstart" ||
			lower == "before_agent_start" || lower == "session_shutdown" || lower == "agent_end"
		toolEvent = lower == "tool.execute.before" || lower == "tool_execution_start"
	default:
		return steerDecision{}, false
	}

	switch {
	case compactEvent:
		if text := memoryCompactText(payload); text != "" {
			return steerDecision{text: text, hookEvent: ev}, true
		}
	case sessionEvent:
		if ev == "SubagentStart" || ev == "subagentStart" {
			return subagentSteer(payload, vendor, ev)
		}
		if isSessionStartEvent(vendor, ev) {
			if text := sessionStartText(payload, vendor); text != "" {
				return steerDecision{text: text, hookEvent: ev}, true
			}
			return steerDecision{}, false
		}
		if isPromptSubmitEvent(vendor, ev) {
			if text := promptSubmitText(payload, vendor); text != "" {
				return steerDecision{text: text, hookEvent: ev}, true
			}
			return steerDecision{}, false
		}
		// Stop, SessionEnd, and other lifecycle events are observability-only.
		return steerDecision{}, false
	case toolEvent:
		// Codex does not consume additionalContext / deny from PreToolUse
		// the way Claude and Cursor do; emitting steer text would be dropped
		// by the host. Session and prompt events still fire.
		if vendor == "codex" {
			return steerDecision{}, false
		}
		return graphGate(payload, vendor, kind, ev)
	}
	return steerDecision{}, false
}

func graphGate(payload []byte, vendor, kind, hookEvent string) (steerDecision, bool) {
	tool := toolNameFromPayload(payload)
	if strings.HasPrefix(strings.ToLower(tool), "graph_") {
		return steerDecision{}, false
	}
	cmd := bashCommandFromPayload(payload)
	if commandLooksLikeSo(cmd) {
		if bashLooksLikeGraphQuery(cmd) {
			if engine.QueryStampFreshFor(stampRoot(payload), steerSessionID(payload)) {
				if !claimQueryRepeat(payload, vendor) {
					return steerDecision{}, false
				}
				return steerDecision{text: steer.QueryRepeatNudge(), hookEvent: hookEvent}, true
			}
			engine.RecordQueryStampFor(stampRoot(payload), steerSessionID(payload))
		}
		return steerDecision{}, false
	}
	gate := strings.ToLower(strings.TrimSpace(kind))
	if isBashTool(tool) {
		switch {
		case bashLooksLikeSearch(cmd):
			gate = "search"
		case bashLooksLikeRead(cmd):
			gate = "read"
		case bashLooksLikeListing(cmd):
			gate = "search"
		default:
			if gate == "search" || gate == "read" {
				return steerDecision{}, false
			}
		}
	}
	if gate == "" {
		switch {
		case hookEvent == "beforeReadFile" || isReadTool(tool):
			gate = "read"
		case isSearchTool(tool):
			gate = "search"
		default:
			return steerDecision{}, false
		}
	} else if !isBashTool(tool) && gate == "search" && tool != "" && !isSearchTool(tool) && !isReadTool(tool) {
		// Scoped matcher already filtered Claude; Cursor preToolUse is unscoped.
		return steerDecision{}, false
	} else if !isBashTool(tool) && gate == "read" && tool != "" && !isReadTool(tool) && hookEvent != "beforeReadFile" {
		return steerDecision{}, false
	}
	if gate == "search" && isBashTool(tool) && !bashLooksLikeSearch(cmd) && !bashLooksLikeListing(cmd) {
		return steerDecision{}, false
	}
	if isSkillDocRead(peekContext(payload).ToolPath) {
		return steerDecision{}, false
	}

	route := promptRoute(payload, vendor)
	if route == routeEmpty {
		return steerDecision{}, false
	}
	if route == routeMemory {
		if !claimMemoryNudge(payload, vendor) {
			return steerDecision{}, false
		}
		return steerDecision{text: steer.MemoryNudge(), hookEvent: hookEvent}, true
	}

	if engine.QueryStampFreshFor(stampRoot(payload), steerSessionID(payload)) {
		if gate == "read" && isSourceRead(payload, tool, hookEvent) {
			if !claimSnippetOverflow(payload, vendor) {
				return steerDecision{}, false
			}
			return steerDecision{text: steer.SnippetOverflowNudge(), hookEvent: hookEvent}, true
		}
		return steerDecision{}, false
	}
	switch gate {
	case "search":
		if !claimGraphNudge(payload, vendor) {
			return steerDecision{}, false
		}
		return steerDecision{text: steer.SearchNudge(), hookEvent: hookEvent}, true
	case "read":
		if hookStrictEnabled() && claimStrictReadDeny(payload, vendor) && isSourceRead(payload, tool, hookEvent) {
			return steerDecision{text: steer.ReadDenyReason(), hookEvent: hookEvent, deny: true}, true
		}
		if !claimGraphNudge(payload, vendor) {
			return steerDecision{}, false
		}
		return steerDecision{text: steer.ReadNudge(), hookEvent: hookEvent}, true
	default:
		return steerDecision{}, false
	}
}

func hookStrictEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("SUPEROPEN_HOOK_STRICT")))
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return false
	}
}

func claimStrictReadDeny(payload []byte, vendor string) bool {
	if engine.QueryStampFreshFor(stampRoot(payload), steerSessionID(payload)) {
		return false
	}
	sessionID := steerSessionID(payload)
	if sessionID == "" {
		return true
	}
	state := sessionstate.Load(sessionID, vendor)
	if state.StrictReadDenied {
		return false
	}
	state.StrictReadDenied = true
	sessionstate.Save(sessionID, vendor, state)
	return true
}

func isSourceRead(payload []byte, tool, hookEvent string) bool {
	path := peekContext(payload).ToolPath
	if isSkillDocRead(path) {
		return false
	}
	if hookEvent == "beforeReadFile" {
		return true
	}
	if !isReadTool(tool) {
		return false
	}
	if path == "" {
		return true
	}
	lower := strings.ToLower(path)
	if strings.Contains(lower, "/.so/") || strings.HasPrefix(filepath.Base(lower), ".") {
		return false
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".rs", ".c", ".h", ".cc", ".cpp", ".java", ".rb", ".php", ".cs", ".kt", ".swift":
		return true
	default:
		return ext != ".md" && ext != ".json" && ext != ".lock"
	}
}

func isSkillDocRead(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	lower := strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
	base := filepath.Base(lower)
	if base == "agents.md" || base == "skill.md" {
		return true
	}
	return strings.Contains(lower, "/skills/so/") || strings.Contains(lower, "/skills/superopen/")
}

func isPromptSubmitEvent(vendor, ev string) bool {
	lower := strings.ToLower(strings.TrimSpace(ev))
	switch vendor {
	case "claude-code", "codex":
		return ev == "UserPromptSubmit"
	case "cursor":
		return ev == "beforeSubmitPrompt"
	case "copilot-cli":
		return ev == "userPromptSubmitted"
	case "gemini":
		return lower == "beforeagent"
	case "pi":
		return lower == "before_agent_start"
	default:
		return strings.EqualFold(ev, "UserPromptSubmit") || strings.EqualFold(ev, "beforeSubmitPrompt") ||
			lower == "beforeagent" || lower == "before_agent_start"
	}
}

func memoryCompactText(payload []byte) string {
	root := graphRoot(payload)
	if root == "" {
		return ""
	}
	return memory.CompactSnapshot(root, steerSessionID(payload))
}

func sessionStartText(payload []byte, vendor string) string {
	root := repoRoot(payload)
	if root == "" {
		return ""
	}
	route := workspaceRoute(root)
	rememberWorkspaceRoute(payload, vendor, route)
	var core string
	switch route {
	case routeCode:
		core = steer.GraphStartLine()
	case routeMemory:
		if text := memory.SessionStartIndex(root); text != "" {
			core = text
		} else {
			core = steer.MemoryStartLine(memory.CountLiveMemories(root))
		}
	default:
		core = memory.SessionStartIndex(root)
	}
	extra := harvest.PendingSessionStartLine(root)
	if extra == "" && route == routeCode {
		extra = memory.PendingDistillLine(root)
	}
	return harvest.JoinStart(core, extra)
}

func promptSubmitText(payload []byte, vendor string) string {
	root := repoRoot(payload)
	prompt := peekContext(payload).Prompt
	kind := classifyPrompt(prompt)
	src := workspaceHasSource(root)
	n := memory.CountLiveMemories(root)
	if kind == routeCode && !src && n > 0 {
		kind = routeMemory
	}
	if kind == "" {
		if src {
			kind = routeCode
		} else if n > 0 {
			kind = routeMemory
		} else {
			kind = routeEmpty
		}
	}
	if kind == routeCode || kind == routeMemory || kind == routeEmpty {
		rememberPromptKind(payload, vendor, kind)
	}
	live := ""
	if pending := pendingLiveWork(root); pending != "" && claimHarvestPending(payload, vendor) {
		live = pending
	}
	if kind != routeMemory {
		return live
	}
	if root == "" {
		return live
	}
	if pack := memory.PromptRecallPack(root, prompt); pack != "" {
		return harvest.JoinStart(pack, live)
	}
	if !claimMemoryIndex(payload, vendor) {
		return live
	}
	text := memory.SessionStartIndex(root)
	if text == "" {
		return live
	}
	return harvest.JoinStart(text, live)
}

func pendingLiveWork(root string) string {
	if root == "" {
		return ""
	}
	if line := harvest.PendingSessionStartLine(root); line != "" {
		return line
	}
	return memory.PendingDistillLine(root)
}

func subagentSteer(payload []byte, vendor, ev string) (steerDecision, bool) {
	if !claimSubagentSteer(payload, vendor) {
		return steerDecision{}, false
	}
	switch promptRoute(payload, vendor) {
	case routeMemory:
		return steerDecision{text: steer.MemoryHookReminder(), hookEvent: ev}, true
	case routeEmpty:
		return steerDecision{}, false
	default:
		return steerDecision{text: steer.HookReminder(), hookEvent: ev}, true
	}
}

func claimGraphNudge(payload []byte, vendor string) bool {
	return claimSessionFlag(payload, vendor, func(s *sessionstate.State) *bool {
		return &s.GraphSteerReminded
	}, "last_graph_nudge")
}

func claimMemoryNudge(payload []byte, vendor string) bool {
	return claimSessionFlag(payload, vendor, func(s *sessionstate.State) *bool {
		return &s.MemorySteerReminded
	}, "last_memory_nudge")
}

func claimMemoryIndex(payload []byte, vendor string) bool {
	return claimSessionFlag(payload, vendor, func(s *sessionstate.State) *bool {
		return &s.MemoryIndexInjected
	}, "last_memory_index")
}

func claimHarvestPending(payload []byte, vendor string) bool {
	return claimSessionFlag(payload, vendor, func(s *sessionstate.State) *bool {
		return &s.HarvestPendingInjected
	}, "last_harvest_pending")
}

func claimSubagentSteer(payload []byte, vendor string) bool {
	return claimSessionFlag(payload, vendor, func(s *sessionstate.State) *bool {
		return &s.SubagentSteerReminded
	}, "last_subagent_steer")
}

func claimSnippetOverflow(payload []byte, vendor string) bool {
	return claimSessionFlag(payload, vendor, func(s *sessionstate.State) *bool {
		return &s.SnippetOverflowReminded
	}, "last_snippet_overflow")
}

func claimQueryRepeat(payload []byte, vendor string) bool {
	return claimSessionFlag(payload, vendor, func(s *sessionstate.State) *bool {
		return &s.QueryRepeatReminded
	}, "last_query_repeat")
}

// claimSessionFlag is once-per-session when the host sent an id. Missing ids
// used to fail open and spam every tool call; fall back to a repo-scoped
// stamp under .so/db/ (same layout QueryStampFresh uses, including Docker /work).
func claimSessionFlag(payload []byte, vendor string, flag func(*sessionstate.State) *bool, stampName string) bool {
	sessionID := steerSessionID(payload)
	if sessionID != "" {
		state := sessionstate.Load(sessionID, vendor)
		f := flag(state)
		if *f {
			return false
		}
		*f = true
		sessionstate.Save(sessionID, vendor, state)
		return true
	}
	return claimRootStamp(payload, stampName)
}

func claimRootStamp(payload []byte, name string) bool {
	root := stampRoot(payload)
	if root == "" {
		return false
	}
	path := filepath.Join(paths.Resolve(root).DBDir, name)
	if engine.QueryStampFreshAt(path) {
		return false
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false
	}
	_ = os.WriteFile(path, []byte(fmt.Sprintf("%d\n", time.Now().Unix())), 0o644)
	return true
}

// stampRoot prefers a graph database root, then any managed repo root, so
// QueryStampFresh and nudge stamps still bind when cwd is Docker /work.
func stampRoot(payload []byte) string {
	if root := graphRoot(payload); root != "" {
		return root
	}
	return repoRoot(payload)
}

func isSessionStartEvent(vendor, ev string) bool {
	switch vendor {
	case "claude-code", "codex":
		return ev == "SessionStart"
	case "cursor":
		return ev == "sessionStart"
	case "gemini":
		return strings.ToLower(ev) == "sessionstart"
	case "copilot-cli":
		return ev == "sessionStart"
	case "opencode":
		lower := strings.ToLower(ev)
		return strings.Contains(lower, "session.created") || lower == "sessionstart" || lower == "session_start"
	case "pi":
		lower := strings.ToLower(ev)
		return lower == "session_start" || lower == "sessionstart"
	default:
		return false
	}
}

// managedFromPayload is true when the hook workspace already has .so/.
func managedFromPayload(payload []byte) bool {
	start := strings.TrimSpace(peekContext(payload).CWD)
	if start == "" {
		wd, err := os.Getwd()
		if err != nil {
			return false
		}
		start = wd
	}
	root, err := paths.FindRoot(start)
	if err != nil || root == "" {
		root = start
	}
	return paths.Managed(root)
}

// graphRoot resolves the repository root for the tool call, and reports ""
// when that repository has no Superopen graph to draw on.
func graphRoot(payload []byte) string {
	start := strings.TrimSpace(peekContext(payload).CWD)
	if start == "" {
		wd, err := os.Getwd()
		if err != nil {
			return ""
		}
		start = wd
	}
	root, err := paths.FindRoot(start)
	if err != nil || root == "" {
		return ""
	}
	if _, err := os.Stat(paths.Resolve(root).Database); err != nil {
		return ""
	}
	return root
}

// steerSessionID mirrors run()'s rollup key so the steer budget is tracked
// against the same cache entry the rest of the hook writes.
func steerSessionID(payload []byte) string {
	probe := peekContext(payload)
	if probe.ConversationID != "" {
		return probe.ConversationID
	}
	return probe.SessionID
}

func toolNameFromPayload(payload []byte) string {
	var m map[string]any
	if json.Unmarshal(payload, &m) != nil {
		return ""
	}
	for _, key := range []string{"tool_name", "toolName", "name", "tool"} {
		if v, ok := m[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// isExploreTool matches the discovery tools whose input names a symbol or file
// the graph can answer for. Edit/Write stay out so the gate does not fire on
// the mutation path.
func isExploreTool(name string) bool {
	return isSearchTool(name) || isReadTool(name)
}

func isSearchTool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "grep", "glob", "bash", "shell", "search", "searchfiles", "semanticsearch",
		"codebase_search", "ripgrep":
		return true
	default:
		return false
	}
}

func isReadTool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "read", "readfile", "read_file", "glob":
		return true
	default:
		return false
	}
}

// termPattern captures identifier-shaped runs, so a regex like `.*Handler.*`
// yields `Handler` rather than a query the index cannot rank.
var termPattern = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_.]*`)

// searchTermFromPayload derives a searchable symbol from a discovery tool's
// input: the pattern for Grep/Glob, or the file stem for Read.
func searchTermFromPayload(payload []byte) string {
	var probe map[string]any
	if json.Unmarshal(payload, &probe) != nil {
		return ""
	}
	input, _ := probe["tool_input"].(map[string]any)
	if input == nil {
		input, _ = probe["toolInput"].(map[string]any)
	}
	if input == nil {
		input, _ = probe["args"].(map[string]any)
	}
	if input == nil {
		input = probe
	}

	for _, key := range []string{"pattern", "query", "regex", "search"} {
		if raw, ok := input[key].(string); ok {
			if term := longestTerm(raw); term != "" {
				return term
			}
		}
	}
	for _, key := range []string{"file_path", "filePath", "path", "notebook_path"} {
		if raw, ok := input[key].(string); ok {
			if term := fileStem(raw); term != "" {
				return term
			}
		}
	}
	if cmd, ok := input["command"].(string); ok && bashLooksLikeSearch(cmd) {
		if term := longestTerm(cmd); term != "" {
			return term
		}
	}
	return ""
}

func bashLooksLikeSearch(cmd string) bool {
	lower := strings.ToLower(cmd)
	for _, tok := range []string{"grep", "ripgrep", "rg ", "rg\t", "find ", "fd ", "ack ", "ag "} {
		if strings.Contains(lower, tok) {
			return true
		}
	}
	return false
}

func bashLooksLikeRead(cmd string) bool {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	for _, tok := range []string{"cat ", "cat\t", "sed -n", "head ", "head\t", "tail ", "tail\t", "nl ", "less ", "more ", "awk ", "python -c", "python3 -c"} {
		if strings.Contains(lower, tok) || strings.HasPrefix(lower, strings.TrimSpace(tok)) {
			return true
		}
	}
	return strings.HasPrefix(lower, "cat") && (len(lower) == 3 || lower[3] == ' ' || lower[3] == '\t')
}

func bashLooksLikeListing(cmd string) bool {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	if strings.HasPrefix(lower, "ls") {
		rest := strings.TrimPrefix(lower, "ls")
		return rest == "" || rest[0] == ' ' || rest[0] == '\t' || rest[0] == '-'
	}
	return false
}

func bashLooksLikeGraphQuery(cmd string) bool {
	return strings.Contains(strings.ToLower(cmd), "graph query")
}

// longestTerm picks the most selective identifier in a raw pattern.
func longestTerm(raw string) string {
	best := ""
	for _, candidate := range termPattern.FindAllString(raw, -1) {
		candidate = strings.Trim(candidate, ".")
		if len(candidate) < augmentMinTermLen {
			continue
		}
		if len(candidate) > len(best) {
			best = candidate
		}
	}
	return best
}

// fileStem reduces a path to its base name without extension.
func fileStem(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if i := strings.LastIndexAny(raw, `/\`); i >= 0 {
		raw = raw[i+1:]
	}
	if i := strings.Index(raw, "."); i > 0 {
		raw = raw[:i]
	}
	if len(raw) < augmentMinTermLen {
		return ""
	}
	return raw
}
