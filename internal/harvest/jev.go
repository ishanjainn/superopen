package harvest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/session"
	"github.com/ishanjainn/superopen/internal/session/trace"
)

const jevEndpoint = "https://api.typesafe.ai/v1/systemone"

// JevMode is true when Settings turned Jev harvest on.
// Agents are not asked to propose while this is set.
func JevMode() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SUPEROPEN_HARVEST_JEV"))) {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

// JevDecision is one evaluation of a finished session.
type JevDecision struct {
	Enabled        bool    `json:"enabled"`
	KeySet         bool    `json:"key_set"`
	SessionID      string  `json:"session_id,omitempty"`
	Task           string  `json:"task,omitempty"`
	Correction     string  `json:"correction,omitempty"`
	Choice         string  `json:"choice,omitempty"`
	Evidence       float64 `json:"evidence,omitempty"`
	CorrectionProb float64 `json:"correction_prob,omitempty"`
	Promote        bool    `json:"promote"`
	MemoryID       int64   `json:"memory_id,omitempty"`
	MemoryTitle    string  `json:"memory_title,omitempty"`
	MemoryText     string  `json:"memory_text,omitempty"`
	Cached         bool    `json:"cached,omitempty"`
	Note           string  `json:"note,omitempty"`
}

type jevStored struct {
	Choice         string  `json:"choice"`
	Evidence       float64 `json:"evidence"`
	CorrectionProb float64 `json:"correction_prob"`
	Promote        bool    `json:"promote"`
	Task           string  `json:"task"`
	Correction     string  `json:"correction"`
	MemoryTitle    string  `json:"memory_title"`
	MemoryText     string  `json:"memory_text"`
	MemoryID       int64   `json:"memory_id,omitempty"`
}

// PromoteGate matches the default Jev policy: promote only when the choice is
// promote, evidence is at least 2.5 of 4, and correction probability is at least 0.80.
func PromoteGate(choice string, evidence, correction float64) bool {
	return strings.EqualFold(strings.TrimSpace(choice), "promote") && evidence >= 2.5 && correction >= 0.80
}

