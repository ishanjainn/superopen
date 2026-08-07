package recommend

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/eval"
	"github.com/ishanjainn/superopen/internal/guardrails"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/harnessvalid"
	"github.com/ishanjainn/superopen/internal/memory"
	"gopkg.in/yaml.v3"
)

type Recommendation struct {
	ID              string    `json:"id"`
	Fingerprint     string    `json:"fingerprint,omitempty"`
	SessionID       string    `json:"session_id,omitempty"`
	Type            string    `json:"type"` // skill | guardrail | docs | eval | graph
	Title           string    `json:"title"`
	Rationale       string    `json:"rationale"`
	Why             string    `json:"why,omitempty"`
	RelatedSessions []string  `json:"related_sessions,omitempty"`
	Evidence        []string  `json:"evidence,omitempty"`
	ProposedPath    string    `json:"proposed_path,omitempty"`
	ProposedBody    string    `json:"proposed_body,omitempty"`
	Status          string    `json:"status"` // pending | applied | dismissed | reverted | invalid | stale
	CreatedAt       time.Time `json:"created_at"`
	// Snapshot fields for revert
	PreviousBody    string    `json:"previous_body,omitempty"`
	PreviousExisted bool      `json:"previous_existed,omitempty"`
	AppliedAt       time.Time `json:"applied_at,omitempty"`
	AppliedPaths    []string  `json:"applied_paths,omitempty"`
	LessonText      string    `json:"lesson_text,omitempty"`
}

