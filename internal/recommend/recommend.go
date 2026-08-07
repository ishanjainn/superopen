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
	"github.com/ishanjainn/superopen/internal/nativedocs"
	"github.com/ishanjainn/superopen/internal/session"
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
	DecisionReason  string    `json:"decision_reason,omitempty"`
	DecisionActor   string    `json:"decision_actor,omitempty"` // human | agent | system
	DecisionAt      time.Time `json:"decision_at,omitempty"`
}

type Decision struct {
	Reason string
	Actor  string
}

func normalizeDecision(d Decision) (Decision, error) {
	d.Reason = strings.TrimSpace(d.Reason)
	d.Actor = strings.ToLower(strings.TrimSpace(d.Actor))
	if d.Reason == "" {
		return d, fmt.Errorf("decision reason is required")
	}
	if d.Actor == "" {
		d.Actor = "agent"
	}
	if d.Actor != "human" && d.Actor != "agent" && d.Actor != "system" {
		return d, fmt.Errorf("decision actor must be human, agent, or system")
	}
	return d, nil
}

// FingerprintKey builds a stable id across sessions for dedupe.
func FingerprintKey(recType, proposedPath, kind string) string {
	recType = strings.ToLower(strings.TrimSpace(recType))
	p := filepath.Clean(proposedPath)
	p = strings.ReplaceAll(p, "\\", "/")
	if i := strings.LastIndex(p, "/.so/"); i >= 0 {
		p = p[i+5:] // strip up to .so/
	} else if strings.Contains(p, "/skills/") {
		base := filepath.Base(p)
		if base == "SKILL.md" {
			p = "skills/" + filepath.Base(filepath.Dir(p))
		} else {
			p = "skills/" + base
		}
	} else if strings.HasSuffix(p, "/AGENTS.md") || filepath.Base(p) == "AGENTS.md" {
		// Nested vs root distinguished via Fingerprint kind (area:… vs architecture).
		p = "AGENTS.md"
	} else if strings.Contains(p, "/rules/") || strings.Contains(p, "/instructions/") {
		p = "rules/" + filepath.Base(p)
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
	if evalRes.EvidenceStatus == "insufficient" {
		return nil, nil
	}
	var draft []Recommendation
	now := time.Now().UTC()

	vendor := ""
	if meta, err := session.NewStore(paths).Get(sessionID); err == nil {
		vendor = meta.Vendor
	}
	skillPath := paths.SkillSKILL("reduce-exploration")
	if vendor != "" {
		skillPath = filepath.Join(harness.SkillsDirForVendor(paths.RepoRoot, vendor), "reduce-exploration", "SKILL.md")
	}
	vendorLabel := harness.NormalizeVendorKind(vendor)
	if vendorLabel == "" {
		vendorLabel = "project"
	}

	if evalRes.Dimensions["wandering"] > 0.6 {
		hot := ""
		if len(evalRes.HotAreas) > 0 {
			hot = evalRes.HotAreas[0]
		}
		if hot != "" {
			nestedPath := filepath.Join(paths.RepoRoot, filepath.FromSlash(hot), "AGENTS.md")
			draft = append(draft, Recommendation{
				ID: fmt.Sprintf("rec_%d_area_agents", now.UnixNano()), SessionID: sessionID,
				Type:  "docs",
				Title: "Add " + hot + "/AGENTS.md for this hot path",
				Rationale: "Eval wandering is high and file activity concentrated under " + hot + ".",
				Why: "Why: the agent searched broadly while mostly touching " + hot + ".\n" +
					"Change: create a focused " + hot + "/AGENTS.md with entrypoints, invariants, and where to look first.\n" +
					"How it helps: the next session in that package reads local guidance instead of re-grepping the whole repo, cutting tokens and wrong-file edits.",
				RelatedSessions: nonEmpty(sessionID),
				Evidence: []string{
					fmt.Sprintf("wandering score %.2f", evalRes.Dimensions["wandering"]),
					"hot_area=" + hot,
				},
				ProposedPath: nestedPath,
				ProposedBody: nestedAgentsBody(hot),
				Status:       "pending", CreatedAt: now,
				Fingerprint: FingerprintKey("docs", nestedPath, "area:"+hot),
			})
		} else {
			draft = append(draft, Recommendation{
				ID: fmt.Sprintf("rec_%d_root_agents", now.UnixNano()), SessionID: sessionID,
				Type:  "docs",
				Title: "Clarify hot paths in root AGENTS.md",
				Rationale: "Eval wandering is high without a single dominant package — root map is the shared fix.",
				Why: "Why: the agent ran many searches without a clear package home.\n" +
					"Change: append a short Hot paths section to root AGENTS.md (shared by every coding agent).\n" +
					"How it helps: the next session starts from documented entrypoints and spends fewer tokens rediscovering structure.",
				RelatedSessions: nonEmpty(sessionID),
				Evidence: []string{
					fmt.Sprintf("wandering score %.2f", evalRes.Dimensions["wandering"]),
				},
				ProposedPath: paths.AgentsMD,
				ProposedBody: "## Hot paths\n\n- Document primary services and entrypoints discovered this session.\n- Prefer `so graph query` before broad Grep when asking how an area works.\n",
				Status:       "pending", CreatedAt: now,
				Fingerprint: FingerprintKey("docs", paths.AgentsMD, "architecture"),
			})
		}
	}
	if evalRes.Dimensions["harness_use"] < 0.4 {
		where := ".agents/skills"
		if vendor != "" {
			where = harness.SkillsRelForKind(vendorLabel)
			if where == "" {
				where = ".agents/skills"
			}
		}
		draft = append(draft, Recommendation{
			ID: fmt.Sprintf("rec_%d_skill", now.UnixNano()), SessionID: sessionID,
			Type:  "skill",
			Title: "Add a " + vendorLabel + " skill to prefer harness before search",
			Rationale: "Eval harness_use is low — this " + vendorLabel + " session skipped AGENTS.md / rules / skills / so graph.",
			Why: "Why: the agent explored without reading durable guidance.\n" +
				"Change: create `" + where + "/reduce-exploration/SKILL.md` for this vendor (Cursor→.cursor, Claude→.claude, …).\n" +
				"How it helps: the next " + vendorLabel + " session can invoke that skill and jump to graph + AGENTS.md instead of broad Grep, saving tokens.",
			RelatedSessions: nonEmpty(sessionID),
			Evidence: []string{
				fmt.Sprintf("harness_use score %.2f", evalRes.Dimensions["harness_use"]),
				"vendor=" + vendorLabel,
			},
			ProposedPath: skillPath,
			ProposedBody: "# Reduce exploration\n\nUse this when starting a codebase question.\n\n1. Run `so graph query \"…\"` before Grep/Glob.\n2. Read root `AGENTS.md` and any nested `*/AGENTS.md` for the area you touch.\n3. Prefer existing project skills/rules over inventing a search plan.\n",
			Status:       "pending", CreatedAt: now,
			Fingerprint: FingerprintKey("skill", skillPath, "reduce-exploration"),
		})
	}
	if evalRes.Dimensions["scope"] < 0.5 {
		guardBody := advisoryGuardrailBody("avoid-unrelated-drift", "Keep edits scoped to the requested task; avoid unrelated drive-by refactors.")
		draft = append(draft, Recommendation{
			ID: fmt.Sprintf("rec_%d_guard", now.UnixNano()), SessionID: sessionID,
			Type:  "guardrail",
			Title: "Warn on unrelated drive-by edits",
			Rationale: "Eval scope is low — edits look broader than the asked task.",
			Why: "Why: the session edited without enough read context or drifted outside the request.\n" +
				"Change: add/strengthen the avoid-unrelated-drift warn rule in `.so/guardrails/guardrails.yaml`.\n" +
				"How it helps: the next session gets an explicit stop/warn signal before sprawling diffs, keeping PRs reviewable.",
			RelatedSessions: nonEmpty(sessionID),
			Evidence: []string{
				fmt.Sprintf("scope score %.2f", evalRes.Dimensions["scope"]),
			},
			ProposedPath: paths.GuardrailsFile,
			ProposedBody: guardBody,
			Status:       "pending", CreatedAt: now,
			Fingerprint: FingerprintKey("guardrail", paths.GuardrailsFile, "avoid-unrelated"),
		})
	}

	// No title-rewrite agent calls (token budget).
	return MergePending(paths, draft)
}

func nestedAgentsBody(hot string) string {
	return nativedocs.DefaultAgentsBody(
		"Area: `"+hot+"`\n\nFocus guidance for this package. Prefer this file over root AGENTS.md when working here.\n\n## Where to look first\n\n- Read neighboring packages only when this area imports them.\n- Use `so graph query` scoped to this package's symbols before repo-wide Grep.\n",
		"## Conventions\n\n- Keep changes inside `"+hot+"` unless the task explicitly requires cross-package edits.\n- Run focused tests for this package before finishing.\n",
		"",
	)
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

func Apply(paths harness.Paths, id string, decision Decision) error {
	decision, err := normalizeDecision(decision)
	if err != nil {
		return err
	}
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
	case "skill":
		name := skillNameFromPath(applied.ProposedPath)
		if name == "" {
			name = "reduce-exploration"
		}
		vendor := sessionVendor(paths, applied)
		if err := nativedocs.UpsertSkill(paths, name, applied.ProposedBody, nativedocs.WriteOpts{Vendor: vendor}); err != nil {
			return err
		}
		applied.AppliedPaths = harness.FindExistingSkills(paths.RepoRoot, name)
		if len(applied.AppliedPaths) == 0 && applied.ProposedPath != "" {
			applied.AppliedPaths = []string{applied.ProposedPath}
		}
	case "docs":
		if applied.ProposedPath != "" && applied.ProposedBody != "" {
			// Root or nested AGENTS.md — shared across vendors.
			body := strings.TrimSpace(applied.ProposedBody) + "\n"
			if err := os.MkdirAll(filepath.Dir(applied.ProposedPath), 0o755); err != nil {
				return err
			}
			isAgents := filepath.Base(applied.ProposedPath) == "AGENTS.md"
			switch {
			case isAgents && !prevExisted:
				// Create nested (or missing root) AGENTS.md as a full document.
				if !strings.HasPrefix(strings.TrimSpace(body), "#") {
					body = nativedocs.DefaultAgentsBody(body, "", "")
				}
				if err := nativedocs.EnsureAgentsAt(applied.ProposedPath, body, true); err != nil {
					return err
				}
			case isAgents:
				// Existing AGENTS — append into the learned section when possible.
				if err := nativedocs.AppendLearnedAt(applied.ProposedPath, body); err != nil {
					f, err2 := os.OpenFile(applied.ProposedPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
					if err2 != nil {
						return err
					}
					_, _ = f.WriteString("\n\n" + body)
					_ = f.Close()
				}
			default:
				f, err := os.OpenFile(applied.ProposedPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
				if err != nil {
					return err
				}
				_, _ = f.WriteString("\n\n" + body)
				_ = f.Close()
			}
			applied.AppliedPaths = []string{applied.ProposedPath}
		}
	default:
		if applied.ProposedPath != "" && applied.ProposedBody != "" {
			if err := os.MkdirAll(filepath.Dir(applied.ProposedPath), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(applied.ProposedPath, []byte(applied.ProposedBody), 0o644); err != nil {
				return err
			}
		}
	}

	lesson := applied.Title + " - " + applied.Rationale + " Resolution: " + decision.Reason
	store := memory.NewStore(paths)
	_ = store.AddLesson(memory.Lesson{
		Text: lesson, Scope: "workspace", Confidence: 0.9, SourceSession: applied.SessionID,
	}, memory.ModePersistent)

	applied.Status = "applied"
	applied.AppliedAt = time.Now().UTC()
	applied.PreviousBody = string(prevBody)
	applied.PreviousExisted = prevExisted
	applied.LessonText = lesson
	applied.DecisionReason = decision.Reason
	applied.DecisionActor = decision.Actor
	applied.DecisionAt = applied.AppliedAt
	if len(applied.AppliedPaths) == 0 && applied.ProposedPath != "" {
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

func Dismiss(paths harness.Paths, id string, decision Decision) error {
	decision, err := normalizeDecision(decision)
	if err != nil {
		return err
	}
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
			r.DecisionReason = decision.Reason
			r.DecisionActor = decision.Actor
			r.DecisionAt = time.Now().UTC()
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
	// A dismissal is durable product feedback. Preserve it as workspace memory so
	// future recommendation generation can avoid repeating the same bad advice.
	_ = memory.NewStore(paths).AddLesson(memory.Lesson{
		Text:          "Recommendation dismissed: " + dismissed.Title + " - " + decision.Reason,
		Scope:         "workspace",
		Confidence:    0.9,
		SourceSession: dismissed.SessionID,
	}, memory.ModePersistent)
	return SavePending(paths, kept)
}

// Revert restores the pre-apply snapshot for an applied recommendation.
func Revert(paths harness.Paths, id string, decision Decision) error {
	decision, err := normalizeDecision(decision)
	if err != nil {
		return err
	}
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
	hist[found].DecisionReason = decision.Reason
	hist[found].DecisionActor = decision.Actor
	hist[found].DecisionAt = time.Now().UTC()
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

func sessionVendor(paths harness.Paths, r *Recommendation) string {
	ids := append([]string{r.SessionID}, r.RelatedSessions...)
	store := session.NewStore(paths)
	for _, id := range ids {
		if id == "" {
			continue
		}
		if meta, err := store.Get(id); err == nil && meta.Vendor != "" {
			return meta.Vendor
		}
	}
	return ""
}

func skillNameFromPath(p string) string {
	p = filepath.ToSlash(p)
	base := filepath.Base(p)
	if strings.EqualFold(base, "SKILL.md") {
		return filepath.Base(filepath.Dir(p))
	}
	return strings.TrimSuffix(base, filepath.Ext(base))
}
