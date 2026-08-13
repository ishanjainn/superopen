package recommend

import (
	"crypto/sha256"
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
	PreviousBody      string    `json:"previous_body,omitempty"`
	PreviousExisted   bool      `json:"previous_existed,omitempty"`
	AppliedAt         time.Time `json:"applied_at,omitempty"`
	AppliedPaths      []string  `json:"applied_paths,omitempty"`
	LessonText        string    `json:"lesson_text,omitempty"`
	DecisionReason    string    `json:"decision_reason,omitempty"`
	DecisionActor     string    `json:"decision_actor,omitempty"` // human | agent | system
	DecisionAt        time.Time `json:"decision_at,omitempty"`
	Vendor            string    `json:"vendor,omitempty"`
	TargetType        string    `json:"target_type,omitempty"`
	ChangeKind        string    `json:"change_kind,omitempty"`
	Confidence        float64   `json:"confidence,omitempty"`
	Verified          bool      `json:"verified,omitempty"`
	ExplicitWorkflow  bool      `json:"explicit_workflow,omitempty"`
	OccurrenceCount   int       `json:"occurrence_count,omitempty"`
	AutoApplyAfter    int       `json:"auto_apply_after,omitempty"`
	AutoApplyReason   string    `json:"auto_apply_reason,omitempty"`
	BaseContentSHA256 string    `json:"base_content_sha256,omitempty"`
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
		p = "guardrails.yaml"
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind != "" {
		return recType + "|" + p + "|" + kind
	}
	return recType + "|" + p
}

