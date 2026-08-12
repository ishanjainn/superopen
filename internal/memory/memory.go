package memory

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/artifactmeta"
	"github.com/ishanjainn/superopen/internal/audit"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/harnessvalid"
	"github.com/ishanjainn/superopen/internal/llm"
)

// Mode controls read/write of durable memory for a session.
type Mode string

const (
	ModePersistent Mode = "persistent"
	ModeIncognito  Mode = "incognito"
	ModeTemporary  Mode = "temporary"
)

type Lesson struct {
	ID            string    `json:"id"`
	Text          string    `json:"text"`
	Scope         string    `json:"scope"` // global | workspace
	Confidence    float64   `json:"confidence"`
	SourceSession string    `json:"source_session,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type SemanticEntry struct {
	Key        string    `json:"key"`
	Value      string    `json:"value"`
	Confidence float64   `json:"confidence"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type EpisodicEntry struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Pattern is durable, compact evidence that the same harness improvement has
// appeared across coding sessions. Full prompts, tool output, and proposed
// file bodies stay in the originating session; memory keeps only the evidence
// summary and counters needed to decide whether a recommendation is recurring.
type Pattern struct {
	Fingerprint      string                `json:"fingerprint"`
	Vendor           string                `json:"vendor"`
	Kind             string                `json:"kind"`
	ChangeKind       string                `json:"change_kind,omitempty"`
	TargetType       string                `json:"target_type,omitempty"`
	TargetPath       string                `json:"target_path,omitempty"`
	Summary          string                `json:"summary"`
	Evidence         []string              `json:"evidence,omitempty"`
	Occurrences      int                   `json:"occurrences"`
	SessionIDs       []string              `json:"session_ids,omitempty"`
	VerifiedSessions []string              `json:"verified_sessions,omitempty"`
	Confidence       float64               `json:"confidence,omitempty"`
	ExplicitWorkflow bool                  `json:"explicit_workflow,omitempty"`
	Status           string                `json:"status"` // pending | applied | dismissed | superseded
	FirstObservedAt  time.Time             `json:"first_observed_at"`
	LastObservedAt   time.Time             `json:"last_observed_at"`
	Verification     []PatternVerification `json:"verification,omitempty"`
}

type PatternVerification struct {
	SessionID string    `json:"session_id,omitempty"`
	Outcome   string    `json:"outcome"`
	At        time.Time `json:"at"`
}

type SearchHit struct {
	Kind    string  `json:"kind"` // lesson|semantic|episodic|prefs|projects|history
	ID      string  `json:"id,omitempty"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

type ContextPack struct {
	Text       string         `json:"text"`
	CharCount  int            `json:"char_count"`
	Mode       Mode           `json:"mode"`
	Sections   map[string]int `json:"sections"`
	ActivePath string         `json:"active_path"`
}

type Store struct {
	Paths harness.Paths
}

// RefreshState records the last lightweight repository refresh. It lives in
// memory/state.json so refresh coordination does not create another file.
type RefreshState struct {
	SHA        string    `json:"sha,omitempty"`
	At         time.Time `json:"at,omitempty"`
	GraphBuilt bool      `json:"graph_built,omitempty"`
}

type stateFile struct {
	About       artifactmeta.About `json:"_about"`
	Lessons     []Lesson           `json:"lessons,omitempty"`
	Semantic    []SemanticEntry    `json:"semantic,omitempty"`
	Episodic    []EpisodicEntry    `json:"episodic,omitempty"`
	Preferences string             `json:"preferences,omitempty"`
	Projects    string             `json:"projects,omitempty"`
	History     []string           `json:"history,omitempty"`
	Harvest     map[string]any     `json:"harvest,omitempty"`
	Refresh     *RefreshState      `json:"refresh,omitempty"`
	Patterns    []Pattern          `json:"patterns,omitempty"`
}

var memoryAbout = artifactmeta.About{
	Purpose:   "Consolidated lessons, preferences, project notes, harvest cursor, and memory refresh state.",
	Authority: "local durable memory state", UpdatedBy: "session review and memory consolidation",
}

func NewStore(paths harness.Paths) *Store {
	return &Store{Paths: paths}
}

func (s *Store) Ensure() error {
	if err := os.MkdirAll(s.Paths.MemoryDir, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(s.statePath()); err == nil {
		return nil
	}
	return s.writeState(stateFile{About: memoryAbout, Preferences: defaultTemplate("preferences.md"), Projects: defaultTemplate("projects.md")})
}

// repairMemoryStructure normalizes preferences/projects without wiping bullets.
func (s *Store) repairMemoryStructure() error {
	st, err := s.readState()
	if err != nil {
		return err
	}
	if harnessvalid.ValidatePreferences(st.Preferences) != nil {
		st.Preferences = harnessvalid.NormalizePreferences(st.Preferences)
	}
	if harnessvalid.ValidateProjects(st.Projects) != nil {
		st.Projects = harnessvalid.NormalizeProjects(st.Projects)
	}
	return s.writeState(st)
}

func (s *Store) statePath() string { return filepath.Join(s.Paths.MemoryDir, "state.json") }

// LoadRefreshState returns the consolidated lightweight-refresh marker.
func (s *Store) LoadRefreshState() RefreshState {
	st, err := s.readState()
	if err != nil || st.Refresh == nil {
		return RefreshState{}
	}
	return *st.Refresh
}

// SaveRefreshState updates only the refresh section of memory/state.json.
func (s *Store) SaveRefreshState(refresh RefreshState) error {
	if err := s.Ensure(); err != nil {
		return err
	}
	st, err := s.readState()
	if err != nil {
		return err
	}
	st.Refresh = &refresh
	return s.writeState(st)
}

// UpsertPattern records one session occurrence. A session contributes at most
// once to a fingerprint, which makes finalize retries idempotent.
func (s *Store) UpsertPattern(p Pattern, sessionID string, verified bool) (Pattern, error) {
	if err := s.Ensure(); err != nil {
		return Pattern{}, err
	}
	p.Fingerprint = strings.TrimSpace(p.Fingerprint)
	p.Vendor = harness.NormalizeVendorKind(p.Vendor)
	p.Summary = truncate(p.Summary, 320)
	p.Evidence = compactStrings(p.Evidence, 6, 240)
	sessionID = strings.TrimSpace(sessionID)
	if p.Fingerprint == "" || p.Vendor == "" || p.Summary == "" || sessionID == "" {
		return Pattern{}, fmt.Errorf("pattern requires fingerprint, supported vendor, summary, and session")
	}
	now := time.Now().UTC()
	st, err := s.readState()
	if err != nil {
		return Pattern{}, err
	}
	for i := range st.Patterns {
		cur := &st.Patterns[i]
		if cur.Fingerprint != p.Fingerprint || cur.Vendor != p.Vendor {
			continue
		}
		if !containsString(cur.SessionIDs, sessionID) {
			cur.SessionIDs = append(cur.SessionIDs, sessionID)
			cur.Occurrences++
		}
		if verified && !containsString(cur.VerifiedSessions, sessionID) {
			cur.VerifiedSessions = append(cur.VerifiedSessions, sessionID)
			cur.Verification = append(cur.Verification, PatternVerification{SessionID: sessionID, Outcome: "passed", At: now})
		}
		cur.Confidence = maxFloat(cur.Confidence, p.Confidence)
		cur.ExplicitWorkflow = cur.ExplicitWorkflow || p.ExplicitWorkflow
		cur.LastObservedAt = now
		if p.Summary != "" {
			cur.Summary = p.Summary
		}
		cur.Evidence = compactStrings(append(cur.Evidence, p.Evidence...), 6, 240)
		if cur.Status == "" {
			cur.Status = "pending"
		}
		if err := s.writeState(st); err != nil {
			return Pattern{}, err
		}
		return *cur, nil
	}
	p.Occurrences = 1
	p.SessionIDs = []string{sessionID}
	p.FirstObservedAt = now
	p.LastObservedAt = now
	if p.Status == "" {
		p.Status = "pending"
	}
	if verified {
		p.VerifiedSessions = []string{sessionID}
		p.Verification = []PatternVerification{{SessionID: sessionID, Outcome: "passed", At: now}}
	}
	st.Patterns = append(st.Patterns, p)
	if err := s.writeState(st); err != nil {
		return Pattern{}, err
	}
	return p, nil
}

func (s *Store) ListPatterns() ([]Pattern, error) {
	st, err := s.readState()
	return st.Patterns, err
}

func (s *Store) SetPatternStatus(fingerprint, vendor, status string) error {
	st, err := s.readState()
	if err != nil {
		return err
	}
	vendor = harness.NormalizeVendorKind(vendor)
	for i := range st.Patterns {
		if st.Patterns[i].Fingerprint == fingerprint && st.Patterns[i].Vendor == vendor {
			st.Patterns[i].Status = status
			return s.writeState(st)
		}
	}
	return nil
}

// RemoveSessionReferences prevents retention from leaving pointers to deleted
// session artifacts. Aggregate occurrence counts and redacted summaries remain
// durable so previously learned recurrence is not forgotten.
func (s *Store) RemoveSessionReferences(sessionID string) error {
	st, err := s.readState()
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	changed := false
	for i := range st.Patterns {
		before := len(st.Patterns[i].SessionIDs) + len(st.Patterns[i].VerifiedSessions) + len(st.Patterns[i].Verification)
		st.Patterns[i].SessionIDs = withoutString(st.Patterns[i].SessionIDs, sessionID)
		st.Patterns[i].VerifiedSessions = withoutString(st.Patterns[i].VerifiedSessions, sessionID)
		verification := st.Patterns[i].Verification[:0]
		for _, v := range st.Patterns[i].Verification {
			if v.SessionID != sessionID {
				verification = append(verification, v)
			}
		}
		st.Patterns[i].Verification = verification
		after := len(st.Patterns[i].SessionIDs) + len(st.Patterns[i].VerifiedSessions) + len(st.Patterns[i].Verification)
		changed = changed || before != after
	}
	if !changed {
		return nil
	}
	return s.writeState(st)
}

func (s *Store) ActivePath() string {
	return filepath.Join(s.Paths.MemoryDir, "context.md")
}

var injectionRe = regexp.MustCompile(`(?i)(ignore (all )?(previous|prior) (instructions|rules)|system\s*:|<\s*/?\s*system\s*>|do not follow|disregard (the )?above)`)

func ContainsInjection(text string) bool {
	return injectionRe.MatchString(text)
}

func (s *Store) AddLesson(l Lesson, mode Mode) error {
	if mode == ModeTemporary || mode == ModeIncognito {
		return fmt.Errorf("memory mode %s blocks writes", mode)
	}
	if err := s.Ensure(); err != nil {
		return err
	}
	text := strings.TrimSpace(l.Text)
	if text == "" {
		return fmt.Errorf("empty lesson")
	}
	if ContainsInjection(text) {
		_ = audit.Append(s.Paths, audit.Event{
			Action: "injection_blocked", Type: "lesson", Detail: truncate(text, 120),
		})
		return fmt.Errorf("lesson blocked by injection screen")
	}
	if l.ID == "" {
		sum := sha1.Sum([]byte(strings.ToLower(text)))
		l.ID = "lesson_" + hex.EncodeToString(sum[:8])
	}
	if l.Scope == "" {
		l.Scope = "workspace"
	}
	if l.Confidence == 0 {
		l.Confidence = 1
	}
	if l.CreatedAt.IsZero() {
		l.CreatedAt = time.Now().UTC()
	}
	st, err := s.readState()
	if err != nil {
		return err
	}
	st.Lessons = append(st.Lessons, l)
	if err := s.writeState(st); err != nil {
		return err
	}
	_ = audit.Append(s.Paths, audit.Event{
		Action: "create", Type: "lesson", Key: l.ID, Detail: truncate(l.Text, 160),
		Session: l.SourceSession,
	})
	_ = s.exportLessonsMarkdown()
	_, _ = s.RefreshActive("")
	return nil
}

func (s *Store) ListLessons() ([]Lesson, error) {
	st, err := s.readState()
	return st.Lessons, err
}

// DeleteLesson removes a lesson by id and refreshes lessons.md.
func (s *Store) DeleteLesson(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("empty lesson id")
	}
	lessons, err := s.ListLessons()
	if err != nil {
		return err
	}
	var kept []Lesson
	found := false
	for _, l := range lessons {
		if l.ID == id {
			found = true
			continue
		}
		kept = append(kept, l)
	}
	if !found {
		return fmt.Errorf("lesson not found: %s", id)
	}
	st, err := s.readState()
	if err != nil {
		return err
	}
	st.Lessons = kept
	if err := s.writeState(st); err != nil {
		return err
	}
	_ = audit.Append(s.Paths, audit.Event{Action: "delete", Type: "lesson", Key: id})
	if err := s.exportLessonsMarkdown(); err != nil {
		return err
	}
	_, _ = s.RefreshActive("")
	return nil
}

// UpdateLesson replaces lesson text by id.
func (s *Store) UpdateLesson(id, text string) error {
	id = strings.TrimSpace(id)
	text = strings.TrimSpace(text)
	if id == "" || text == "" {
		return fmt.Errorf("id and text required")
	}
	if ContainsInjection(text) {
		return fmt.Errorf("lesson blocked by injection screen")
	}
	lessons, err := s.ListLessons()
	if err != nil {
		return err
	}
	found := false
	for i := range lessons {
		if lessons[i].ID == id {
			lessons[i].Text = text
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("lesson not found: %s", id)
	}
	st, err := s.readState()
	if err != nil {
		return err
	}
	st.Lessons = lessons
	if err := s.writeState(st); err != nil {
		return err
	}
	_ = audit.Append(s.Paths, audit.Event{Action: "update", Type: "lesson", Key: id, Detail: truncate(text, 160)})
	if err := s.exportLessonsMarkdown(); err != nil {
		return err
	}
	_, _ = s.RefreshActive("")
	return nil
}

func (s *Store) UpsertSemantic(e SemanticEntry, mode Mode) error {
	if mode != ModePersistent {
		return fmt.Errorf("memory mode %s blocks writes", mode)
	}
	if ContainsInjection(e.Value) || ContainsInjection(e.Key) {
		_ = audit.Append(s.Paths, audit.Event{Action: "injection_blocked", Type: "semantic", Key: e.Key})
		return fmt.Errorf("semantic entry blocked by injection screen")
	}
	if err := s.Ensure(); err != nil {
		return err
	}
	e.UpdatedAt = time.Now().UTC()
	existing, _ := s.ListSemantic()
	var kept []SemanticEntry
	for _, x := range existing {
		if x.Key != e.Key {
			kept = append(kept, x)
		}
	}
	kept = append(kept, e)
	st, err := s.readState()
	if err != nil {
		return err
	}
	st.Semantic = kept
	if err := s.writeState(st); err != nil {
		return err
	}
	_ = audit.Append(s.Paths, audit.Event{Action: "update", Type: "semantic", Key: e.Key, Detail: truncate(e.Value, 120)})
	return nil
}

func (s *Store) ListSemantic() ([]SemanticEntry, error) {
	st, err := s.readState()
	return st.Semantic, err
}

func (s *Store) AddEpisodic(e EpisodicEntry, mode Mode) error {
	if mode != ModePersistent {
		return fmt.Errorf("memory mode %s blocks writes", mode)
	}
	if ContainsInjection(e.Text) {
		_ = audit.Append(s.Paths, audit.Event{Action: "injection_blocked", Type: "episodic", Detail: truncate(e.Text, 120)})
		return fmt.Errorf("episodic entry blocked by injection screen")
	}
	if err := s.Ensure(); err != nil {
		return err
	}
	if e.ID == "" {
		sum := sha1.Sum([]byte(e.Text + time.Now().String()))
		e.ID = "ep_" + hex.EncodeToString(sum[:8])
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	st, err := s.readState()
	if err != nil {
		return err
	}
	st.Episodic = append(st.Episodic, e)
	return s.writeState(st)
}

func (s *Store) ListEpisodic() ([]EpisodicEntry, error) {
	st, err := s.readState()
	return st.Episodic, err
}

func (s *Store) AppendHistory(summary string) error {
	if err := s.Ensure(); err != nil {
		return err
	}
	st, err := s.readState()
	if err != nil {
		return err
	}
	st.History = append(st.History, fmt.Sprintf("### %s\n%s", time.Now().UTC().Format(time.RFC3339), strings.TrimSpace(summary)))
	if len(st.History) > 100 {
		st.History = st.History[len(st.History)-100:]
	}
	return s.writeState(st)
}

func (s *Store) Search(q string, limit int) ([]SearchHit, error) {
	q = strings.TrimSpace(strings.ToLower(q))
	if q == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	var hits []SearchHit
	score := func(text string) float64 {
		t := strings.ToLower(text)
		if !strings.Contains(t, q) {
			// token overlap
			var n float64
			for _, tok := range strings.Fields(q) {
				if len(tok) > 2 && strings.Contains(t, tok) {
					n++
				}
			}
			return n
		}
		return 3 + float64(strings.Count(t, q))
	}
	add := func(kind, id, snippet string, sc float64) {
		if sc <= 0 {
			return
		}
		hits = append(hits, SearchHit{Kind: kind, ID: id, Snippet: truncate(snippet, 240), Score: sc})
	}
	if lessons, err := s.ListLessons(); err == nil {
		for _, l := range lessons {
			add("lesson", l.ID, l.Text, score(l.Text))
		}
	}
	if sem, err := s.ListSemantic(); err == nil {
		for _, e := range sem {
			add("semantic", e.Key, e.Key+": "+e.Value, score(e.Key+" "+e.Value))
		}
	}
	if eps, err := s.ListEpisodic(); err == nil {
		for _, e := range eps {
			add("episodic", e.ID, e.Text, score(e.Text))
		}
	}
	if st, err := s.readState(); err == nil {
		add("preferences", "preferences", st.Preferences, score(st.Preferences))
		add("projects", "projects", st.Projects, score(st.Projects))
		for i, h := range st.History {
			add("history", fmt.Sprintf("history-%d", i), h, score(h))
		}
	}
	// sort by score desc
	for i := 0; i < len(hits); i++ {
		for j := i + 1; j < len(hits); j++ {
			if hits[j].Score > hits[i].Score {
				hits[i], hits[j] = hits[j], hits[i]
			}
		}
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

// BuildSessionContext assembles a capped memory pack for SessionStart / start-from-memory.
func (s *Store) BuildSessionContext(budget int, query string, mode Mode) (ContextPack, error) {
	if err := s.Ensure(); err != nil {
		return ContextPack{}, err
	}
	if budget <= 0 {
		budget = 12000
	}
	pack := ContextPack{Mode: mode, Sections: map[string]int{}, ActivePath: s.ActivePath()}
	if mode == ModeTemporary {
		pack.Text = "<!-- Superopen generated session context. This is derived from approved memory and project guidance and may be regenerated. -->\n<!-- Updated by session review and memory consolidation. -->\n# Superopen memory\n\n(temporary mode - no memory injected)\n"
		pack.CharCount = len(pack.Text)
		_ = os.WriteFile(s.ActivePath(), []byte(pack.Text), 0o644)
		return pack, nil
	}

	var b strings.Builder
	b.WriteString("<!-- Superopen generated session context. This is derived from approved memory and project guidance and may be regenerated. -->\n")
	b.WriteString("<!-- Updated by session review and memory consolidation. -->\n")
	b.WriteString("# Superopen session memory\n\n")
	b.WriteString("Read this pack before exploring. Prefer `so graph query` / `AGENTS.md` for code structure.\n\n")

	writeSection := func(title, body string, capn int) {
		body = strings.TrimSpace(body)
		if body == "" {
			return
		}
		if len(body) > capn {
			body = body[:capn] + "\n…[truncated]"
		}
		sec := fmt.Sprintf("## [%s]\n\n%s\n\n", title, body)
		if b.Len()+len(sec) > budget {
			return
		}
		b.WriteString(sec)
		pack.Sections[title] = len(body)
	}

	st, _ := s.readState()
	writeSection("Preferences", st.Preferences, 2000)
	writeSection("Projects", st.Projects, 2500)
	writeSection("History", s.readRecentHistory(14), 2600)

	if sem, err := s.ListSemantic(); err == nil && len(sem) > 0 {
		var lines []string
		for _, e := range sem {
			if query != "" && !strings.Contains(strings.ToLower(e.Key+" "+e.Value), strings.ToLower(query)) {
				continue
			}
			lines = append(lines, fmt.Sprintf("- %s: %s", e.Key, e.Value))
			if len(lines) >= 20 {
				break
			}
		}
		if len(lines) == 0 {
			for i, e := range sem {
				lines = append(lines, fmt.Sprintf("- %s: %s", e.Key, e.Value))
				if i >= 15 {
					break
				}
			}
		}
		writeSection("Semantic Memory", strings.Join(lines, "\n"), 2000)
	}

	if query != "" {
		if hits, err := s.Search(query, 8); err == nil {
			var lines []string
			for _, h := range hits {
				if h.Kind == "episodic" || h.Kind == "lesson" {
					lines = append(lines, fmt.Sprintf("- (%s) %s", h.Kind, h.Snippet))
				}
			}
			writeSection("Episodic Memory", strings.Join(lines, "\n"), 1500)
		}
	} else if eps, err := s.ListEpisodic(); err == nil && len(eps) > 0 {
		var lines []string
		start := 0
		if len(eps) > 8 {
			start = len(eps) - 8
		}
		for _, e := range eps[start:] {
			lines = append(lines, "- "+truncate(e.Text, 280))
		}
		writeSection("Episodic Memory", strings.Join(lines, "\n"), 1500)
	}

	if lessons, err := s.ListLessons(); err == nil && len(lessons) > 0 {
		var lines []string
		start := 0
		if len(lessons) > 50 {
			start = len(lessons) - 50
		}
		for _, l := range lessons[start:] {
			lines = append(lines, "- "+truncate(l.Text, 200))
		}
		writeSection("Learned corrections", strings.Join(lines, "\n"), 3000)
	}

	// skills index
	if ents, err := os.ReadDir(s.Paths.SkillsDir); err == nil {
		var lines []string
		for _, e := range ents {
			name := e.Name()
			if strings.HasSuffix(name, ".md") || e.IsDir() {
				lines = append(lines, "- "+name)
			}
		}
		writeSection("Skills", strings.Join(lines, "\n"), 800)
	}

	pack.Text = b.String()
	pack.CharCount = len(pack.Text)
	_ = os.WriteFile(s.ActivePath(), []byte(pack.Text), 0o644)
	return pack, nil
}

func (s *Store) readRecentHistory(days int) string {
	st, err := s.readState()
	if err != nil {
		return ""
	}
	start := 0
	if len(st.History) > days {
		start = len(st.History) - days
	}
	return strings.Join(st.History[start:], "\n\n")
}

func firstEntry(content string) string {
	parts := strings.Split(content, "### ")
	if len(parts) < 2 {
		return truncate(content, 400)
	}
	return truncate("### "+parts[1], 400)
}

// Consolidate updates the consolidated state, then rebuilds context.md.
func (s *Store) Consolidate(sessionSummary string, completer llm.Completer) (hint string, err error) {
	if err := s.Ensure(); err != nil {
		return "", err
	}
	summary := strings.TrimSpace(sessionSummary)
	if summary == "" {
		summary = "manual consolidate"
	}
	_ = s.AppendHistory(summary)

	if completer == nil || !completer.Available() {
		if summary != "" {
			st, _ := s.readState()
			updated := harnessvalid.AppendToProjectsSection(st.Projects, "Notes",
				fmt.Sprintf("%s - %s", time.Now().UTC().Format("2006-01-02"), truncate(summary, 200)))
			st.Projects = updated
			_ = s.writeState(st)
			s.heuristicSemanticFromSummary(summary)
		}
		_, _ = s.BuildSessionContext(12000, "", ModePersistent)
		return "Install/login Claude Code or Codex for richer consolidation, or set an API key.", nil
	}
	st, _ := s.readState()
	prefs, projects := []byte(st.Preferences), []byte(st.Projects)
	out, err := completer.Complete(
		`You update Superopen memory. Reply JSON only:
{"preferences_md":"...","projects_md":"...","semantic":[{"key":"...","value":"..."}]}
Keep preferences/projects faithful; invent no secrets. semantic: 3-8 short durable facts from the session.
preferences_md MUST start with "# Preferences" and use bullet lists only (no second H1).
projects_md MUST start with "# Projects" and include H2 sections exactly: "## Current focus", "## Active areas", "## Do not touch", "## Notes".`,
		fmt.Sprintf("Current preferences.md:\n%s\n\nCurrent projects.md:\n%s\n\nSession summary:\n%s", prefs, projects, summary),
	)
	if err != nil {
		s.heuristicSemanticFromSummary(summary)
		_, _ = s.BuildSessionContext(12000, "", ModePersistent)
		return "consolidation model error: " + err.Error(), nil
	}
	raw := llm.ExtractJSON(out)
	var parsed struct {
		PreferencesMD string `json:"preferences_md"`
		ProjectsMD    string `json:"projects_md"`
		Semantic      []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"semantic"`
	}
	if json.Unmarshal([]byte(raw), &parsed) != nil {
		s.heuristicSemanticFromSummary(summary)
		_, _ = s.BuildSessionContext(12000, "", ModePersistent)
		return "consolidation parse failed; kept heuristic history only", nil
	}
	if strings.TrimSpace(parsed.PreferencesMD) != "" {
		if harnessvalid.ValidatePreferences(parsed.PreferencesMD) != nil {
			_ = audit.Append(s.Paths, audit.Event{Action: "memory.structure_reject", Type: "memory", Detail: "preferences"})
		} else {
			st.Preferences = harnessvalid.NormalizePreferences(parsed.PreferencesMD)
		}
	}
	if strings.TrimSpace(parsed.ProjectsMD) != "" {
		norm := harnessvalid.NormalizeProjects(parsed.ProjectsMD)
		if harnessvalid.ValidateProjects(norm) != nil {
			_ = audit.Append(s.Paths, audit.Event{Action: "memory.structure_reject", Type: "memory", Detail: "projects"})
		} else {
			st.Projects = norm
		}
	}
	_ = s.writeState(st)
	n := 0
	for _, e := range parsed.Semantic {
		k := strings.TrimSpace(e.Key)
		v := strings.TrimSpace(e.Value)
		if k == "" || v == "" {
			continue
		}
		_ = s.UpsertSemantic(SemanticEntry{Key: k, Value: v, Confidence: 0.8}, ModePersistent)
		n++
		if n >= 8 {
			break
		}
	}
	if n == 0 {
		s.heuristicSemanticFromSummary(summary)
	}
	_ = audit.Append(s.Paths, audit.Event{Action: "update", Type: "memory", Detail: "consolidate via " + completer.Backend()})
	_, _ = s.BuildSessionContext(12000, "", ModePersistent)
	return "", nil
}

