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

	"github.com/superopen/so/internal/audit"
	"github.com/superopen/so/internal/config"
	"github.com/superopen/so/internal/harness"
	"github.com/superopen/so/internal/harnessvalid"
	"github.com/superopen/so/internal/llm"
	"github.com/superopen/so/internal/memory"
	"github.com/superopen/so/internal/retention"
	"github.com/superopen/so/internal/session"
)

const (
	maxSummaryChars = 6000
	maxSnippetChars = 2500
	ledgerName      = "harvest-ledger.json"
	pendingName     = "pending-harvest.json"
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

type ledgerFile struct {
	Entries map[string]ledgerEntry `json:"entries"`
}

// Delta is the JSON shape we ask the coding-agent CLI to return.
type Delta struct {
	Lessons      []string          `json:"lessons,omitempty"`
	PrefsAppend  string            `json:"prefs_append,omitempty"`
	ProjectsNote string            `json:"projects_note,omitempty"`
	Knowledge    map[string]string `json:"knowledge,omitempty"`
	RulesAppend  map[string]string `json:"rules_append,omitempty"`
	Skills       map[string]string `json:"skills,omitempty"`
	Guardrails   string            `json:"guardrails_note,omitempty"`
	EvalsNote    string            `json:"evals_note,omitempty"`
	NeedRecs     bool              `json:"need_recs,omitempty"`
	RecsWhy      string            `json:"recs_why,omitempty"`
}

// Run harvests one session: local summary → budgeted agent CLI → apply deltas.
func Run(paths harness.Paths, cfg config.Config, sessionID string, trigger Trigger) (Result, error) {
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

	snippets := currentSnippets(paths)
	delta, usedAgent, err := proposeDelta(cfg, summary, snippets)
	if err != nil {
		_, _ = memory.NewStore(paths).Consolidate(summary, llm.NewMemoryCompleter(cfg))
		markHarvested(paths, sessionID, trigger, srcMtime)
		res.Reason = "agent_fallback"
		res.Applied = 1
		return res, nil
	}

	applied := applyDelta(paths, delta)
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
	throttle := filepath.Join(paths.MemoryDir, "idle-sweep-at")
	if data, err := os.ReadFile(throttle); err == nil {
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data))); err == nil {
			if time.Since(t) < time.Hour {
				return nil, nil
			}
		}
	}
	_ = os.WriteFile(throttle, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)

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

type pendingFile struct {
	Sessions map[string]time.Time `json:"sessions"`
}

func pendingPath(paths harness.Paths) string {
	return filepath.Join(paths.MemoryDir, pendingName)
}

func loadPending(paths harness.Paths) map[string]time.Time {
	out := map[string]time.Time{}
	data, err := os.ReadFile(pendingPath(paths))
	if err != nil {
		return out
	}
	var f pendingFile
	if json.Unmarshal(data, &f) != nil || f.Sessions == nil {
		return out
	}
	return f.Sessions
}

