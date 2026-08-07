package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/llm"
	"github.com/ishanjainn/superopen/internal/session"
	"github.com/ishanjainn/superopen/internal/tracestore"
)

// Result mirrors process dimensions + optional task notes.
type Result struct {
	SessionID      string             `json:"session_id"`
	At             time.Time          `json:"at"`
	Dimensions     map[string]float64 `json:"dimensions"`
	Notes          []string           `json:"notes,omitempty"`
	Score          float64            `json:"score"`
	Badge          string             `json:"badge"`
	EvidenceStatus string             `json:"evidence_status,omitempty"` // sufficient | insufficient
	// HotAreas are repo-relative dirs that dominated file activity (for nested AGENTS.md recs).
	HotAreas []string `json:"hot_areas,omitempty"`
}

type activitySignals struct {
	reads       int
	edits       int
	searches    int
	toolCalls   int
	harnessHits int
	files       map[string]bool
}

func collectActivitySignals(spans []tracestore.Span) activitySignals {
	s := activitySignals{files: map[string]bool{}}
	for _, sp := range spans {
		name := strings.ToLower(sp.Name)
		attrs := sp.Attributes
		toolName := strings.ToLower(attrs["gen_ai.tool.name"] + " " + attrs["coding_agent.tool.name"])
		arguments := strings.ToLower(attrs["gen_ai.tool.call.arguments"] + " " + attrs["coding_agent.tool.arguments"] + " " + attrs["coding_agent.tool.command"])
		completedTool := strings.Contains(name, "tool.call") || strings.Contains(name, "tool.execute")
		if completedTool {
			s.toolCalls++
		}

		readSignal := strings.Contains(name, "read") || containsAny(toolName, "read", "open", "view")
		editSignal := strings.Contains(name, "edit") || strings.Contains(name, "write") || containsAny(toolName, "edit", "write", "apply_patch")
		searchSignal := strings.Contains(name, "search") || strings.Contains(name, "grep") || strings.Contains(name, "glob") || containsAny(toolName, "search", "grep", "glob")
		if completedTool {
			readSignal = readSignal || containsAny(arguments, "sed -n", "cat ", "head ", "tail ", "jq ")
			editSignal = editSignal || containsAny(arguments, "apply_patch", "gofmt -w")
			searchSignal = searchSignal || containsAny(arguments, "rg ", "grep ", "find ")
		}
		if readSignal {
			s.reads++
		}
		if editSignal {
			s.edits++
		}
		if searchSignal {
			s.searches++
		}

		for _, key := range []string{"coding_agent.file_path", "code.file.path"} {
			if p := attrs[key]; p != "" {
				s.files[p] = true
				if isHarnessReference(strings.ToLower(p)) {
					s.harnessHits++
				}
			}
		}
		if completedTool && isHarnessReference(toolName+" "+arguments) {
			s.harnessHits++
		}
	}
	return s
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func isHarnessReference(value string) bool {
	return containsAny(value,
		".so/", "agents.md", "/agents.md", "claude.md",
		".claude/rules", ".cursor/rules", ".agents/rules", ".codex/rules",
		".claude/skills", ".cursor/skills", ".agents/skills", ".pi/skills",
		"so graph", "so query", "so knowledge", "so rules", "so guard", "so skill", "/so",
	)
}

// hotAreasFromFiles returns up to 2 concentrated package dirs from touched files.
func hotAreasFromFiles(files map[string]bool) []string {
	counts := map[string]int{}
	for p := range files {
		area := guidanceArea(p)
		if area == "" {
			continue
		}
		counts[area]++
	}
	type kv struct {
		k string
		v int
	}
	var ranked []kv
	for k, v := range counts {
		ranked = append(ranked, kv{k, v})
	}
	for i := 0; i < len(ranked); i++ {
		for j := i + 1; j < len(ranked); j++ {
			if ranked[j].v > ranked[i].v {
				ranked[i], ranked[j] = ranked[j], ranked[i]
			}
		}
	}
	var out []string
	total := 0
	for _, r := range ranked {
		total += r.v
	}
	for i := 0; i < len(ranked) && i < 2; i++ {
		// Require the area to account for a meaningful share of file touches.
		if total > 0 && float64(ranked[i].v)/float64(total) < 0.35 && ranked[i].v < 3 {
			continue
		}
		out = append(out, ranked[i].k)
	}
	return out
}

func guidanceArea(path string) string {
	p := filepath.ToSlash(path)
	p = strings.TrimPrefix(p, "./")
	// Drop absolute prefix noise by taking from known roots.
	for _, root := range []string{"internal/", "cmd/", "web/", "sdk/", "plugins/", "templates/", "docs/"} {
		if i := strings.Index(p, root); i >= 0 {
			p = p[i:]
			break
		}
	}
	parts := strings.Split(p, "/")
	if len(parts) == 0 || parts[0] == "" || strings.HasPrefix(parts[0], ".") {
		return ""
	}
	switch parts[0] {
	case "internal", "cmd", "web", "sdk", "plugins":
		if len(parts) >= 2 && parts[1] != "" && !strings.Contains(parts[1], ".") {
			return parts[0] + "/" + parts[1]
		}
		return parts[0]
	default:
		return ""
	}
}

func Run(paths harness.Paths, cfg config.Config, sessionID string, spans []tracestore.Span, completer llm.Completer) (Result, error) {
	res := Result{
		SessionID: sessionID,
		At:        time.Now().UTC(),
		Dimensions: map[string]float64{
			"exploration":  0.7,
			"scope":        0.7,
			"wandering":    0.3,
			"verification": 0.5,
			"harness_use":  0.5,
		},
	}

	// Deterministic signals from normalized span names, tool names, and tool
	// arguments. Codex uses generic coding_agent.tool.call span names, so the
	// operation must not be inferred from the span name alone.
	signals := collectActivitySignals(spans)
	if signals.toolCalls == 0 && signals.reads == 0 && signals.edits == 0 && signals.searches == 0 && len(signals.files) == 0 {
		res.Dimensions = map[string]float64{}
		res.Score = 0
		res.Badge = "unknown"
		res.EvidenceStatus = "insufficient"
		res.Notes = append(res.Notes, "Insufficient activity telemetry to score this session.")
		return persistResult(paths, res)
	}
	res.EvidenceStatus = "sufficient"
	res.HotAreas = hotAreasFromFiles(signals.files)
	if signals.searches > 15 {
		res.Dimensions["wandering"] = 0.8
		res.Dimensions["exploration"] = 0.4
		note := fmt.Sprintf("Problem: %d search tools ran with little reuse of existing guidance. Impact: next similar task will re-discover the same paths and burn tokens.", signals.searches)
		if len(res.HotAreas) > 0 {
			note += " Hot area: " + strings.Join(res.HotAreas, ", ") + "."
		}
		res.Notes = append(res.Notes, note)
	}
	if signals.harnessHits > 0 {
		res.Dimensions["harness_use"] = 0.9
		res.Notes = append(res.Notes, "Harness guidance was consulted (AGENTS.md / rules / skills / so graph) — good reuse for the next session.")
	} else if len(spans) > 5 {
		res.Dimensions["harness_use"] = 0.2
		res.Notes = append(res.Notes, "Problem: session never opened AGENTS.md, project rules/skills, or so graph. Impact: agents rediscover structure instead of following durable guidance.")
	}
	if signals.edits > 0 && signals.reads == 0 {
		res.Dimensions["scope"] = 0.4
		res.Notes = append(res.Notes, "Problem: edits landed without prior reads. Impact: higher risk of unrelated churn and harder review.")
	}

	backend := strings.ToLower(strings.TrimSpace(cfg.Evals.Backend))
	if backend == "" {
		backend = "auto"
	}
	wantModel := backend != "heuristics" && backend != "none" && backend != "off"
	if wantModel && completer != nil && completer.Available() {
		summary := fmt.Sprintf("session=%s reads=%d edits=%d searches=%d tool_calls=%d harness_hits=%d files=%d notes=%v",
			sessionID, signals.reads, signals.edits, signals.searches, signals.toolCalls, signals.harnessHits, len(signals.files), res.Notes)
		out, err := completer.Complete(
			"You score coding-agent sessions. Reply with JSON only: {\"exploration\":0-1,\"scope\":0-1,\"wandering\":0-1,\"verification\":0-1,\"note\":\"one sentence: problem + how better AGENTS.md/rules/skills would help next session\"}",
			summary,
		)
		if err == nil {
			res.Notes = append(res.Notes, "model:"+completer.Backend())
			var parsed map[string]any
			if json.Unmarshal([]byte(extractJSON(out)), &parsed) == nil {
				for _, k := range []string{"exploration", "scope", "wandering", "verification"} {
					if v, ok := parsed[k].(float64); ok {
						res.Dimensions[k] = v
					}
				}
				if n, ok := parsed["note"].(string); ok && n != "" {
					res.Notes = append(res.Notes, n)
				}
			}
		} else {
			res.Notes = append(res.Notes, "model enrichment skipped: "+err.Error())
		}
	}

	sum := 0.0
	for k, v := range res.Dimensions {
		if k == "wandering" {
			sum += 1 - v
		} else {
			sum += v
		}
	}
	res.Score = sum / float64(len(res.Dimensions))
	switch {
	case res.Score >= 0.75:
		res.Badge = "good"
	case res.Score >= 0.45:
		res.Badge = "ok"
	default:
		res.Badge = "poor"
	}

	return persistResult(paths, res)
}

func persistResult(paths harness.Paths, res Result) (Result, error) {
	dir := paths.SessionDir(res.SessionID)
	_ = os.MkdirAll(dir, 0o755)
	data, _ := json.MarshalIndent(res, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "eval.json"), data, 0o644)

	// update session meta badge
	ss := session.NewStore(paths)
	if meta, err := ss.Get(res.SessionID); err == nil {
		meta.EvalBadge = res.Badge
		_ = ss.UpdateMeta(meta)
	}

	// append history
	_ = appendHistory(paths.EvalsHistory, res)
	return res, nil
}

// LatestResult returns the materialized latest evaluation for a session.
// Callers use it to avoid re-evaluating a closed chat whose final result is
// already newer than the session's ended_at timestamp.
func LatestResult(paths harness.Paths, sessionID string) (Result, bool) {
	data, err := os.ReadFile(filepath.Join(paths.SessionDir(sessionID), "eval.json"))
	if err != nil {
		return Result{}, false
	}
	var res Result
	if json.Unmarshal(data, &res) != nil || res.SessionID == "" || res.At.IsZero() {
		return Result{}, false
	}
	return res, true
}

func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}

func appendHistory(path string, res Result) error {
	var hist []Result
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &hist)
	}
	hist = append(hist, res)
	data, err := json.MarshalIndent(hist, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