func (s *Store) heuristicSemanticFromSummary(summary string) {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return
	}
	// Pull short "Key: value" or "- key - value" style lines.
	for _, line := range strings.Split(summary, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
		line = strings.TrimSpace(line)
		if len(line) < 8 || len(line) > 200 {
			continue
		}
		var key, val string
		if i := strings.Index(line, ":"); i > 0 && i < 40 {
			key = strings.TrimSpace(line[:i])
			val = strings.TrimSpace(line[i+1:])
		} else if i := strings.Index(line, " - "); i > 0 && i < 40 {
			key = strings.TrimSpace(line[:i])
			val = strings.TrimSpace(line[i+len(" - "):])
		} else {
			continue
		}
		if key == "" || val == "" {
			continue
		}
		_ = s.UpsertSemantic(SemanticEntry{Key: truncate(key, 64), Value: truncate(val, 200), Confidence: 0.6}, ModePersistent)
	}
}

// Status summarizes memory health for doctor / UI.
type Status struct {
	PrefsStub     bool   `json:"prefs_stub"`
	ProjectsStub  bool   `json:"projects_stub"`
	StructureOK   bool   `json:"structure_ok"`
	LessonCount   int    `json:"lesson_count"`
	SemanticCount int    `json:"semantic_count"`
	ActiveBytes   int64  `json:"active_bytes"`
	ActivePath    string `json:"active_path"`
}