func savePending(paths harness.Paths, sessions map[string]time.Time) error {
	if err := os.MkdirAll(paths.MemoryDir, 0o755); err != nil {
		return err
	}
	if sessions == nil {
		sessions = map[string]time.Time{}
	}
	data, err := json.MarshalIndent(pendingFile{Sessions: sessions}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(pendingPath(paths), append(data, '\n'), 0o644)
}

// MarkPending records a session that still needs SessionEnd-equivalent harvest
// (Codex Stop and similar turn-boundary vendors).
func MarkPending(paths harness.Paths, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	m := loadPending(paths)
	m[sessionID] = time.Now().UTC()
	return savePending(paths, m)
}

// ClearPending drops a session from the pending set after a successful end harvest.
func ClearPending(paths harness.Paths, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	m := loadPending(paths)
	if _, ok := m[sessionID]; !ok {
		return nil
	}
	delete(m, sessionID)
	return savePending(paths, m)
}

const finalizePendingName = "finalize-pending"

// MarkFinalizePending records which session the next `so sessions finalize`
// (no args) should materialize - set by SessionEnd / Stop coding hooks.
func MarkFinalizePending(paths harness.Paths, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	p := filepath.Join(paths.Root, finalizePendingName)
	return os.WriteFile(p, []byte(sessionID+"\n"), 0o644)
}

// ConsumeFinalizePending returns and clears the pending finalize session id.
func ConsumeFinalizePending(paths harness.Paths) string {
	p := filepath.Join(paths.Root, finalizePendingName)
	data, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	_ = os.Remove(p)
	return strings.TrimSpace(string(data))
}

// FlushPending harvests every pending session except exceptID (usually the
// session that just started). Gives Codex/Stop-only vendors SessionEnd parity
// when the next coding session begins.
func FlushPending(paths harness.Paths, cfg config.Config, exceptID string) []Result {
	m := loadPending(paths)
	if len(m) == 0 {
		return nil
	}
	var out []Result
	remaining := map[string]time.Time{}
	for id, at := range m {
		if id == "" || id == exceptID {
			if id != "" {
				remaining[id] = at
			}
			continue
		}
		r, err := Run(paths, cfg, id, TriggerSessionEnd)
		if err != nil {
			r.Reason = err.Error()
			remaining[id] = at
		} else if r.Skipped && (r.Reason == "empty_summary" || r.Reason == "no_session") {
			remaining[id] = at
		}
		// success or nothing_new / already applied → drop from pending
		out = append(out, r)
	}
	_ = savePending(paths, remaining)
	return out
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
	metaPath := filepath.Join(paths.SessionDir(sessionID), "meta.json")
	if data, err := os.ReadFile(metaPath); err == nil {
		b.WriteString("## Meta\n")
		b.Write(data)
		b.WriteString("\n")
	}
	transcript := filepath.Join(paths.SessionDir(sessionID), "transcript.jsonl")
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
		filepath.Join(paths.MemoryDir, "preferences.md"),
		filepath.Join(paths.MemoryDir, "projects.md"),
		filepath.Join(paths.KnowledgeDir, "architecture.md"),
		filepath.Join(paths.RulesDir, "coding.md"),
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
	system := `You update a Superopen harness after a coding session.
Return ONLY compact JSON matching:
{"lessons":["..."],"prefs_append":"","projects_note":"","knowledge":{"architecture.md":"append md"},"rules_append":{"coding.md":"..."},"skills":{},"guardrails_note":"","evals_note":"","need_recs":false,"recs_why":""}
Rules: prefer empty fields; lessons only for durable always/never/prefer; knowledge/rules only when team-useful; need_recs true only if a concrete harness gap exists.`
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

func applyDelta(paths harness.Paths, d Delta) int {
	n := 0
	store := memory.NewStore(paths)
	ev := []string{"harvest_delta"}
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
		p := filepath.Join(paths.MemoryDir, "preferences.md")
		existing, _ := os.ReadFile(p)
		updated := harnessvalid.AppendPreferencesBullet(string(existing), s)
		if harnessvalid.ValidatePreferences(updated) == nil {
			_ = os.WriteFile(p, []byte(updated), 0o644)
			n++
		}
	}
	if s := strings.TrimSpace(d.ProjectsNote); s != "" {
		p := filepath.Join(paths.MemoryDir, "projects.md")
		existing, _ := os.ReadFile(p)
		updated := harnessvalid.AppendToProjectsSection(string(existing), "Notes", s)
		if harnessvalid.ValidateProjects(updated) == nil {
			_ = os.WriteFile(p, []byte(updated), 0o644)
			n++
		}
	}
	for rel, body := range d.Knowledge {
		rel = filepath.Clean(rel)
		if strings.Contains(rel, "..") || filepath.IsAbs(rel) {
			continue
		}
		full := filepath.Join(paths.KnowledgeDir, rel)
		if err := harnessvalid.ValidateSoftWrite(harnessvalid.SoftWrite{
			Path: full, Body: body, Evidence: ev, AppendOnly: true,
		}); err != nil {
			continue
		}
		appendFile(full, "\n\n"+strings.TrimSpace(body)+"\n")
		n++
	}
	for rel, body := range d.RulesAppend {
		rel = filepath.Clean(rel)
		if strings.Contains(rel, "..") || filepath.IsAbs(rel) {
			continue
		}
		full := filepath.Join(paths.RulesDir, rel)
		if err := harnessvalid.ValidateSoftWrite(harnessvalid.SoftWrite{
			Path: full, Body: body, Evidence: ev, AppendOnly: true,
		}); err != nil {
			continue
		}
		appendFile(full, "\n\n"+strings.TrimSpace(body)+"\n")
		n++
	}
	for name, body := range d.Skills {
		name = filepath.Base(name)
		if name == "." || name == "/" {
			continue
		}
		if !strings.HasSuffix(name, ".md") {
			name += ".md"
		}
		p := filepath.Join(paths.SkillsDir, name)
		if err := harnessvalid.ValidateSoftWrite(harnessvalid.SoftWrite{
			Path: p, Body: body, Evidence: ev, CreateOnly: true,
		}); err != nil {
			continue
		}
		_ = os.WriteFile(p, []byte(strings.TrimSpace(body)+"\n"), 0o644)
		n++
	}
	// Do NOT mutate guardrails.yaml or evals configs from harvest.
	// Recommendations come from eval (so finalize / so eval), not harvest.
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
	for _, name := range []string{"meta.json", "transcript.jsonl"} {
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
	data, err := os.ReadFile(filepath.Join(paths.MemoryDir, ledgerName))
	if err != nil {
		return map[string]ledgerEntry{}
	}
	var lf ledgerFile
	if json.Unmarshal(data, &lf) != nil || lf.Entries == nil {
		return map[string]ledgerEntry{}
	}
	return lf.Entries
}

func markHarvested(paths harness.Paths, sessionID string, trigger Trigger, mtime int64) {
	entries := loadLedger(paths)
	entries[sessionID] = ledgerEntry{
		SessionID:   sessionID,
		HarvestedAt: time.Now().UTC(),
		Trigger:     trigger,
		SourceMtime: mtime,
	}
	lf := ledgerFile{Entries: entries}
	data, _ := json.MarshalIndent(lf, "", "  ")
	_ = os.MkdirAll(paths.MemoryDir, 0o755)
	_ = os.WriteFile(filepath.Join(paths.MemoryDir, ledgerName), data, 0o644)
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