// FingerprintKey builds a stable id across sessions for dedupe.
func FingerprintKey(recType, proposedPath, kind string) string {
	recType = strings.ToLower(strings.TrimSpace(recType))
	p := filepath.Clean(proposedPath)
	p = strings.ReplaceAll(p, "\\", "/")
	if i := strings.LastIndex(p, "/.so/"); i >= 0 {
		p = p[i+5:] // strip up to .so/
	} else if i := strings.LastIndex(p, "/skills/"); i >= 0 {
		p = "skills/" + filepath.Base(p)
	} else if i := strings.LastIndex(p, "/knowledge/"); i >= 0 {
		p = "knowledge/" + filepath.Base(p)
	} else if strings.HasSuffix(p, "guardrails.yaml") {
		p = "guardrails/guardrails.yaml"
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind != "" {
		return recType + "|" + p + "|" + kind
	}
	return recType + "|" + p
}

func Generate(paths harness.Paths, sessionID string, evalRes eval.Result, _ interface{}) ([]Recommendation, error) {
	var draft []Recommendation
	now := time.Now().UTC()

	if evalRes.Dimensions["wandering"] > 0.6 {
		draft = append(draft, Recommendation{
			ID: fmt.Sprintf("rec_%d_graph", now.UnixNano()), SessionID: sessionID,
			Type: "docs", Title: "Improve architecture map for hot paths",
			Rationale: "High wandering - agent searched excessively.",
			Why: "Enriching .so/knowledge/architecture.md (or rebuilding the semantic graph) gives the next session clearer boundaries so it spends fewer tokens rediscovering structure.",
			RelatedSessions: nonEmpty(sessionID),
			Evidence: []string{
				fmt.Sprintf("wandering score %.2f", evalRes.Dimensions["wandering"]),
			},
			ProposedPath: filepath.Join(paths.KnowledgeDir, "architecture.md"),
			ProposedBody: "## Hot paths\n\n- Document primary services and entrypoints discovered this session.\n",
			Status: "pending", CreatedAt: now,
			Fingerprint: FingerprintKey("docs", filepath.Join(paths.KnowledgeDir, "architecture.md"), "architecture"),
		})
	}
	if evalRes.Dimensions["harness_use"] < 0.4 {
		draft = append(draft, Recommendation{
			ID: fmt.Sprintf("rec_%d_skill", now.UnixNano()), SessionID: sessionID,
			Type: "skill", Title: "Add skill for recurring exploration pattern",
			Rationale: "Harness underused - agent skipped .so skills/context.",
			Why: "A focused skill that points at graph query + context before Grep/Glob reduces token waste on the next similar task.",
			RelatedSessions: nonEmpty(sessionID),
			Evidence: []string{
				fmt.Sprintf("harness_use score %.2f", evalRes.Dimensions["harness_use"]),
			},
			ProposedPath: filepath.Join(paths.SkillsDir, "reduce-exploration.md"),
			ProposedBody: "# Reduce exploration\n\n1. Run `so graph query` before Grep/Glob.\n2. Read `.so/knowledge/architecture.md` for service boundaries.\n3. Prefer skills over broad searches.\n",
			Status: "pending", CreatedAt: now,
			Fingerprint: FingerprintKey("skill", filepath.Join(paths.SkillsDir, "reduce-exploration.md"), "reduce-exploration"),
		})
	}
	if evalRes.Dimensions["scope"] < 0.5 {
		guardBody := advisoryGuardrailBody("avoid-unrelated-drift", "Keep edits scoped to the requested task; avoid unrelated drive-by refactors.")
		draft = append(draft, Recommendation{
			ID: fmt.Sprintf("rec_%d_guard", now.UnixNano()), SessionID: sessionID,
			Type: "guardrail", Title: "Tighten unrelated-change guardrail",
			Rationale: "Low scope score - edits drifted outside the task.",
			Why: "Reinforcing the avoid-unrelated guardrail keeps the next session's diffs smaller and reviewable.",
			RelatedSessions: nonEmpty(sessionID),
			Evidence: []string{
				fmt.Sprintf("scope score %.2f", evalRes.Dimensions["scope"]),
			},
			ProposedPath: paths.GuardrailsFile,
			ProposedBody: guardBody,
			Status: "pending", CreatedAt: now,
			Fingerprint: FingerprintKey("guardrail", paths.GuardrailsFile, "avoid-unrelated"),
		})
	}

	// No title-rewrite agent calls (token budget).
	return MergePending(paths, draft)
}

func advisoryGuardrailBody(id, desc string) string {
	f := guardrails.File{
		Rules: []guardrails.Rule{{
			ID: id, Description: desc, Severity: "warn", Source: "recommend",
		}},
	}
	data, _ := yaml.Marshal(f)
	return string(data)
}

func LoadPending(paths harness.Paths) ([]Recommendation, error) {
	data, err := os.ReadFile(paths.PendingRecs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var recs []Recommendation
	return recs, json.Unmarshal(data, &recs)
}

func SavePending(paths harness.Paths, recs []Recommendation) error {
	_ = os.MkdirAll(filepath.Dir(paths.PendingRecs), 0o755)
	data, err := json.MarshalIndent(recs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(paths.PendingRecs, data, 0o644)
}

// EnqueuePending merges a single recommendation via fingerprint rules.
func EnqueuePending(paths harness.Paths, r Recommendation) ([]Recommendation, error) {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	if r.ID == "" {
		r.ID = fmt.Sprintf("rec_%d", time.Now().UTC().UnixNano())
	}
	if r.Fingerprint == "" {
		r.Fingerprint = FingerprintKey(r.Type, r.ProposedPath, "")
	}
	if r.Status == "" {
		r.Status = "pending"
	}
	return MergePending(paths, []Recommendation{r})
}

// MergePending upserts by fingerprint: merge open, suppress applied/dismissed.
func MergePending(paths harness.Paths, draft []Recommendation) ([]Recommendation, error) {
	pending, _ := LoadPending(paths)
	hist, _ := LoadHistory(paths)

	fpPending := map[string]int{}
	for i, r := range pending {
		fp := r.Fingerprint
		if fp == "" {
			fp = FingerprintKey(r.Type, r.ProposedPath, "")
			pending[i].Fingerprint = fp
		}
		fpPending[fp] = i
	}
	blocked := map[string]bool{}
	for _, r := range hist {
		fp := r.Fingerprint
		if fp == "" {
			fp = FingerprintKey(r.Type, r.ProposedPath, "")
		}
		if r.Status == "applied" || r.Status == "dismissed" {
			blocked[fp] = true
		}
		if r.Status == "reverted" {
			delete(blocked, fp)
		}
	}

	var upserted []Recommendation
	for _, r := range draft {
		if r.Fingerprint == "" {
			r.Fingerprint = FingerprintKey(r.Type, r.ProposedPath, "")
		}
		if err := harnessvalid.Applyable(r.Type, r.ProposedPath, r.ProposedBody, r.Evidence); err != nil {
			// Never enqueue non-applyable drafts - they clog HITL with dead rows.
			continue
		}
		if blocked[r.Fingerprint] {
			continue
		}
		if i, ok := fpPending[r.Fingerprint]; ok {
			prev := pending[i]
			r.ID = prev.ID
			r.CreatedAt = prev.CreatedAt
			r.RelatedSessions = mergeSessions(prev.RelatedSessions, r.RelatedSessions)
			if r.Evidence == nil {
				r.Evidence = prev.Evidence
			}
			pending[i] = r
			upserted = append(upserted, r)
			continue
		}
		pending = append(pending, r)
		fpPending[r.Fingerprint] = len(pending) - 1
		upserted = append(upserted, r)
	}
	if err := SavePending(paths, pending); err != nil {
		return upserted, err
	}
	return upserted, nil
}

func mergeSessions(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range append(a, b...) {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func Apply(paths harness.Paths, id string) error {
	pending, err := LoadPending(paths)
	if err != nil {
		return err
	}
	var kept []Recommendation
	var applied *Recommendation
	for i := range pending {
		if pending[i].ID == id {
			r := pending[i]
			applied = &r
			continue
		}
		kept = append(kept, pending[i])
	}
	if applied == nil {
		return fmt.Errorf("recommendation %s not found", id)
	}
	if err := harnessvalid.Applyable(applied.Type, applied.ProposedPath, applied.ProposedBody, applied.Evidence); err != nil {
		return fmt.Errorf("not applyable: %w", err)
	}

	var prevBody []byte
	prevExisted := false
	if applied.ProposedPath != "" {
		if data, err := os.ReadFile(applied.ProposedPath); err == nil {
			prevBody = data
			prevExisted = true
		}
	}

	switch strings.ToLower(applied.Type) {
	case "guardrail":
		if err := mergeGuardrails(paths, applied.ProposedBody); err != nil {
			return err
		}
	default:
		if applied.ProposedPath != "" && applied.ProposedBody != "" {
			if err := os.MkdirAll(filepath.Dir(applied.ProposedPath), 0o755); err != nil {
				return err
			}
			// skills: create-only if exists with content → append for docs
			if strings.ToLower(applied.Type) == "skill" {
				if info, err := os.Stat(applied.ProposedPath); err == nil && info.Size() > 0 {
					// keep existing; still mark applied with lesson
				} else if err := os.WriteFile(applied.ProposedPath, []byte(applied.ProposedBody), 0o644); err != nil {
					return err
				}
			} else if strings.ToLower(applied.Type) == "docs" {
				f, err := os.OpenFile(applied.ProposedPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
				if err != nil {
					return err
				}
				_, _ = f.WriteString("\n\n" + strings.TrimSpace(applied.ProposedBody) + "\n")
				_ = f.Close()
			} else {
				if err := os.WriteFile(applied.ProposedPath, []byte(applied.ProposedBody), 0o644); err != nil {
					return err
				}
			}
		}
	}

	lesson := applied.Title + " - " + applied.Rationale
	store := memory.NewStore(paths)
	_ = store.AddLesson(memory.Lesson{
		Text: lesson, Scope: "workspace", Confidence: 0.9, SourceSession: applied.SessionID,
	}, memory.ModePersistent)

	applied.Status = "applied"
	applied.AppliedAt = time.Now().UTC()
	applied.PreviousBody = string(prevBody)
	applied.PreviousExisted = prevExisted
	applied.LessonText = lesson
	if applied.ProposedPath != "" {
		applied.AppliedPaths = []string{applied.ProposedPath}
	}

	hist := []Recommendation{}
	if data, err := os.ReadFile(paths.RecsHistory); err == nil {
		_ = json.Unmarshal(data, &hist)
	}
	hist = append(hist, *applied)
	hdata, _ := json.MarshalIndent(hist, "", "  ")
	_ = os.WriteFile(paths.RecsHistory, hdata, 0o644)
	return SavePending(paths, kept)
}

func mergeGuardrails(paths harness.Paths, body string) error {
	incoming, err := harnessvalid.ValidateGuardrailsBody(body)
	if err != nil {
		return err
	}
	eng, _ := guardrails.Load(paths)
	existing := guardrails.File{
		Rules: eng.Rules, Approval: eng.Policy.Approval,
		DeniedCommands: eng.Policy.DeniedCommands, SensitivePaths: eng.Policy.SensitivePaths,
		RedactOutput: eng.Policy.RedactOutput,
	}
	if data, err := os.ReadFile(paths.GuardrailsFile); err == nil {
		_ = yaml.Unmarshal(data, &existing)
	}
	byID := map[string]bool{}
	for _, r := range existing.Rules {
		byID[r.ID] = true
	}
	for _, r := range incoming.Rules {
		if r.ID == "" || byID[r.ID] {
			continue
		}
		existing.Rules = append(existing.Rules, r)
		byID[r.ID] = true
	}
	if incoming.Approval != "" && existing.Approval == "" {
		existing.Approval = incoming.Approval
	}
	for _, c := range incoming.DeniedCommands {
		if !containsStr(existing.DeniedCommands, c) {
			existing.DeniedCommands = append(existing.DeniedCommands, c)
		}
	}
	for _, p := range incoming.SensitivePaths {
		if !containsStr(existing.SensitivePaths, p) {
			existing.SensitivePaths = append(existing.SensitivePaths, p)
		}
	}
	out, err := yaml.Marshal(existing)
	if err != nil {
		return err
	}
	header := "# Superopen guardrails (advisory rules + enforcement)\n# Edit freely; so sync will not overwrite.\n\n"
	_ = os.MkdirAll(filepath.Dir(paths.GuardrailsFile), 0o755)
	return os.WriteFile(paths.GuardrailsFile, append([]byte(header), out...), 0o644)
}

func containsStr(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}

func Dismiss(paths harness.Paths, id string) error {
	pending, err := LoadPending(paths)
	if err != nil {
		return err
	}
	var kept []Recommendation
	var dismissed *Recommendation
	for i := range pending {
		if pending[i].ID == id {
			r := pending[i]
			r.Status = "dismissed"
			dismissed = &r
			continue
		}
		kept = append(kept, pending[i])
	}
	if dismissed == nil {
		return fmt.Errorf("recommendation %s not found", id)
	}
	hist := []Recommendation{}
	if data, err := os.ReadFile(paths.RecsHistory); err == nil {
		_ = json.Unmarshal(data, &hist)
	}
	hist = append(hist, *dismissed)
	hdata, _ := json.MarshalIndent(hist, "", "  ")
	_ = os.WriteFile(paths.RecsHistory, hdata, 0o644)
	return SavePending(paths, kept)
}

// Revert restores the pre-apply snapshot for an applied recommendation.
func Revert(paths harness.Paths, id string) error {
	hist, err := LoadHistory(paths)
	if err != nil {
		return err
	}
	found := -1
	for i := range hist {
		if hist[i].ID == id && hist[i].Status == "applied" {
			found = i
			break
		}
	}
	if found < 0 {
		return fmt.Errorf("applied recommendation %s not found", id)
	}
	r := hist[found]
	for _, p := range r.AppliedPaths {
		if p == "" {
			continue
		}
		if !r.PreviousExisted {
			_ = os.Remove(p)
			continue
		}
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte(r.PreviousBody), 0o644)
	}
	hist[found].Status = "reverted"
	hdata, _ := json.MarshalIndent(hist, "", "  ")
	return os.WriteFile(paths.RecsHistory, hdata, 0o644)
}

// MarkStaleFlags pending recs whose proposed paths disappeared after pull.
func MarkStaleFlags(paths harness.Paths) error {
	pending, err := LoadPending(paths)
	if err != nil {
		return err
	}
	changed := false
	for i := range pending {
		p := pending[i].ProposedPath
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); os.IsNotExist(err) {
			if pending[i].Status != "stale" {
				pending[i].Status = "stale"
				changed = true
			}
		}
	}
	if !changed {
		return nil
	}
	return SavePending(paths, pending)
}

func LoadHistory(paths harness.Paths) ([]Recommendation, error) {
	data, err := os.ReadFile(paths.RecsHistory)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var recs []Recommendation
	return recs, json.Unmarshal(data, &recs)
}

func nonEmpty(ids ...string) []string {
	var out []string
	for _, id := range ids {
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}