func (s *Store) Status() Status {
	_ = s.Ensure()
	st := Status{ActivePath: s.ActivePath(), StructureOK: true}
	mem, memErr := s.readState()
	if memErr == nil {
		st.PrefsStub = IsStubMarkdown(mem.Preferences)
		if harnessvalid.ValidatePreferences(mem.Preferences) != nil {
			st.StructureOK = false
		}
	} else {
		st.PrefsStub = true
		st.StructureOK = false
	}
	if memErr == nil {
		st.ProjectsStub = IsStubMarkdown(mem.Projects)
		if harnessvalid.ValidateProjects(mem.Projects) != nil {
			st.StructureOK = false
		}
	} else {
		st.ProjectsStub = true
		st.StructureOK = false
	}
	if lessons, err := s.ListLessons(); err == nil {
		st.LessonCount = len(lessons)
	}
	if sem, err := s.ListSemantic(); err == nil {
		st.SemanticCount = len(sem)
	}
	if info, err := os.Stat(s.ActivePath()); err == nil {
		st.ActiveBytes = info.Size()
	}
	return st
}

func (s *Store) exportLessonsMarkdown() error {
	return nil
}

func (s *Store) readState() (stateFile, error) {
	var st stateFile
	data, err := os.ReadFile(s.statePath())
	if err != nil {
		return st, err
	}
	err = json.Unmarshal(data, &st)
	return st, err
}

