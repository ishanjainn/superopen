// Package harvest updates memory + harness docs after session end or long idle.
// Cost constraint: summarize locally first, then one small agent-CLI call for JSON deltas.
package harvest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ishanjainn/superopen/internal/audit"
	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/llm"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/nativedocs"
	"github.com/ishanjainn/superopen/internal/retention"
	"github.com/ishanjainn/superopen/internal/runtimestate"
	"github.com/ishanjainn/superopen/internal/session"
)

const (
	maxSummaryChars = 6000
	maxSnippetChars = 2500
	// turnEndDebounce limits Codex Stop harvests so mid-session turns
	// don't spam the agent CLI; pending flush on SessionStart catches the rest.
	turnEndDebounce = 15 * time.Minute
)

// Trigger identifies why harvest ran.
type Trigger string

const (
	TriggerSessionEnd Trigger = "session_end"
	TriggerIdle       Trigger = "idle"
	TriggerFinalize   Trigger = "finalize"
	// TriggerTurnEnd is used when a vendor has no SessionEnd (e.g. Codex Stop).
	TriggerTurnEnd Trigger = "turn_end"
)

// Result is a machine-readable harvest outcome.
type Result struct {
	SessionID string  `json:"session_id"`
	Trigger   Trigger `json:"trigger"`
	Skipped   bool    `json:"skipped,omitempty"`
	Reason    string  `json:"reason,omitempty"`
	Applied   int     `json:"applied,omitempty"`
	Recs      int     `json:"recs,omitempty"`
}

type ledgerEntry struct {
	SessionID   string    `json:"session_id"`
	HarvestedAt time.Time `json:"harvested_at"`
	Trigger     Trigger   `json:"trigger"`
	SourceMtime int64     `json:"source_mtime"`
}

// Delta is the JSON shape we ask the coding-agent CLI to return.
// Prefer update/remove over endless append — prune superseded guidance.
type Delta struct {
	Lessons         []string            `json:"lessons,omitempty"`
	PrefsAppend     string              `json:"prefs_append,omitempty"`
	ProjectsNote    string              `json:"projects_note,omitempty"`
	Knowledge       map[string]string   `json:"knowledge,omitempty"`        // agents_rel → append text ("" = root AGENTS.md)
	KnowledgeRemove map[string][]string `json:"knowledge_remove,omitempty"` // agents_rel → needles to drop from learned
	RulesAppend     map[string]string   `json:"rules_append,omitempty"`
	RulesSet        map[string]string   `json:"rules_set,omitempty"` // replace whole rule file
	RulesRemove     map[string][]string `json:"rules_remove,omitempty"`
	Skills          map[string]string   `json:"skills,omitempty"`
	SkillsSet       map[string]string   `json:"skills_set,omitempty"`
	SkillsRemove    []string            `json:"skills_remove,omitempty"`
	Guardrails      string              `json:"guardrails_note,omitempty"`
	EvalsNote       string              `json:"evals_note,omitempty"`
	NeedRecs        bool                `json:"need_recs,omitempty"`
	RecsWhy         string              `json:"recs_why,omitempty"`
}

// RunOpts controls harvest side effects.
type RunOpts struct {
	// SkipNativeDocs writes only untracked memory (lessons/prefs); leaves
	// AGENTS.md / rules / skills untouched so git hooks stay clean.
	SkipNativeDocs bool
	// LocalOnly records the harvest cursor/history and refreshes context without
	// a second reviewer call. Finalization uses this after eval already returned
	// evaluation, recommendation, and memory results in one invocation.
	LocalOnly bool
}

