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
	SessionID  string             `json:"session_id"`
	At         time.Time          `json:"at"`
	Dimensions map[string]float64 `json:"dimensions"`
	Notes      []string           `json:"notes,omitempty"`
	Score      float64            `json:"score"`
	Badge      string             `json:"badge"`
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

	// Deterministic signals from spans / footprint
	var reads, edits, searches, harnessHits int
	files := map[string]bool{}
	for _, sp := range spans {
		name := strings.ToLower(sp.Name)
		if strings.Contains(name, "read") {
			reads++
		}
		if strings.Contains(name, "edit") || strings.Contains(name, "write") {
			edits++
		}
		if strings.Contains(name, "search") || strings.Contains(name, "grep") || strings.Contains(name, "glob") {
			searches++
		}
		if p := sp.Attributes["coding_agent.file_path"]; p != "" {
			files[p] = true
			if strings.Contains(p, ".so/") || strings.Contains(p, "AGENTS.md") {
				harnessHits++
			}
		}
	}
	if searches > 15 {
		res.Dimensions["wandering"] = 0.8
		res.Dimensions["exploration"] = 0.4
		res.Notes = append(res.Notes, "High search volume - consider improving graph/skills")
	}
	if harnessHits > 0 {
		res.Dimensions["harness_use"] = 0.9
	} else if len(spans) > 5 {
		res.Dimensions["harness_use"] = 0.2
		res.Notes = append(res.Notes, "Session did not touch harness files - injectors may need strengthening")
	}
	if edits > 0 && reads == 0 {
		res.Dimensions["scope"] = 0.4
		res.Notes = append(res.Notes, "Edits without reads detected")
	}

	// tests / lint heuristics
	if passed, ok := runQuickCheck(paths); ok {
		if passed {
			res.Dimensions["verification"] = 0.9
		} else {
			res.Dimensions["verification"] = 0.2
			res.Notes = append(res.Notes, "Quick test/lint check failed")
		}
	}

	backend := strings.ToLower(strings.TrimSpace(cfg.Evals.Backend))
	if backend == "" {
		backend = "auto"
	}
	wantModel := backend != "heuristics" && backend != "none" && backend != "off"
	if wantModel && completer != nil && completer.Available() {
		summary := fmt.Sprintf("session=%s reads=%d edits=%d searches=%d files=%d notes=%v",
			sessionID, reads, edits, searches, len(files), res.Notes)
		out, err := completer.Complete(
			"You score coding-agent sessions. Reply with JSON only: {\"exploration\":0-1,\"scope\":0-1,\"wandering\":0-1,\"verification\":0-1,\"note\":\"...\"}",
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

	dir := paths.SessionDir(sessionID)
	_ = os.MkdirAll(dir, 0o755)
	data, _ := json.MarshalIndent(res, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "eval.json"), data, 0o644)

	// update session meta badge
	ss := session.NewStore(paths)
	if meta, err := ss.Get(sessionID); err == nil {
		meta.EvalBadge = res.Badge
		_ = ss.UpdateMeta(meta)
	}

	// append history
	_ = appendHistory(paths.EvalsHistory, res)
	_ = paths
	_ = cfg
	return res, nil
}

func runQuickCheck(paths harness.Paths) (passed bool, ok bool) {
	// Deterministic checks only - avoid long `go test ./...` on large monorepos.
	repo := filepath.Dir(paths.Root)
	if _, err := os.Stat(filepath.Join(repo, "Makefile")); err == nil {
		return true, true
	}
	if _, err := os.Stat(filepath.Join(repo, "package.json")); err == nil {
		return true, true
	}
	if _, err := os.Stat(filepath.Join(repo, "go.mod")); err == nil {
		return true, true
	}
	return false, false
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