func Generate(paths harness.Paths, sessionID string, evalRes eval.Result, _ interface{}) ([]Recommendation, error) {
	if evalRes.EvidenceStatus == "insufficient" || evalRes.EvaluationScope == "snapshot" {
		return nil, nil
	}
	var draft []Recommendation
	now := time.Now().UTC()

	vendor := ""
	if meta, err := session.NewStore(paths).Get(sessionID); err == nil {
		vendor = meta.Vendor
	}
	vendorLabel := harness.NormalizeVendorKind(vendor)
	if vendorLabel == "" {
		vendorLabel = "unknown"
	}
	patterns := RecordFindings(paths, sessionID, vendorLabel, evalRes.Findings)
	if doc, err := session.NewStore(paths).ReadDocument(sessionID); err == nil {
		var ids, edited []string
		for _, retrieval := range doc.MemoryRetrievals {
			ids = append(ids, retrieval.PatternIDs...)
		}
		for _, file := range doc.Footprint.Files {
			if file.State == "edited" {
				edited = append(edited, file.Path)
			}
		}
		_ = memory.NewStore(paths).ConsolidateRetrievals(sessionID, vendorLabel, ids, edited, evalRes.Verified)
	}
	for _, d := range evalRes.Drafts {
		pattern := patterns[d.Fingerprint]
		title := strings.TrimSpace(d.Title)
		if title == "" {
			title = "Improve " + d.Type + " from session evidence"
		}
		rationale := strings.TrimSpace(d.Rationale)
		if rationale == "" {
			rationale = pattern.Summary
		}
		draft = append(draft, Recommendation{
			ID: fmt.Sprintf("rec_%d_review", now.UnixNano()+int64(len(draft))), SessionID: sessionID,
			Type: d.Type, TargetType: d.Type, ChangeKind: d.ChangeKind, Title: title,
			Rationale: rationale, Why: rationale, Fingerprint: d.Fingerprint,
			RelatedSessions: append([]string(nil), pattern.SessionIDs...), Evidence: d.Evidence,
			ProposedPath: d.Path, ProposedBody: d.Body, Status: "pending", CreatedAt: now,
			Vendor: vendorLabel, Confidence: pattern.Confidence,
			Verified: len(pattern.VerifiedSessions) > 0, ExplicitWorkflow: pattern.ExplicitWorkflow,
			OccurrenceCount: pattern.Occurrences, AutoApplyAfter: autoApplyThreshold(d.Type, d.ChangeKind),
		})
	}
	skillsDir := harness.SkillsDirForVendor(paths.RepoRoot, vendor)
	skillPath := ""
	if skillsDir != "" {
		skillPath = filepath.Join(skillsDir, "reduce-exploration", "SKILL.md")
	}

	if wandering, applicable := evalRes.Dimensions["wandering"]; applicable && wandering > 0.6 {
		hot := ""
		if len(evalRes.HotAreas) > 0 {
			hot = evalRes.HotAreas[0]
		}
		if hot != "" {
			nestedPath := filepath.Join(paths.RepoRoot, filepath.FromSlash(hot), "AGENTS.md")
			draft = append(draft, Recommendation{
				ID: fmt.Sprintf("rec_%d_area_agents", now.UnixNano()), SessionID: sessionID,
				Type:      "docs",
				Title:     "Add " + hot + "/AGENTS.md for this hot path",
				Rationale: "Eval wandering is high and file activity concentrated under " + hot + ".",
				Why: "Why: the agent searched broadly while mostly touching " + hot + ".\n" +
					"Change: create a focused " + hot + "/AGENTS.md with entrypoints, invariants, and where to look first.\n" +
					"How it helps: the next session in that package reads local guidance instead of re-grepping the whole repo, cutting tokens and wrong-file edits.",
				RelatedSessions: nonEmpty(sessionID),
				Evidence: []string{
					fmt.Sprintf("wandering score %.2f", wandering),
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
				Type:      "docs",
				Title:     "Clarify hot paths in root AGENTS.md",
				Rationale: "Eval wandering is high without a single dominant package — root map is the shared fix.",
				Why: "Why: the agent ran many searches without a clear package home.\n" +
					"Change: append a short Hot paths section to root AGENTS.md (shared by every coding agent).\n" +
					"How it helps: the next session starts from documented entrypoints and spends fewer tokens rediscovering structure.",
				RelatedSessions: nonEmpty(sessionID),
				Evidence: []string{
					fmt.Sprintf("wandering score %.2f", wandering),
				},
				ProposedPath: paths.AgentsMD,
				ProposedBody: "## Hot paths\n\n- Document primary services and entrypoints discovered this session.\n- Prefer `so graph query` before broad Grep when asking how an area works.\n",
				Status:       "pending", CreatedAt: now,
				Fingerprint: FingerprintKey("docs", paths.AgentsMD, "architecture"),
			})
		}
	}
	if harnessUse, applicable := evalRes.Dimensions["harness_use"]; applicable && harnessUse < 0.4 && skillPath != "" {
		where := harness.SkillsRelForKind(vendorLabel)
		draft = append(draft, Recommendation{
			ID: fmt.Sprintf("rec_%d_skill", now.UnixNano()), SessionID: sessionID,
			Type:      "skill",
			Title:     "Add a " + vendorLabel + " skill to prefer harness before search",
			Rationale: "Eval harness_use is low — this " + vendorLabel + " session skipped AGENTS.md / rules / skills / so graph.",
			Why: "Why: the agent explored without reading durable guidance.\n" +
				"Change: create `" + where + "/reduce-exploration/SKILL.md` for this vendor (Cursor→.cursor, Claude→.claude, …).\n" +
				"How it helps: the next " + vendorLabel + " session can invoke that skill and jump to graph + AGENTS.md instead of broad Grep, saving tokens.",
			RelatedSessions: nonEmpty(sessionID),
			Evidence: []string{
				fmt.Sprintf("harness_use score %.2f", harnessUse),
				"vendor=" + vendorLabel,
			},
			ProposedPath: skillPath,
			ProposedBody: "# Reduce exploration\n\nUse this when starting a codebase question.\n\n1. Run `so graph query \"…\"` before Grep/Glob.\n2. Read root `AGENTS.md` and any nested `*/AGENTS.md` for the area you touch.\n3. Prefer existing project skills/rules over inventing a search plan.\n",
			Status:       "pending", CreatedAt: now,
			Fingerprint: FingerprintKey("skill", skillPath, "reduce-exploration"),
		})
	}
	if scope, applicable := evalRes.Dimensions["scope"]; applicable && scope < 0.5 {
		guardBody := advisoryGuardrailBody("avoid-unrelated-drift", "Keep edits scoped to the requested task; avoid unrelated drive-by refactors.")
		draft = append(draft, Recommendation{
			ID: fmt.Sprintf("rec_%d_guard", now.UnixNano()), SessionID: sessionID,
			Type:      "guardrail",
			Title:     "Warn on unrelated drive-by edits",
			Rationale: "Eval scope is low — edits look broader than the asked task.",
			Why: "Why: the session edited without enough read context or drifted outside the request.\n" +
				"Change: add/strengthen the avoid-unrelated-drift warn rule in `.so/guardrails.yaml`.\n" +
				"How it helps: the next session gets an explicit stop/warn signal before sprawling diffs, keeping PRs reviewable.",
			RelatedSessions: nonEmpty(sessionID),
			Evidence: []string{
				fmt.Sprintf("scope score %.2f", scope),
			},
			ProposedPath: paths.GuardrailsFile,
			ProposedBody: guardBody,
			Status:       "pending", CreatedAt: now,
			Fingerprint: FingerprintKey("guardrail", paths.GuardrailsFile, "avoid-unrelated"),
		})
	}

	// Coarse heuristic recommendations also participate in the same durable
	// evidence model, so all existing recommendation sources share one policy.
	for i := range draft {
		draft[i].Verified = draft[i].Verified || evalRes.Verified
		enrichRecommendation(paths, sessionID, vendorLabel, &draft[i], patterns)
	}
	return MergePending(paths, draft)
}

// RecordFindings consolidates session review evidence into durable memory even
// when automatic recommendation generation is disabled.
func RecordFindings(paths harness.Paths, sessionID, vendor string, findings []session.ReviewFinding) map[string]memory.Pattern {
	store := memory.NewStore(paths)
	footprint, _ := session.NewStore(paths).GetFootprint(sessionID)
	modified := map[string]bool{}
	for _, file := range footprint.Files {
		if file.State == "edited" {
			modified[filepath.ToSlash(file.Path)] = true
		}
	}
	out := map[string]memory.Pattern{}
	for _, finding := range findings {
		if harness.NormalizeVendorKind(finding.Vendor) != harness.NormalizeVendorKind(vendor) {
			continue
		}
		if finding.Kind == "failure" && finding.TargetPath != "" {
			_ = store.RecordContradiction(sessionID, vendor, finding.TargetPath)
		}
		pattern, err := store.UpsertPattern(memory.Pattern{
			Fingerprint: finding.Fingerprint, Vendor: vendor, Kind: finding.Kind,
			ChangeKind: finding.ChangeKind, TargetType: finding.TargetType, TargetPath: finding.TargetPath,
			Summary: finding.Summary, Evidence: finding.Evidence, Confidence: finding.Confidence,
			ExplicitWorkflow: finding.ExplicitWorkflow, Scope: findingScope(finding),
			Paths: mergeSessions(nonEmpty(finding.TargetPath), finding.Paths), Keywords: append(finding.Keywords, strings.Fields(finding.Summary)...),
			Symbols: finding.Symbols, ErrorSignatures: finding.ErrorSignatures, Applicability: finding.Applicability,
			SourceSHA256: fileSHA256(filepath.Join(paths.RepoRoot, filepath.FromSlash(finding.TargetPath))),
			EvidenceRefs: []memory.EvidenceRef{{SessionID: sessionID, EventIDs: finding.EventIDs, Summary: finding.Summary, Modified: modified[filepath.ToSlash(finding.TargetPath)], SessionFileCount: len(footprint.Files)}},
		}, sessionID, finding.Verified)
		if err == nil {
			out[finding.Fingerprint] = pattern
		}
	}
	return out
}

func findingScope(f session.ReviewFinding) string {
	if !f.ExplicitWorkflow {
		return "vendor"
	}
	if f.TargetType == "memory" || (f.TargetType == "docs" && strings.EqualFold(filepath.Base(f.TargetPath), "AGENTS.md")) {
		return "shared"
	}
	return "vendor"
}

func enrichRecommendation(paths harness.Paths, sessionID, vendor string, rec *Recommendation, known map[string]memory.Pattern) {
	if rec.Fingerprint == "" {
		rec.Fingerprint = FingerprintKey(rec.Type, rec.ProposedPath, rec.Title)
	}
	pattern, ok := known[rec.Fingerprint]
	if !ok {
		rec.Fingerprint = harness.NormalizeVendorKind(vendor) + "|" + rec.Fingerprint
		verified := rec.Verified
		pattern, _ = memory.NewStore(paths).UpsertPattern(memory.Pattern{
			Fingerprint: rec.Fingerprint, Vendor: vendor, Kind: "guidance_gap",
			ChangeKind: nonEmptyString(rec.ChangeKind, inferChangeKind(rec.ProposedPath)),
			TargetType: rec.Type, TargetPath: repoRelative(paths.RepoRoot, rec.ProposedPath),
			Summary: rec.Rationale, Evidence: rec.Evidence, Confidence: nonZeroFloat(rec.Confidence, 0.65),
			ExplicitWorkflow: rec.ExplicitWorkflow, Paths: nonEmpty(repoRelative(paths.RepoRoot, rec.ProposedPath)),
			Keywords: strings.Fields(rec.Rationale), EvidenceRefs: []memory.EvidenceRef{{SessionID: sessionID, Summary: rec.Rationale}},
		}, sessionID, verified)
	}
	rec.Vendor = vendor
	rec.TargetType = nonEmptyString(rec.TargetType, rec.Type)
	rec.ChangeKind = nonEmptyString(rec.ChangeKind, inferChangeKind(rec.ProposedPath))
	rec.Confidence = pattern.Confidence
	rec.Verified = len(pattern.VerifiedSessions) > 0
	rec.ExplicitWorkflow = pattern.ExplicitWorkflow
	rec.OccurrenceCount = pattern.Occurrences
	rec.AutoApplyAfter = autoApplyThreshold(rec.Type, rec.ChangeKind)
	if len(pattern.SessionIDs) > 0 {
		rec.RelatedSessions = mergeSessions(rec.RelatedSessions, pattern.SessionIDs)
	}
	if rec.ChangeKind == "update" && rec.ProposedPath != "" {
		rec.BaseContentSHA256 = fileSHA256(rec.ProposedPath)
	}
}

func autoApplyThreshold(recType, changeKind string) int {
	typ := strings.ToLower(strings.TrimSpace(recType))
	if strings.EqualFold(changeKind, "create") && (typ == "skill" || typ == "rule" || typ == "rules" || typ == "docs") {
		return 3
	}
	return 1
}

func inferChangeKind(path string) string {
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return "update"
	}
	return "create"
}