// Run harvests one session: local summary → budgeted agent CLI → apply deltas.
func Run(paths harness.Paths, cfg config.Config, sessionID string, trigger Trigger, opts ...RunOpts) (Result, error) {
	var o RunOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	res := Result{SessionID: sessionID, Trigger: trigger}
	if !cfg.MemoryEnabled() && trigger != TriggerFinalize {
		res.Skipped = true
		res.Reason = "memory.disabled"
		return res, nil
	}
	if sessionID == "" {
		res.Skipped = true
		res.Reason = "no_session"
		return res, nil
	}

	srcMtime := sessionSourceMtime(paths, sessionID)
	if last, ok := loadLedger(paths)[sessionID]; ok {
		if last.SourceMtime > 0 && srcMtime > 0 && srcMtime <= last.SourceMtime {
			res.Skipped = true
			res.Reason = "nothing_new"
			return res, nil
		}
		if trigger == TriggerIdle && !last.HarvestedAt.IsZero() {
			res.Skipped = true
			res.Reason = "already_harvested_idle"
			return res, nil
		}
		if trigger == TriggerTurnEnd && !last.HarvestedAt.IsZero() && time.Since(last.HarvestedAt) < turnEndDebounce {
			res.Skipped = true
			res.Reason = "turn_end_debounced"
			return res, nil
		}
	}

	summary := summarizeLocal(paths, sessionID)
	if strings.TrimSpace(summary) == "" {
		res.Skipped = true
		res.Reason = "empty_summary"
		return res, nil
	}

	vendor := ""
	if meta, err := session.NewStore(paths).Get(sessionID); err == nil {
		vendor = meta.Vendor
	}
	if o.LocalOnly {
		store := memory.NewStore(paths)
		_ = store.AppendHistory(summary)
		_, _ = store.BuildSessionContext(12000, "", memory.ModePersistent)
		markHarvested(paths, sessionID, trigger, srcMtime)
		res.Reason = "review_result"
		return res, nil
	}

	snippets := currentSnippets(paths)
	delta, usedAgent, err := proposeDelta(cfg, summary, snippets)
	if err != nil {
		_, _ = memory.NewStore(paths).Consolidate(summary, llm.NewMemoryCompleter(cfg))
		markHarvested(paths, sessionID, trigger, srcMtime)
		res.Reason = "agent_fallback"
		res.Applied = 1
		return res, nil
	}

	// Harness guidance changes flow through session recommendations so they are
	// evidence-linked, vendor-scoped, current-file validated, and reversible.
	// Harvest owns memory consolidation only; it never mutates live guidance.
	applied := applyDelta(paths, delta, vendor, true)
	_, _ = memory.NewStore(paths).Consolidate(summary, llm.NewMemoryCompleter(cfg))
	applied++

	markHarvested(paths, sessionID, trigger, srcMtime)
	_ = audit.Append(paths, audit.Event{
		Action:  "harvest",
		Type:    "session",
		Session: sessionID,
		Detail:  fmt.Sprintf("trigger=%s applied=%d agent=%v", trigger, applied, usedAgent),
	})
	res.Applied = applied
	return res, nil
}

// IdleSweep harvests sessions with no activity for cfg idle hours.
// Throttled to at most once per hour per harness (safe to call from Codex Stop).
func IdleSweep(paths harness.Paths, cfg config.Config) ([]Result, error) {
	run, err := runtimestate.TouchIfStale(paths.RepoRoot, "idle_sweep", time.Hour)
	if err != nil {
		return nil, err
	}
	if !run {
		return nil, nil
	}

	_, _ = retention.Prune(paths, cfg)

	hours := cfg.IdleHarvestHours()
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
	store := session.NewStore(paths)
	metas, err := store.List()
	if err != nil {
		return nil, err
	}
	var out []Result
	for _, m := range metas {
		if m.Status == session.StatusEnded || m.EndedAt != nil {
			continue
		}
		last := m.StartedAt
		if mt := sessionSourceMtime(paths, m.ID); mt > 0 {
			last = time.Unix(mt, 0)
		}
		if last.After(cutoff) {
			continue
		}
		r, err := Run(paths, cfg, m.ID, TriggerIdle)
		if err != nil {
			r.Reason = err.Error()
		}
		out = append(out, r)
	}
	return out, nil
}

