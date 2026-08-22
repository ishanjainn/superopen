package hook

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/agent/sessionstate"
	"github.com/ishanjainn/superopen/internal/agent/steer"
	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/ishanjainn/superopen/internal/graph/client"
	"github.com/ishanjainn/superopen/internal/graph/engine"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/paths"
)

const (
	// augmentHitLimit caps how many indexed symbols one augment carries.
	augmentHitLimit = 5
	// augmentSessionCap caps how many explore-tool augments a session
	// receives. Past this the agent has either adopted the graph or
	// decided not to, and repeating only spends the user's tokens.
	augmentSessionCap = 3
	// augmentTimeout bounds the embedded graph lookup so a cold or
	// locked database can never stall the host's tool call.
	augmentTimeout = 1500 * time.Millisecond
	// augmentMinTermLen skips terms too short to rank meaningfully.
	augmentMinTermLen = 3
)

var priorWorkCue = regexp.MustCompile(`(?i)last time|we decided|remember|what did we`)

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
			// Explore subagents never see SessionStart; inject a one-line reminder.
			return steerDecision{text: steer.HookReminder(), hookEvent: ev}, true
		}
		if isSessionStartEvent(vendor, ev) {
			if text := memorySessionIndexText(payload); text != "" {
				return steerDecision{text: text, hookEvent: ev}, true
			}
			return steerDecision{}, false
		}
		if isPromptSubmitEvent(vendor, ev) {
			prompt := peekContext(payload).Prompt
			if priorWorkCue.MatchString(prompt) {
				if text := memorySessionIndexText(payload); text != "" {
					return steerDecision{text: text, hookEvent: ev}, true
				}
			}
			return steerDecision{}, false
		}
		// Stop, SessionEnd, and other lifecycle events are observability-only.
		return steerDecision{}, false
	case toolEvent:
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
	gate := strings.ToLower(strings.TrimSpace(kind))
	if gate == "" {
		switch {
		case hookEvent == "beforeReadFile" || isReadTool(tool):
			gate = "read"
		case isSearchTool(tool):
			gate = "search"
		default:
			return steerDecision{}, false
		}
	} else if gate == "search" && tool != "" && !isSearchTool(tool) && !isReadTool(tool) {
		// Scoped matcher already filtered Claude; Cursor preToolUse is unscoped.
		return steerDecision{}, false
	} else if gate == "read" && tool != "" && !isReadTool(tool) && hookEvent != "beforeReadFile" {
		return steerDecision{}, false
	}
	switch gate {
	case "search":
		// Static MANDATORY one-liner only. Do not append ExploreAugment hit lists.
		return steerDecision{text: steer.SearchNudge(), hookEvent: hookEvent}, true
	case "read":
		if isSkillDocRead(peekContext(payload).ToolPath) {
			return steerDecision{}, false
		}
		if hookStrictEnabled() && claimStrictReadDeny(payload, vendor) && isSourceRead(payload, tool, hookEvent) {
			return steerDecision{text: steer.ReadDenyReason(), hookEvent: hookEvent, deny: true}, true
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
	if engine.QueryStampFresh(graphRoot(payload)) {
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
	switch vendor {
	case "claude-code", "codex":
		return ev == "UserPromptSubmit"
	case "cursor":
		return ev == "beforeSubmitPrompt"
	case "copilot-cli":
		return ev == "userPromptSubmitted"
	default:
		return strings.EqualFold(ev, "UserPromptSubmit") || strings.EqualFold(ev, "beforeSubmitPrompt")
	}
}

func memoryCompactText(payload []byte) string {
	root := graphRoot(payload)
	if root == "" {
		return ""
	}
	return memory.CompactSnapshot(root, steerSessionID(payload))
}

func memorySessionIndexText(payload []byte) string {
	root := graphRoot(payload)
	if root == "" {
		return ""
	}
	text := memory.SessionStartIndex(root)
	if strings.TrimSpace(text) == "" {
		return ""
	}
	return text
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

// claimSessionReminder reports whether this session still owes the durable
// graph-first reminder. Hosts fire session-level events once per prompt, so
// repeating the same sentence every turn is pure context tax.
func claimSessionReminder(payload []byte, vendor string) bool {
	sessionID := steerSessionID(payload)
	if sessionID == "" {
		return true
	}
	state := sessionstate.Load(sessionID, vendor)
	if state.GraphSteerReminded {
		return false
	}
	state.GraphSteerReminded = true
	sessionstate.Save(sessionID, vendor, state)
	return true
}

// exploreAugment turns an imminent Grep/Glob/Read into graph context: it looks
// the term up in the embedded graph and returns compact matches, or "" when
// the tool carries no usable term, the repository has no graph, the term is
// unindexed, or this session already spent its augment budget.
func exploreAugment(payload []byte, vendor string) string {
	tool := toolNameFromPayload(payload)
	if !isExploreTool(tool) {
		return ""
	}
	term := searchTermFromPayload(payload)
	if term == "" {
		return ""
	}
	root := graphRoot(payload)
	if root == "" {
		return ""
	}

	sessionID := steerSessionID(payload)
	var state *sessionstate.State
	if sessionID != "" {
		state = sessionstate.Load(sessionID, vendor)
		if state.GraphSteerCount >= augmentSessionCap {
			return ""
		}
		for _, seen := range state.GraphSteerTerms {
			if strings.EqualFold(seen, term) {
				return ""
			}
		}
	}

	total, hits := searchGraphForTerm(root, term)
	if len(hits) == 0 {
		return ""
	}
	text := steer.ExploreAugment(term, total, hits)
	if text == "" {
		return ""
	}
	if state != nil {
		state.GraphSteerCount++
		state.GraphSteerTerms = append(state.GraphSteerTerms, term)
		sessionstate.Save(sessionID, vendor, state)
	}
	return text
}

// searchGraphForTerm runs a bounded search against the embedded engine.
// Every failure path returns no hits so the hook stays silent.
func searchGraphForTerm(root, term string) (int, []steer.GraphHit) {
	graphClient, err := client.Resolve()
	if err != nil {
		return 0, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), augmentTimeout)
	defer cancel()

	var raw json.RawMessage
	request := api.SearchRequest{RepoRoot: root, Query: term, Limit: augmentHitLimit}
	if err := graphClient.Call(ctx, api.OpSearch, request, &raw); err != nil {
		return 0, nil
	}
	var result api.SearchResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return 0, nil
	}
	matches := result.Matches
	if len(matches) == 0 {
		matches = result.Semantic
	}
	if len(matches) > augmentHitLimit {
		matches = matches[:augmentHitLimit]
	}
	hits := make([]steer.GraphHit, 0, len(matches))
	for _, match := range matches {
		hits = append(hits, steer.GraphHit{
			QualifiedName: match.QualifiedName,
			Label:         match.Label,
			File:          match.Location.File,
			Lines:         lineSpan(match.Location.StartLine, match.Location.EndLine),
		})
	}
	total := result.Page.Total
	if total == 0 {
		total = len(hits)
	}
	return total, hits
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

func lineSpan(start, end int) string {
	if start <= 0 {
		return "-"
	}
	if end <= 0 || end == start {
		return fmt.Sprintf("%d", start)
	}
	return fmt.Sprintf("%d-%d", start, end)
}