func (s *Store) writeState(st stateFile) error {
	st.About = memoryAbout
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Paths.MemoryDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.statePath(), append(b, '\n'), 0o644)
}

func (s *Store) AppendPreferenceText(text string) error {
	if err := s.Ensure(); err != nil {
		return err
	}
	st, err := s.readState()
	if err != nil {
		return err
	}
	updated := harnessvalid.AppendPreferencesBullet(st.Preferences, strings.TrimSpace(text))
	if err := harnessvalid.ValidatePreferences(updated); err != nil {
		return err
	}
	st.Preferences = updated
	return s.writeState(st)
}

func (s *Store) AppendProjectNote(text string) error {
	if err := s.Ensure(); err != nil {
		return err
	}
	st, err := s.readState()
	if err != nil {
		return err
	}
	updated := harnessvalid.AppendToProjectsSection(st.Projects, "Notes", strings.TrimSpace(text))
	if err := harnessvalid.ValidateProjects(updated); err != nil {
		return err
	}
	st.Projects = updated
	return s.writeState(st)
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func compactStrings(values []string, limit, maxLen int) []string {
	seen := map[string]bool{}
	out := make([]string, 0, limit)
	for _, value := range values {
		value = truncate(value, maxLen)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
		if len(out) == limit {
			break
		}
	}
	return out
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func withoutString(values []string, needle string) []string {
	out := values[:0]
	for _, value := range values {
		if value != needle {
			out = append(out, value)
		}
	}
	return out
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