// MarkPending records a session that still needs SessionEnd-equivalent harvest
// (Codex Stop and similar turn-boundary vendors).
func MarkPending(paths harness.Paths, sessionID, vendor string) error {
	if sessionID == "" {
		return nil
	}
	store := session.NewStore(paths)
	if _, err := store.Get(sessionID); err != nil {
		_ = store.Start(session.Meta{ID: sessionID, Vendor: vendor, StartedAt: time.Now().UTC()})
	}
	return store.WriteDocument(sessionID, func(d *session.Document) { d.Review.Status = "pending"; d.Review.Trigger = string(TriggerTurnEnd) })
}

// ClearPending drops a session from the pending set after a successful end harvest.
func ClearPending(paths harness.Paths, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	return session.NewStore(paths).WriteDocument(sessionID, func(d *session.Document) {
		if d.Review.Status == "pending" {
			d.Review.Status = ""
		}
	})
}

// FlushPending harvests every pending session except exceptID (usually the
// session that just started). Gives Codex/Stop-only vendors SessionEnd parity
// when the next coding session begins.
func FlushPending(paths harness.Paths, cfg config.Config, exceptID string) []Result {
	return FlushPendingVendor(paths, cfg, exceptID, "")
}

// FlushPendingVendor processes only the immediately preceding unreviewed
// session owned by the vendor starting a new chat.
func FlushPendingVendor(paths harness.Paths, cfg config.Config, exceptID, vendor string) []Result {
	store := session.NewStore(paths)
	entries, _ := store.List()
	for _, m := range entries {
		if m.ID == "" || m.ID == exceptID {
			continue
		}
		if vendor != "" && harness.NormalizeVendorKind(m.Vendor) != harness.NormalizeVendorKind(vendor) {
			continue
		}
		d, err := store.ReadDocument(m.ID)
		if err != nil || d.Review.Status != "pending" {
			continue
		}
		r, runErr := Run(paths, cfg, m.ID, TriggerSessionEnd)
		if runErr != nil {
			r.Reason = runErr.Error()
		} else {
			_ = ClearPending(paths, m.ID)
		}
		return []Result{r}
	}
	return nil
}

// PendingVendor returns only the immediately preceding pending session for a
// vendor. It performs no review work, so SessionStart can remain non-blocking.
func PendingVendor(paths harness.Paths, exceptID, vendor string) string {
	store := session.NewStore(paths)
	entries, _ := store.List()
	for _, m := range entries {
		if m.ID == "" || m.ID == exceptID || harness.NormalizeVendorKind(m.Vendor) != harness.NormalizeVendorKind(vendor) {
			continue
		}
		d, err := store.ReadDocument(m.ID)
		if err == nil && (d.Review.Status == "pending" || d.Review.Status == "failed") {
			return m.ID
		}
		return ""
	}
	return ""
}

// RunDebounced is Run for turn-end triggers (debounce lives inside Run).
func RunDebounced(paths harness.Paths, cfg config.Config, sessionID string, trigger Trigger) (Result, error) {
	if trigger == "" {
		trigger = TriggerTurnEnd
	}
	return Run(paths, cfg, sessionID, trigger)
}

func summarizeLocal(paths harness.Paths, sessionID string) string {
	var b strings.Builder
	metaPath := filepath.Join(paths.SessionDir(sessionID), "session.json")
	if data, err := os.ReadFile(metaPath); err == nil {
		b.WriteString("## Meta\n")
		b.Write(data)
		b.WriteString("\n")
	}
	transcript := filepath.Join(paths.SessionDir(sessionID), "events.jsonl")
	if data, err := os.ReadFile(transcript); err == nil {
		b.WriteString("## Transcript (truncated)\n")
		b.WriteString(truncateRunes(extractTextTurns(string(data)), maxSummaryChars))
	}
	if b.Len() == 0 {
		if data, err := os.ReadFile(paths.Lessons); err == nil {
			b.WriteString(truncateRunes(string(data), 2000))
		}
	}
	return b.String()
}

