package memory

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

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

func NewStore(paths harness.Paths) *Store {
	return &Store{Paths: paths}
}

func (s *Store) Ensure() error {
	dirs := []string{
		s.Paths.MemoryDir,
		filepath.Join(s.Paths.MemoryDir, "history"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	if err := s.SeedFromTemplates(); err != nil {
		return err
	}
	_ = s.repairMemoryStructure()
	_ = s.migrateLessonsMD()
	return nil
}

// repairMemoryStructure normalizes preferences/projects without wiping bullets.
func (s *Store) repairMemoryStructure() error {
	prefsPath := filepath.Join(s.Paths.MemoryDir, "preferences.md")
	if data, err := os.ReadFile(prefsPath); err == nil {
		raw := string(data)
		if harnessvalid.ValidatePreferences(raw) != nil {
			_ = os.WriteFile(prefsPath, []byte(harnessvalid.NormalizePreferences(raw)), 0o644)
		}
	}
	projPath := filepath.Join(s.Paths.MemoryDir, "projects.md")
	if data, err := os.ReadFile(projPath); err == nil {
		raw := string(data)
		if harnessvalid.ValidateProjects(raw) != nil {
			_ = os.WriteFile(projPath, []byte(harnessvalid.NormalizeProjects(raw)), 0o644)
		}
	}
	return nil
}

func (s *Store) lessonsPath() string {
	return filepath.Join(s.Paths.MemoryDir, "lessons.jsonl")
}

func (s *Store) semanticPath() string {
	return filepath.Join(s.Paths.MemoryDir, "semantic.jsonl")
}

func (s *Store) episodicPath() string {
	return filepath.Join(s.Paths.MemoryDir, "episodic.jsonl")
}

func (s *Store) ActivePath() string {
	return filepath.Join(s.Paths.MemoryDir, "active-context.md")
}

func (s *Store) migrateLessonsMD() error {
	legacy := s.Paths.Lessons
	data, err := os.ReadFile(legacy)
	if err != nil || len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}
	if _, err := os.Stat(s.lessonsPath()); err == nil {
		return nil
	}
	text := strings.TrimSpace(string(data))
	if text == "" || text == "# Lessons" || strings.HasPrefix(text, "# Lessons\n\nApproved") {
		return nil
	}
	// Write directly - do not call AddLesson (which calls Ensure → recurse).
	l := Lesson{
		Text:       text,
		Scope:      "workspace",
		Confidence: 0.5,
		CreatedAt:  time.Now().UTC(),
	}
	sum := sha1.Sum([]byte(strings.ToLower(text)))
	l.ID = "lesson_" + hex.EncodeToString(sum[:8])
	return appendJSONL(s.lessonsPath(), l)
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
	if err := appendJSONL(s.lessonsPath(), l); err != nil {
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
	var out []Lesson
	err := readJSONL(s.lessonsPath(), func(raw []byte) {
		var l Lesson
		if json.Unmarshal(raw, &l) == nil && l.Text != "" {
			out = append(out, l)
		}
	})
	return out, err
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
	if err := rewriteJSONL(s.lessonsPath(), kept); err != nil {
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
	if err := rewriteJSONL(s.lessonsPath(), lessons); err != nil {
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
	if err := rewriteJSONL(s.semanticPath(), kept); err != nil {
		return err
	}
	_ = audit.Append(s.Paths, audit.Event{Action: "update", Type: "semantic", Key: e.Key, Detail: truncate(e.Value, 120)})
	return nil
}

func (s *Store) ListSemantic() ([]SemanticEntry, error) {
	var out []SemanticEntry
	err := readJSONL(s.semanticPath(), func(raw []byte) {
		var e SemanticEntry
		if json.Unmarshal(raw, &e) == nil && e.Key != "" {
			out = append(out, e)
		}
	})
	return out, err
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
	return appendJSONL(s.episodicPath(), e)
}

func (s *Store) ListEpisodic() ([]EpisodicEntry, error) {
	var out []EpisodicEntry
	err := readJSONL(s.episodicPath(), func(raw []byte) {
		var e EpisodicEntry
		if json.Unmarshal(raw, &e) == nil && e.Text != "" {
			out = append(out, e)
		}
	})
	return out, err
}

func (s *Store) AppendHistory(summary string) error {
	if err := s.Ensure(); err != nil {
		return err
	}
	day := time.Now().UTC().Format("2006-01-02")
	p := filepath.Join(s.Paths.MemoryDir, "history", day+".md")
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "\n### %s\n%s\n", time.Now().UTC().Format(time.RFC3339), strings.TrimSpace(summary))
	return err
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
	for _, name := range []string{"preferences.md", "projects.md"} {
		if data, err := os.ReadFile(filepath.Join(s.Paths.MemoryDir, name)); err == nil {
			add(strings.TrimSuffix(name, ".md"), name, string(data), score(string(data)))
		}
	}
	_ = filepath.WalkDir(filepath.Join(s.Paths.MemoryDir, "history"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, _ := os.ReadFile(path)
		add("history", filepath.Base(path), string(data), score(string(data)))
		return nil
	})
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
		pack.Text = "# Superopen memory\n\n(temporary mode - no memory injected)\n"
		pack.CharCount = len(pack.Text)
		_ = os.WriteFile(s.ActivePath(), []byte(pack.Text), 0o644)
		return pack, nil
	}

	var b strings.Builder
	b.WriteString("# Superopen session memory\n\n")
	b.WriteString("Read this pack before exploring. Prefer `so graph query` / `.so/knowledge` for code structure.\n\n")

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

	prefs, _ := os.ReadFile(filepath.Join(s.Paths.MemoryDir, "preferences.md"))
	writeSection("Preferences", string(prefs), 2000)
	projects, _ := os.ReadFile(filepath.Join(s.Paths.MemoryDir, "projects.md"))
	writeSection("Projects", string(projects), 2500)
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
	var parts []string
	now := time.Now().UTC()
	for i := 0; i < 181; i++ {
		day := now.AddDate(0, 0, -i)
		p := filepath.Join(s.Paths.MemoryDir, "history", day.Format("2006-01-02")+".md")
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			continue
		}
		header := "#### " + day.Format("2006-01-02")
		switch {
		case i < days:
			parts = append(parts, header+"\n"+content)
		case i < 61:
			first := firstEntry(content)
			n := strings.Count(content, "### ")
			parts = append(parts, fmt.Sprintf("%s\n%s\n…%d more entries", header, first, max(0, n-1)))
		case i < 181:
			n := strings.Count(content, "### ")
			parts = append(parts, fmt.Sprintf("%s - %d entries", header, n))
		}
	}
	return strings.Join(parts, "\n\n")
}

func firstEntry(content string) string {
	parts := strings.Split(content, "### ")
	if len(parts) < 2 {
		return truncate(content, 400)
	}
	return truncate("### "+parts[1], 400)
}

// Consolidate updates prefs/projects/history/semantic, then rebuilds active-context.md.
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
			p := filepath.Join(s.Paths.MemoryDir, "projects.md")
			existing, _ := os.ReadFile(p)
			updated := harnessvalid.AppendToProjectsSection(string(existing), "Notes",
				fmt.Sprintf("%s - %s", time.Now().UTC().Format("2006-01-02"), truncate(summary, 200)))
			_ = os.WriteFile(p, []byte(updated), 0o644)
			s.heuristicSemanticFromSummary(summary)
		}
		_, _ = s.BuildSessionContext(12000, "", ModePersistent)
		return "Install/login Claude Code or Codex for richer consolidation, or set an API key.", nil
	}
	prefs, _ := os.ReadFile(filepath.Join(s.Paths.MemoryDir, "preferences.md"))
	projects, _ := os.ReadFile(filepath.Join(s.Paths.MemoryDir, "projects.md"))
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
			_ = os.WriteFile(filepath.Join(s.Paths.MemoryDir, "preferences.md"), []byte(harnessvalid.NormalizePreferences(parsed.PreferencesMD)), 0o644)
		}
	}
	if strings.TrimSpace(parsed.ProjectsMD) != "" {
		norm := harnessvalid.NormalizeProjects(parsed.ProjectsMD)
		if harnessvalid.ValidateProjects(norm) != nil {
			_ = audit.Append(s.Paths, audit.Event{Action: "memory.structure_reject", Type: "memory", Detail: "projects"})
		} else {
			_ = os.WriteFile(filepath.Join(s.Paths.MemoryDir, "projects.md"), []byte(norm), 0o644)
		}
	}
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
	if data, err := os.ReadFile(filepath.Join(s.Paths.MemoryDir, "preferences.md")); err == nil {
		st.PrefsStub = IsStubMarkdown(string(data))
		if harnessvalid.ValidatePreferences(string(data)) != nil {
			st.StructureOK = false
		}
	} else {
		st.PrefsStub = true
		st.StructureOK = false
	}
	if data, err := os.ReadFile(filepath.Join(s.Paths.MemoryDir, "projects.md")); err == nil {
		st.ProjectsStub = IsStubMarkdown(string(data))
		if harnessvalid.ValidateProjects(string(data)) != nil {
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
	lessons, err := s.ListLessons()
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# Lessons\n\n")
	for _, l := range lessons {
		b.WriteString(fmt.Sprintf("## %s (%s)\n%s\n\n", l.ID, l.CreatedAt.Format(time.RFC3339), l.Text))
	}
	return os.WriteFile(s.Paths.Lessons, []byte(b.String()), 0o644)
}

func appendJSONL(path string, v any) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(v)
}

func rewriteJSONL[T any](path string, items []T) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, it := range items {
		if err := enc.Encode(it); err != nil {
			return err
		}
	}
	return nil
}

func readJSONL(path string, fn func([]byte)) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		cp := append([]byte(nil), line...)
		fn(cp)
	}
	return sc.Err()
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
