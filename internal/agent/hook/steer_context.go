package hook

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/agent/sessionstate"
	"github.com/ishanjainn/superopen/internal/agent/steer"
	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/ishanjainn/superopen/internal/graph/client"
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

// emitSteerContext writes vendor-specific additionalContext when the event
// supports it. Unknown protocols get no stdout (fail-open).
func emitSteerContext(vendor, event string, payload []byte) {
	text, hookEvent, ok := steerTextFor(vendor, event, payload)
	if !ok || text == "" {
		return
	}
	var body any
	switch vendor {
	case "claude-code":
		body = map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName":     hookEvent,
				"additionalContext": text,
			},
		}
	case "cursor":
		body = map[string]any{"additional_context": text}
	case "codex":
		body = map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName":     hookEvent,
				"additionalContext": text,
			},
		}
	case "gemini", "copilot-cli":
		// Best-effort shared shape used by several CLI hosts.
		body = map[string]any{"additionalContext": text}
	default:
		return
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(body); err != nil {
		fmt.Fprintf(os.Stderr, "so hook: steer stdout: %v\n", err)
	}
}

func steerTextFor(vendor, event string, payload []byte) (text, hookEvent string, ok bool) {
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
	default:
		return "", "", false
	}

	switch {
	case compactEvent:
		if text := memoryCompactText(payload); text != "" {
			return text, ev, true
		}
	case sessionEvent:
		var parts []string
		if claimSessionReminder(payload, vendor) {
			parts = append(parts, steer.HookReminder())
		}
		if vendor == "cursor" && (ev == "beforeSubmitPrompt") {
			text := strings.TrimSpace(strings.Join(parts, "\n\n"))
			if text == "" {
				return "", "", false
			}
			return text, ev, true
		}
		if ev == "UserPromptSubmit" && (vendor == "claude-code" || vendor == "codex") {
			if mem := nextTurnPack(payload); mem != "" {
				parts = append(parts, mem)
			}
		} else if mem := claimMemoryPack(payload, vendor); mem != "" {
			parts = append(parts, mem)
		}
		text := strings.TrimSpace(strings.Join(parts, "\n\n"))
		if text == "" {
			return "", "", false
		}
		return text, ev, true
	case toolEvent:
		if augment := exploreAugment(payload, vendor); augment != "" {
			return augment, ev, true
		}
	}
	return "", "", false
}

func claimMemoryPack(payload []byte, vendor string) string {
	sessionID := steerSessionID(payload)
	if sessionID != "" {
		state := sessionstate.Load(sessionID, vendor)
		if state.MemorySteerReminded {
			return ""
		}
		state.MemorySteerReminded = true
		sessionstate.Save(sessionID, vendor, state)
	}
	root := graphRoot(payload)
	if root == "" {
		return ""
	}
	cue := strings.TrimSpace(peekContext(payload).Prompt)
	pack, err := memory.PackForRoot(root, cue, sessionID)
	if err != nil || strings.TrimSpace(pack.Text) == "" {
		return ""
	}
	text := pack.Text
	if pack.AskDistill && sessionID != "" {
		state := sessionstate.Load(sessionID, vendor)
		if !state.MemoryDistillAsked {
			state.MemoryDistillAsked = true
			sessionstate.Save(sessionID, vendor, state)
			if extra := memory.LiveDistillInstruction(pack.PendingSession); extra != "" {
				text = text + "\n" + extra
			}
		}
	}
	return text
}

func nextTurnPack(payload []byte) string {
	root := graphRoot(payload)
	if root == "" {
		return ""
	}
	cue := strings.TrimSpace(peekContext(payload).Prompt)
	pack, err := memory.PackNextForRoot(root, cue, steerSessionID(payload))
	if err != nil || strings.TrimSpace(pack.Text) == "" {
		return ""
	}
	return pack.Text
}

func memoryCompactText(payload []byte) string {
	root := graphRoot(payload)
	if root == "" {
		return ""
	}
	return memory.CompactSnapshot(root, steerSessionID(payload))
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
// the graph can answer for. Shell and directory listings are deliberately
// excluded: their arguments rarely yield a term worth a graph lookup, and
// augmenting every one of them was the bulk of the old nudge's token cost.
func isExploreTool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "grep", "glob", "read", "readfile", "read_file", "search",
		"searchfiles", "semanticsearch", "codebase_search", "ripgrep":
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
	return ""
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