func extractTextTurns(jsonl string) string {
	var parts []string
	for _, line := range strings.Split(jsonl, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row map[string]any
		if json.Unmarshal([]byte(line), &row) != nil {
			continue
		}
		role, _ := row["role"].(string)
		text, _ := row["text"].(string)
		if text == "" {
			text, _ = row["content"].(string)
		}
		if text == "" {
			continue
		}
		if role == "" {
			role = "turn"
		}
		parts = append(parts, role+": "+firstLine(text, 400))
	}
	return strings.Join(parts, "\n")
}

func currentSnippets(paths harness.Paths) string {
	var b strings.Builder
	files := []string{
		filepath.Join(paths.MemoryDir, "state.json"),
		paths.AgentsMD,
	}
	if codingPath, err := nativedocs.RulePath(paths, "coding"); err == nil {
		files = append(files, codingPath)
	}
	n := len(files)
	if n == 0 {
		n = 1
	}
	per := maxSnippetChars / n
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		b.WriteString("### " + filepath.Base(f) + "\n")
		b.WriteString(truncateRunes(string(data), per))
		b.WriteString("\n")
	}
	return b.String()
}

func proposeDelta(cfg config.Config, summary, snippets string) (Delta, bool, error) {
	system := `You update Superopen guidance after a coding session.
Return ONLY compact JSON:
{"lessons":[],"prefs_append":"","projects_note":"","knowledge":{"":"append to root AGENTS.md learned"},"knowledge_remove":{"":["outdated needle"]},"rules_append":{"coding":"..."},"rules_set":{},"rules_remove":{"coding":["line needle"]},"skills":{},"skills_set":{},"skills_remove":[],"need_recs":false,"recs_why":""}
Rules:
- Prefer empty fields. Do not keep appending forever — use knowledge_remove / rules_remove / skills_remove when guidance is obsolete or wrong.
- knowledge keys are AGENTS.md locations: "" or omit for root; "internal/pkg" for nested internal/pkg/AGENTS.md (create only when that area needs local guidance).
- rules_* and skills_* write only to the session vendor's native tree; never copy guidance across vendors and never touch the so skill.
- AGENTS.md knowledge_* is always shared.
- lessons only for durable always/never/prefer; need_recs true only for a concrete harness gap.`
	user := "SESSION SUMMARY (local extract, truncated):\n" + summary + "\n\nCURRENT SNIPPETS:\n" + snippets

	completer := llm.NewMemoryCompleter(cfg)
	if completer == nil || !completer.Available() {
		return Delta{}, false, fmt.Errorf("no agent delta")
	}
	text, err := completer.Complete(system, user)
	if err != nil {
		return Delta{}, false, err
	}
	d, err := parseDelta(text)
	if err != nil {
		return Delta{}, false, err
	}
	usedAgent := strings.HasPrefix(completer.Backend(), "agent_cli")
	return d, usedAgent, nil
}

func parseDelta(text string) (Delta, error) {
	text = strings.TrimSpace(text)
	if i := strings.Index(text, "{"); i >= 0 {
		if j := strings.LastIndex(text, "}"); j > i {
			text = text[i : j+1]
		}
	}
	var d Delta
	if err := json.Unmarshal([]byte(text), &d); err != nil {
		return Delta{}, err
	}
	return d, nil
}