func repoRelative(root, path string) string {
	if path == "" {
		return ""
	}
	if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

func nonEmptyString(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func nonZeroFloat(value, fallback float64) float64 {
	if value != 0 {
		return value
	}
	return fallback
}

func fileSHA256(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
}

func validateCurrent(r Recommendation) error {
	switch strings.ToLower(strings.TrimSpace(r.ChangeKind)) {
	case "create":
		if info, err := os.Stat(r.ProposedPath); err == nil && !info.IsDir() {
			return fmt.Errorf("target now exists")
		}
	case "update":
		if r.BaseContentSHA256 != "" && fileSHA256(r.ProposedPath) != r.BaseContentSHA256 {
			return fmt.Errorf("target changed after review")
		}
	case "remove", "restructure":
		return fmt.Errorf("%s requires explicit implementation", r.ChangeKind)
	}
	return nil
}

func validateOwnership(paths harness.Paths, r Recommendation) error {
	vendor := harness.NormalizeVendorKind(r.Vendor)
	if vendor == "" {
		vendor = harness.NormalizeVendorKind(sessionVendor(paths, &r))
	}
	if vendor == "" {
		return fmt.Errorf("unsupported originating vendor")
	}
	path, err := filepath.Abs(r.ProposedPath)
	if err != nil {
		return err
	}
	within := func(root string) bool {
		root, _ = filepath.Abs(root)
		return root != "" && (path == root || strings.HasPrefix(path, root+string(os.PathSeparator)))
	}
	switch strings.ToLower(strings.TrimSpace(r.Type)) {
	case "skill":
		if !within(harness.SkillsDirForVendor(paths.RepoRoot, vendor)) {
			return fmt.Errorf("skill target is outside %s vendor tree", vendor)
		}
		lower := strings.ToLower(filepath.ToSlash(path))
		if strings.Contains(lower, "/skills/so/") || strings.Contains(lower, "/skills/superopen/") {
			return fmt.Errorf("managed Superopen skill is protected")
		}
	case "rules":
		if !within(harness.RulesDirForVendor(paths.RepoRoot, vendor)) {
			return fmt.Errorf("rule target is outside %s vendor tree", vendor)
		}
	case "docs":
		if !within(paths.RepoRoot) || filepath.Base(path) != "AGENTS.md" {
			return fmt.Errorf("shared documentation target must be AGENTS.md inside the repository")
		}
	case "guardrail":
		if path != paths.GuardrailsFile {
			return fmt.Errorf("guardrail target must be guardrails.yaml")
		}
	case "eval", "evals":
		if path != paths.EvalsConfig {
			return fmt.Errorf("evaluation target must be evals.yaml")
		}
	}
	return nil
}

func mergeEvidence(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range append(a, b...) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
		if len(out) == 10 {
			break
		}
	}
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
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
	all, err := loadAll(paths)
	if err != nil {
		return nil, err
	}
	var out []Recommendation
	for _, r := range all {
		if r.Status == "pending" || r.Status == "stale" {
			out = append(out, r)
		}
	}
	return out, nil
}

func SavePending(paths harness.Paths, recs []Recommendation) error {
	all, err := loadAll(paths)
	if err != nil {
		return err
	}
	byID := map[string]Recommendation{}
	for _, r := range all {
		if r.Status != "pending" && r.Status != "stale" {
			byID[r.ID] = r
		}
	}
	for _, r := range recs {
		byID[r.ID] = r
	}
	return saveAll(paths, byID)
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
			r.Evidence = mergeEvidence(prev.Evidence, r.Evidence)
			r.OccurrenceCount = maxInt(prev.OccurrenceCount, r.OccurrenceCount)
			r.Verified = r.Verified || prev.Verified
			r.ExplicitWorkflow = r.ExplicitWorkflow || prev.ExplicitWorkflow
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
	if err := validateOwnership(paths, *applied); err != nil {
		return fmt.Errorf("recommendation ownership: %w", err)
	}
	if err := validateCurrent(*applied); err != nil {
		return fmt.Errorf("recommendation is stale: %w", err)
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
		if harness.SkillsDirForVendor(paths.RepoRoot, vendor) == "" {
			return fmt.Errorf("recommendation has no explicit supported session vendor")
		}
		if err := nativedocs.UpsertSkill(paths, name, applied.ProposedBody, nativedocs.WriteOpts{Vendor: vendor}); err != nil {
			return err
		}
		applied.AppliedPaths = []string{filepath.Join(harness.SkillsDirForVendor(paths.RepoRoot, vendor), name, "SKILL.md")}
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
					return fmt.Errorf("AGENTS.md has no valid managed learned section: %w", err)
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
	if decision.Actor == "system" {
		applied.AutoApplyReason = decision.Reason
	}
	if len(applied.AppliedPaths) == 0 && applied.ProposedPath != "" {
		applied.AppliedPaths = []string{applied.ProposedPath}
	}

	_ = saveOne(paths, *applied)
	_ = memory.NewStore(paths).SetPatternStatus(applied.Fingerprint, applied.Vendor, "applied")
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
		DeniedTools: eng.Policy.DeniedTools, DeniedCommands: eng.Policy.DeniedCommands, SensitivePaths: eng.Policy.SensitivePaths,
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
	for _, tool := range incoming.DeniedTools {
		if !containsStr(existing.DeniedTools, tool) {
			existing.DeniedTools = append(existing.DeniedTools, tool)
		}
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
	header := "# Superopen guardrails. These are shared project safety rules enforced at coding-agent hook boundaries.\n# Authoritative project policy updated by project maintainers and approved recommendations.\n\n"
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
	_ = saveOne(paths, *dismissed)
	_ = memory.NewStore(paths).SetPatternStatus(dismissed.Fingerprint, dismissed.Vendor, "dismissed")
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
	_ = memory.NewStore(paths).RecordPatternRevert(hist[found].Fingerprint, hist[found].Vendor, decision.Reason)
	return saveOne(paths, hist[found])
}

// ShouldAutoApply is the shared policy for finalization. Soft updates may be
// applied immediately, but creating guidance requires recurrence plus verified
// execution; policy changes, removals, and restructures always require review.
func ShouldAutoApply(paths harness.Paths, r Recommendation) (bool, string) {
	change := strings.ToLower(strings.TrimSpace(r.ChangeKind))
	typ := strings.ToLower(strings.TrimSpace(r.Type))
	path := filepath.ToSlash(strings.ToLower(r.ProposedPath))
	if change == "remove" || change == "restructure" {
		return false, "removals and restructures require approval"
	}
	if typ == "guardrail" || typ == "policy" || typ == "eval" || typ == "evals" {
		return false, "guardrail and evaluation policy changes require approval"
	}
	if strings.Contains(path, "/.agents/") || strings.Contains(path, "/skills/so/") || strings.Contains(path, "/skills/superopen/") {
		return false, "shared agents and managed Superopen skills are protected"
	}
	if harness.NormalizeVendorKind(r.Vendor) == "" {
		return false, "recommendation has no supported originating vendor"
	}
	createsGuidance := change == "create" && (typ == "skill" || typ == "rule" || typ == "rules" || typ == "docs")
	if createsGuidance {
		if !r.Verified {
			return false, "new guidance workflow has not been verified"
		}
		if r.OccurrenceCount < 3 && !r.ExplicitWorkflow {
			return false, fmt.Sprintf("new guidance has %d of 3 supporting sessions", r.OccurrenceCount)
		}
		if typ == "skill" {
			name := skillNameFromPath(r.ProposedPath)
			for _, existing := range harness.FindExistingSkills(paths.RepoRoot, name) {
				if filepath.Clean(existing) != filepath.Clean(r.ProposedPath) {
					return false, "an existing skill already covers this name"
				}
			}
		}
		if r.ExplicitWorkflow {
			return true, "verified explicit workflow satisfies the automatic creation threshold"
		}
		return true, "verified workflow reached the automatic creation threshold"
	}
	return true, "validated existing guidance update"
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
	all, err := loadAll(paths)
	if err != nil {
		return nil, err
	}
	var out []Recommendation
	for _, r := range all {
		if r.Status != "pending" && r.Status != "stale" {
			out = append(out, r)
		}
	}
	return out, nil
}

func loadAll(paths harness.Paths) ([]Recommendation, error) {
	entries, err := os.ReadDir(paths.SessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Recommendation
	store := session.NewStore(paths)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		d, err := store.ReadDocument(e.Name())
		if err != nil || len(d.Recommendations) == 0 {
			continue
		}
		var rs []Recommendation
		if json.Unmarshal(d.Recommendations, &rs) == nil {
			out = append(out, rs...)
		}
	}
	return out, nil
}

func saveAll(paths harness.Paths, all map[string]Recommendation) error {
	bySession := map[string][]Recommendation{}
	for _, r := range all {
		id := strings.TrimSpace(r.SessionID)
		if id == "" {
			id = "_system"
		}
		bySession[id] = append(bySession[id], r)
	}
	store := session.NewStore(paths)
	entries, _ := os.ReadDir(paths.SessionsDir)
	seen := map[string]bool{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		seen[id] = true
		rs := bySession[id]
		raw, _ := json.Marshal(rs)
		if err := store.WriteDocument(id, func(d *session.Document) { d.Recommendations = raw }); err != nil {
			return err
		}
	}
	for id, rs := range bySession {
		if seen[id] {
			continue
		}
		raw, _ := json.Marshal(rs)
		if err := store.WriteDocument(id, func(d *session.Document) { d.Recommendations = raw }); err != nil {
			return err
		}
	}
	return nil
}

func saveOne(paths harness.Paths, rec Recommendation) error {
	all, err := loadAll(paths)
	if err != nil {
		return err
	}
	m := map[string]Recommendation{}
	for _, r := range all {
		m[r.ID] = r
	}
	m[rec.ID] = rec
	return saveAll(paths, m)
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