var postJev = func(key, state string) ([]byte, error) {
	body, err := json.Marshal(map[string]any{
		"model": "jev-latest",
		"state": state,
		"questions": map[string]any{
			"decision": map[string]any{
				"type":         "choice",
				"instructions": "Should this session become reusable memory for later agents?",
				"criteria": map[string]string{
					"promote": "A correction or workflow later agents should reuse",
					"discard": "Nothing durable, or the trace is routine",
				},
			},
			"evidence": map[string]any{
				"type":         "score",
				"instructions": "How strong is the evidence in this trace, from 0 (none) to 4 (decisive)?",
				"criteria":     []string{"none", "weak", "partial", "strong"},
			},
			"correction": map[string]any{
				"type":         "noul",
				"instructions": "Does this session contain a human correction worth keeping?",
			},
		},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, jevEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("jev: %s", resp.Status)
	}
	return raw, nil
}

// ParseJevAnswers reads a systemone response into choice, evidence (0-4), and correction probability (0-1).
func ParseJevAnswers(raw []byte) (choice string, evidence, correction float64, err error) {
	var doc map[string]any
	if err = json.Unmarshal(raw, &doc); err != nil {
		return "", 0, 0, err
	}
	answers, _ := doc["answers"].(map[string]any)
	if answers == nil {
		return "", 0, 0, fmt.Errorf("jev: missing answers")
	}
	choice = firstString(answers["decision"], "choice", "key", "winner", "answer", "label")
	if choice == "" {
		return "", 0, 0, fmt.Errorf("jev: missing decision")
	}
	evidence = firstNumber(answers["evidence"], "score", "value")
	if evidence > 0 && evidence <= 1 {
		evidence *= 4
	}
	correction = firstNumber(answers["correction"], "noul", "probability", "score")
	if correction > 1 {
		correction = correction / 4
	}
	return choice, evidence, correction, nil
}

// EvaluateJev scores the latest finished session. A stored decision is reused.
func EvaluateJev(root string) (JevDecision, error) {
	out := JevDecision{Enabled: JevMode(), KeySet: strings.TrimSpace(os.Getenv("TYPESAFE_API_KEY")) != ""}
	if !out.Enabled {
		out.Note = "Jev harvest is off"
		return out, nil
	}
	if !out.KeySet {
		return out, fmt.Errorf("TYPESAFE_API_KEY is not set")
	}
	meta, ok := latestFinished(root)
	if !ok {
		out.Note = "No finished session yet"
		return out, nil
	}
	out.SessionID = meta.ID
	store, err := OpenRoot(root)
	if err != nil {
		return out, err
	}
	defer store.Close()
	if raw, hit := store.JevRun(meta.ID); hit {
		return decisionFromStored(out, raw, true), nil
	}
	cand := candidateFromSession(root, meta)
	out.Task = cand.Task
	out.Correction = cand.Correction
	out.MemoryTitle = cand.Title
	out.MemoryText = cand.Text
	state := "Task: " + cand.Task + "\nCorrection: " + cand.Correction + "\n" + cand.Timeline
	body, err := postJev(strings.TrimSpace(os.Getenv("TYPESAFE_API_KEY")), state)
	if err != nil {
		return out, err
	}
	choice, evidence, corr, err := ParseJevAnswers(body)
	if err != nil {
		return out, err
	}
	out.Choice = choice
	out.Evidence = evidence
	out.CorrectionProb = corr
	out.Promote = PromoteGate(choice, evidence, corr) && strings.TrimSpace(cand.Text) != ""
	stored := jevStored{
		Choice: choice, Evidence: evidence, CorrectionProb: corr, Promote: out.Promote,
		Task: cand.Task, Correction: cand.Correction, MemoryTitle: cand.Title, MemoryText: cand.Text,
	}
	status := StatusSkipped
	if out.Promote {
		ep, err := memory.CaptureRoot(root, memory.CaptureInput{
			SessionID: meta.ID,
			Kind:      memory.KindSession,
			Source:    memory.SourceAgent,
			Title:     cand.Title,
			Text:      cand.Text,
			Horizon:   memory.HorizonMedium,
		})
		if err != nil {
			return out, err
		}
		out.MemoryID = ep.ID
		stored.MemoryID = ep.ID
		status = StatusApplied
	}
	blob, _ := json.Marshal(stored)
	if _, err := store.InsertRun(meta.ID, status, "jev", string(blob)); err != nil {
		return out, err
	}
	return out, nil
}

func decisionFromStored(base JevDecision, raw string, cached bool) JevDecision {
	var stored jevStored
	if json.Unmarshal([]byte(raw), &stored) != nil {
		base.Note = "Stored Jev decision could not be read"
		return base
	}
	base.Cached = cached
	base.Choice = stored.Choice
	base.Evidence = stored.Evidence
	base.CorrectionProb = stored.CorrectionProb
	base.Promote = stored.Promote
	base.Task = stored.Task
	base.Correction = stored.Correction
	base.MemoryTitle = stored.MemoryTitle
	base.MemoryText = stored.MemoryText
	base.MemoryID = stored.MemoryID
	return base
}

type jevCandidate struct {
	Task       string
	Correction string
	Timeline   string
	Title      string
	Text       string
}

func latestFinished(root string) (session.Meta, bool) {
	layout := paths.Resolve(root)
	entries, err := session.NewStore(layout).List()
	if err != nil {
		return session.Meta{}, false
	}
	for _, meta := range entries {
		if meta.Status == session.StatusEnded && meta.ID != "" {
			return meta, true
		}
	}
	return session.Meta{}, false
}

func candidateFromSession(root string, meta session.Meta) jevCandidate {
	task := strings.TrimSpace(meta.PromptPreview)
	if task == "" {
		task = strings.TrimSpace(meta.Title)
	}
	layout := paths.Resolve(root)
	spans, _ := trace.NewLocalJSONL(layout.TracesDir).Query(trace.QueryFilter{SessionID: meta.ID, Limit: 40})
	var prompts []string
	for _, sp := range spans {
		if sp.Attributes == nil {
			continue
		}
		p := strings.TrimSpace(firstPrompt(sp.Attributes))
		if p == "" {
			continue
		}
		if len(prompts) == 0 || prompts[len(prompts)-1] != p {
			prompts = append(prompts, p)
		}
	}
	if task == "" && len(prompts) > 0 {
		task = prompts[0]
	}
	correction := ""
	if len(prompts) > 1 {
		last := prompts[len(prompts)-1]
		if last != task {
			correction = last
		}
	}
	title := task
	text := correction
	if text == "" {
		text = task
	}
	if correction != "" {
		title = correction
	}
	title = clipRunes(title, 80)
	return jevCandidate{
		Task: task, Correction: correction, Timeline: session.Digest(root, meta.ID),
		Title: title, Text: text,
	}
}

func firstPrompt(attrs map[string]string) string {
	for _, key := range []string{"gen_ai.prompt", "gen_ai.content.prompt"} {
		if v := strings.TrimSpace(attrs[key]); v != "" {
			return v
		}
	}
	return ""
}

func clipRunes(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func firstString(v any, keys ...string) string {
	m, _ := v.(map[string]any)
	if m == nil {
		if s, ok := v.(string); ok {
			return s
		}
		return ""
	}
	for _, key := range keys {
		if s, ok := m[key].(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func firstNumber(v any, keys ...string) float64 {
	m, _ := v.(map[string]any)
	if m == nil {
		return asFloat(v)
	}
	for _, key := range keys {
		if n := asFloat(m[key]); n != 0 || m[key] != nil {
			if _, ok := m[key].(float64); ok || m[key] != nil {
				if f := asFloat(m[key]); f != 0 || isNumeric(m[key]) {
					return f
				}
			}
		}
	}
	return 0
}

func isNumeric(v any) bool {
	switch v.(type) {
	case float64, float32, int, int64, json.Number:
		return true
	default:
		return false
	}
}

func asFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}