func applyDelta(paths harness.Paths, d Delta, vendor string, skipNativeDocs bool) int {
	n := 0
	wopts := nativedocs.WriteOpts{Vendor: vendor}
	store := memory.NewStore(paths)
	for _, lesson := range d.Lessons {
		lesson = strings.TrimSpace(lesson)
		if lesson == "" {
			continue
		}
		if err := store.AddLesson(memory.Lesson{Text: lesson, Scope: "workspace", Confidence: 0.7}, memory.ModePersistent); err == nil {
			n++
		}
	}
	if s := strings.TrimSpace(d.PrefsAppend); s != "" {
		if store.AppendPreferenceText(s) == nil {
			n++
		}
	}
	if s := strings.TrimSpace(d.ProjectsNote); s != "" {
		if store.AppendProjectNote(s) == nil {
			n++
		}
	}
	if skipNativeDocs {
		_ = d.Guardrails
		_ = d.EvalsNote
		return n
	}
	for rel, body := range d.Knowledge {
		agentsPath, err := nativedocs.AgentsFile(paths.RepoRoot, rel)
		if err != nil {
			continue
		}
		_ = nativedocs.EnsureAgentsAt(agentsPath, nativedocs.DefaultAgentsBody("", "", ""), false)
		if err := nativedocs.AppendLearnedAt(agentsPath, body); err == nil {
			n++
		}
	}
	for rel, needles := range d.KnowledgeRemove {
		agentsPath, err := nativedocs.AgentsFile(paths.RepoRoot, rel)
		if err != nil {
			continue
		}
		for _, needle := range needles {
			if err := nativedocs.RemoveLearnedContaining(agentsPath, needle); err == nil {
				n++
			}
		}
	}
	for rel, body := range d.RulesSet {
		if err := nativedocs.UpsertRule(paths, rel, body, wopts); err == nil {
			n++
		}
	}
	for rel, body := range d.RulesAppend {
		if err := nativedocs.AppendRule(paths, rel, body, wopts); err == nil {
			n++
		}
	}
	for rel, needles := range d.RulesRemove {
		for _, needle := range needles {
			if err := nativedocs.RemoveRuleContaining(paths, rel, needle, wopts); err == nil {
				n++
			}
		}
	}
	for name, body := range d.SkillsSet {
		name = filepath.Base(name)
		name = strings.TrimSuffix(name, ".md")
		if err := nativedocs.UpsertSkill(paths, name, body, wopts); err == nil {
			n++
		}
	}
	for name, body := range d.Skills {
		name = filepath.Base(name)
		name = strings.TrimSuffix(name, ".md")
		if err := nativedocs.WriteSkillCreateOnly(paths, name, body, wopts); err == nil {
			n++
		}
	}
	for _, name := range d.SkillsRemove {
		name = filepath.Base(name)
		name = strings.TrimSuffix(name, ".md")
		if err := nativedocs.RemoveSkill(paths, name, wopts); err == nil {
			n++
		}
	}
	// Do NOT mutate guardrails.yaml or evals configs from harvest.
	_ = d.Guardrails
	_ = d.EvalsNote
	_ = d.NeedRecs
	_ = d.RecsWhy
	return n
}

func appendFile(path, extra string) {
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(extra)
}

func sessionSourceMtime(paths harness.Paths, sessionID string) int64 {
	best := int64(0)
	for _, name := range []string{"session.json", "events.jsonl"} {
		info, err := os.Stat(filepath.Join(paths.SessionDir(sessionID), name))
		if err != nil {
			continue
		}
		if t := info.ModTime().Unix(); t > best {
			best = t
		}
	}
	return best
}

func loadLedger(paths harness.Paths) map[string]ledgerEntry {
	out := map[string]ledgerEntry{}
	store := session.NewStore(paths)
	metas, _ := store.List()
	for _, m := range metas {
		d, err := store.ReadDocument(m.ID)
		if err != nil || d.Review.HarvestedAt == nil {
			continue
		}
		out[m.ID] = ledgerEntry{SessionID: m.ID, HarvestedAt: *d.Review.HarvestedAt, Trigger: Trigger(d.Review.HarvestTrigger), SourceMtime: d.Review.HarvestedSourceMtime}
	}
	return out
}

func markHarvested(paths harness.Paths, sessionID string, trigger Trigger, mtime int64) {
	now := time.Now().UTC()
	_ = session.NewStore(paths).WriteDocument(sessionID, func(d *session.Document) {
		d.Review.HarvestedAt = &now
		d.Review.HarvestedSourceMtime = mtime
		d.Review.HarvestTrigger = string(trigger)
	})
}

func truncateRunes(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max]) + "\n…(truncated)"
}

func firstLine(s string, max int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if utf8.RuneCountInString(s) > max {
		r := []rune(s)
		s = string(r[:max])
	}
	return s
}
