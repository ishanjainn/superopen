package session

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/artifactmeta"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/llm"
	"github.com/ishanjainn/superopen/internal/otlp"
	"github.com/ishanjainn/superopen/internal/redact"
	"github.com/ishanjainn/superopen/internal/session/agentlinks"
	"github.com/ishanjainn/superopen/internal/tracestore"
)

type Status string

const (
	StatusActive Status = "active"
	StatusEnded  Status = "ended"
)

// Meta is stored in .so/sessions/<id>/session.json.
type Meta struct {
	ID            string     `json:"id"`
	Vendor        string     `json:"vendor"`
	Model         string     `json:"model,omitempty"`
	User          string     `json:"user,omitempty"`  // gen_ai.user.name (email / identity)
	Title         string     `json:"title,omitempty"` // AI/vendor display name
	PromptPreview string     `json:"prompt_preview,omitempty"`
	Status        Status     `json:"status"`
	StartedAt     time.Time  `json:"started_at"`
	EndedAt       *time.Time `json:"ended_at,omitempty"`
	DurationMs    int64      `json:"duration_ms,omitempty"`
	Tokens        int64      `json:"tokens,omitempty"`
	CostUSD       float64    `json:"cost_usd,omitempty"`
	EvalBadge     string     `json:"eval_badge,omitempty"`
	ParentID      string     `json:"parent_id,omitempty"`
	IsSubagent    bool       `json:"is_subagent,omitempty"`

	// VCS / join fields (materialized from spans + optional git trailers).
	ProjectID    string              `json:"project_id,omitempty"`
	RepoRoot     string              `json:"repo_root,omitempty"`
	Branch       string              `json:"branch,omitempty"`
	BaseSHA      string              `json:"base_sha,omitempty"`
	HeadSHA      string              `json:"head_sha,omitempty"`
	Commits      []CommitRef         `json:"commits,omitempty"`
	PullRequests []PRRef             `json:"pull_requests,omitempty"`
	Attribution  *AttributionSummary `json:"attribution,omitempty"`
	Summary      string              `json:"summary,omitempty"`
}

type FootprintFile struct {
	Path  string `json:"path"`
	State string `json:"state"` // seen | read | edited
	Count int    `json:"count"`
}

type Footprint struct {
	Files []FootprintFile `json:"files"`
}

type IndexEntry = Meta

type ReviewState struct {
	Status               string          `json:"status,omitempty"`
	Trigger              string          `json:"trigger,omitempty"`
	Backend              string          `json:"backend,omitempty"`
	StartedAt            *time.Time      `json:"started_at,omitempty"`
	CompletedAt          *time.Time      `json:"completed_at,omitempty"`
	Error                string          `json:"error,omitempty"`
	HarvestedAt          *time.Time      `json:"harvested_at,omitempty"`
	HarvestedSourceMtime int64           `json:"harvested_source_mtime,omitempty"`
	HarvestTrigger       string          `json:"harvest_trigger,omitempty"`
	Findings             []ReviewFinding `json:"findings,omitempty"`
}

// ReviewFinding is compact, redacted evidence from one session review. It
// intentionally excludes proposed file bodies; actionable content belongs to
// the recommendation record and cross-session counters belong to memory.
type ReviewFinding struct {
	Fingerprint      string   `json:"fingerprint"`
	Kind             string   `json:"kind"`
	ChangeKind       string   `json:"change_kind,omitempty"`
	Summary          string   `json:"summary"`
	Vendor           string   `json:"vendor"`
	TargetType       string   `json:"target_type,omitempty"`
	TargetPath       string   `json:"target_path,omitempty"`
	Confidence       float64  `json:"confidence,omitempty"`
	Verified         bool     `json:"verified,omitempty"`
	ExplicitWorkflow bool     `json:"explicit_workflow,omitempty"`
	Evidence         []string `json:"evidence,omitempty"`
	EventIDs         []string `json:"event_ids,omitempty"`
	Keywords         []string `json:"keywords,omitempty"`
	Paths            []string `json:"paths,omitempty"`
	Symbols          []string `json:"symbols,omitempty"`
	ErrorSignatures  []string `json:"error_signatures,omitempty"`
	Applicability    string   `json:"applicability,omitempty"`
}

type MemoryRetrieval struct {
	PatternIDs      []string  `json:"pattern_ids"`
	Scores          []string  `json:"scores,omitempty"`
	Reasons         []string  `json:"selection_reasons,omitempty"`
	TargetPaths     []string  `json:"target_paths,omitempty"`
	EstimatedTokens int64     `json:"estimated_tokens"`
	TurnID          string    `json:"turn_id,omitempty"`
	Delivery        string    `json:"delivery,omitempty"`
	At              time.Time `json:"at"`
}

type Document struct {
	About artifactmeta.About `json:"_about"`
	Meta
	Footprint        Footprint         `json:"footprint,omitempty"`
	Evaluation       json.RawMessage   `json:"evaluation,omitempty"`
	Recommendations  json.RawMessage   `json:"recommendations,omitempty"`
	Replay           json.RawMessage   `json:"replay,omitempty"`
	Port             json.RawMessage   `json:"port,omitempty"`
	Review           ReviewState       `json:"review,omitempty"`
	MemoryRetrievals []MemoryRetrieval `json:"memory_retrievals,omitempty"`
}

type indexFile struct {
	About         artifactmeta.About `json:"_about"`
	Sessions      []Meta             `json:"sessions"`
	PendingSpawns json.RawMessage    `json:"pending_spawns,omitempty"`
	Links         json.RawMessage    `json:"links,omitempty"`
}

var sessionAbout = artifactmeta.About{
	Purpose:   "Materialized state, summary, footprint, review, recommendations, and replay metadata for one coding session.",
	Authority: "authoritative session state derived from events.jsonl",
	UpdatedBy: "session materializer and review worker",
}

var indexAbout = artifactmeta.About{
	Purpose:   "Rebuildable catalog of sessions, parent-child links, pending reviews, and the latest session for each vendor.",
	Authority: "derived from session.json files with temporary coordination state",
	UpdatedBy: "session ingestion and review workers",
}

// Store manages session materialization under .so/sessions/.
type Store struct {
	Paths harness.Paths
}

func NewStore(paths harness.Paths) *Store {
	return &Store{Paths: paths}
}

// Ensure creates the self-describing empty session catalog. Individual
// session directories remain event-driven.
func (s *Store) Ensure() error {
	if _, err := os.Stat(s.Paths.SessionsIndex); err == nil {
		// Continue below to repair any event stream that predates its
		// materialized session document.
	} else if !os.IsNotExist(err) {
		return err
	} else if err := writeJSON(s.Paths.SessionsIndex, indexFile{About: indexAbout, Sessions: []Meta{}}); err != nil {
		return err
	}
	entries, err := os.ReadDir(s.Paths.SessionsDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		id := entry.Name()
		if _, err := os.Stat(filepath.Join(s.Paths.SessionDir(id), "session.json")); err == nil {
			// session.json is authoritative and index.json is rebuildable. Repair
			// an interrupted/migrated catalog instead of leaving the session hidden
			// from CLI consumers.
			if meta, getErr := s.Get(id); getErr == nil {
				if err := s.upsertIndex(meta); err != nil {
					return err
				}
			}
			continue
		}
		events := filepath.Join(s.Paths.SessionDir(id), "events.jsonl")
		info, err := os.Stat(events)
		if err != nil {
			continue
		}
		meta := Meta{ID: id, Status: StatusActive, StartedAt: info.ModTime().UTC()}
		if data, readErr := os.ReadFile(events); readErr == nil {
			for _, line := range strings.Split(string(data), "\n") {
				var row map[string]any
				if json.Unmarshal([]byte(line), &row) != nil || row["type"] == "superopen.file_manifest" {
					continue
				}
				if vendor, _ := row["vendor"].(string); vendor != "" {
					meta.Vendor = vendor
				}
				if at, _ := row["at"].(string); at != "" {
					if parsed, parseErr := time.Parse(time.RFC3339Nano, at); parseErr == nil {
						meta.StartedAt = parsed
					}
				}
				break
			}
		}
		if err := s.Start(meta); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) List() ([]IndexEntry, error) {
	data, err := os.ReadFile(s.Paths.SessionsIndex)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var idx indexFile
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}
	// The catalog is derived. Merge authoritative session documents on every
	// read so a stale/interrupted concurrent index write cannot hide sessions.
	byID := make(map[string]Meta, len(idx.Sessions))
	for _, meta := range idx.Sessions {
		byID[meta.ID] = meta
	}
	if dirs, readErr := os.ReadDir(s.Paths.SessionsDir); readErr == nil {
		for _, dir := range dirs {
			if !dir.IsDir() || strings.HasPrefix(dir.Name(), ".") {
				continue
			}
			if meta, getErr := s.Get(dir.Name()); getErr == nil && meta.ID != "" {
				byID[meta.ID] = meta
			}
		}
	}
	entries := make([]Meta, 0, len(byID))
	for _, meta := range byID {
		entries = append(entries, meta)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].StartedAt.After(entries[j].StartedAt)
	})
	return entries, nil
}

// ListItem is a sessions-index row enriched for the UI (checkpoints, files, search hits).
type ListItem struct {
	Meta
	Checkpoints int      `json:"checkpoints"` // restorable .so snapshots (git commit / finalize)
	Turns       int      `json:"turns"`       // user prompt markers in transcript
	Files       []string `json:"files,omitempty"`
	Match       string   `json:"match,omitempty"` // why this row matched a search query
	hasActivity bool
}

// ListDetailed returns top-level sessions (subagents nested under a parent
// are omitted - use Children to fetch them when viewing a parent chat).
func (s *Store) ListDetailed() ([]ListItem, error) {
	entries, err := s.List()
	if err != nil {
		return nil, err
	}
	links := agentlinks.LoadMap(s.Paths.SessionsDir)
	out := make([]ListItem, 0, len(entries))
	for _, e := range entries {
		if parent := s.resolveNestedParent(e, links); parent != "" {
			continue
		}
		item := s.enrich(e)
		if IsEmptyListItem(item) {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

// resolveNestedParent returns the parent id when e is a nested subagent,
// repairing session.json / agent-links when discovery finds a link that
// was not stamped yet (Cursor Task agents).
//
// Orphan is_subagent=true without parent_id is treated as poison (parent
// chats that emitted subagent-* spans on their own id) and cleared.
//
// False nesting under empty parent stubs (greedy pending-claim / test
// pollution) is cleared so Sessions does not go blank.
func (s *Store) resolveNestedParent(e Meta, links map[string]string) string {
	parent := e.ParentID
	if parent == "" {
		if e.IsSubagent {
			s.clearOrphanSubagent(e.ID)
		}
		if p := links[e.ID]; p != "" {
			parent = p
		} else if p := agentlinks.ResolveParent(s.Paths.SessionsDir, e.ID, e.Vendor); p != "" {
			parent = p
		}
	}
	if parent == "" {
		return ""
	}
	if s.isEmptyParentStub(parent) {
		s.clearFalseNesting(e.ID)
		return ""
	}
	if e.ParentID != parent || !e.IsSubagent {
		s.stampNested(e.ID, parent, e.Vendor)
	}
	return parent
}

func (s *Store) isEmptyParentStub(parentID string) bool {
	meta, err := s.Get(parentID)
	if err != nil {
		// Parent not on disk yet - keep nesting (normal Task race).
		return false
	}
	return IsEmptyListItem(s.enrich(meta))
}

func (s *Store) clearFalseNesting(id string) {
	_ = agentlinks.Unregister(s.Paths.SessionsDir, id)
	meta, err := s.Get(id)
	if err != nil {
		return
	}
	if meta.ParentID == "" && !meta.IsSubagent {
		return
	}
	meta.ParentID = ""
	meta.IsSubagent = false
	_ = s.Start(meta)
}

func (s *Store) clearOrphanSubagent(id string) {
	meta, err := s.Get(id)
	if err != nil || !meta.IsSubagent || meta.ParentID != "" {
		return
	}
	meta.IsSubagent = false
	_ = s.Start(meta)
}

func (s *Store) stampNested(id, parent, vendor string) {
	_ = agentlinks.Register(s.Paths.SessionsDir, id, parent, vendor, "list-repair")
	meta, err := s.Get(id)
	if err != nil {
		return
	}
	if meta.ParentID == parent && meta.IsSubagent {
		return
	}
	meta.ParentID = parent
	meta.IsSubagent = true
	_ = s.Start(meta)
}

// Children returns sessions whose parent_id matches the given parent chat id.
func (s *Store) Children(parentID string) ([]ListItem, error) {
	if parentID == "" {
		return nil, nil
	}
	entries, err := s.List()
	if err != nil {
		return nil, err
	}
	out := make([]ListItem, 0)
	for _, e := range entries {
		if e.ParentID != parentID {
			continue
		}
		out = append(out, s.enrich(e))
	}
	return out, nil
}

func isNestedSession(m Meta) bool {
	// Require a real parent. Lone is_subagent flags poison parent chats
	// that emit coding_agent.agent.type=subagent spans on their own id.
	return m.ParentID != ""
}

// Search filters sessions by title, changed files, or chat transcript text.
func (s *Store) Search(q string) ([]ListItem, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return s.ListDetailed()
	}
	needle := strings.ToLower(q)
	entries, err := s.List()
	if err != nil {
		return nil, err
	}
	links := agentlinks.LoadMap(s.Paths.SessionsDir)
	out := make([]ListItem, 0)
	for _, e := range entries {
		if s.resolveNestedParent(e, links) != "" {
			continue
		}
		item := s.enrich(e)
		if IsEmptyListItem(item) {
			continue
		}
		if hit := s.matchQuery(item, needle); hit != "" {
			item.Match = hit
			out = append(out, item)
		}
	}
	return out, nil
}

func (s *Store) enrich(meta Meta) ListItem {
	item := ListItem{Meta: meta}
	dir := s.Paths.SessionDir(meta.ID)

	if fp, err := s.GetFootprint(meta.ID); err == nil {
		for _, f := range fp.Files {
			item.Files = append(item.Files, f.Path)
		}
	}

	item.Turns, item.Model, item.hasActivity = s.scanTranscript(dir, meta.Model)
	if item.Model != "" && meta.Model == "" {
		item.Meta.Model = item.Model
	}
	item.Checkpoints = countCheckpointDirs(filepath.Join(dir, "checkpoints"))
	return item
}

func countCheckpointDirs(dir string) int {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range ents {
		if e.IsDir() {
			n++
		}
	}
	return n
}

func (s *Store) scanTranscript(dir, existingModel string) (turns int, model string, hasActivity bool) {
	model = existingModel
	f, err := os.Open(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		return 0, model, false
	}
	defer f.Close()
	spans := make([]tracestore.Span, 0)

	dec := json.NewDecoder(f)
	for {
		var sp tracestore.Span
		if err := dec.Decode(&sp); err != nil {
			break
		}
		spans = append(spans, sp)
		attrs := sp.Attributes
		name := strings.ToLower(sp.Name)
		if strings.Contains(name, "user_prompt") || strings.Contains(name, "user.prompt") {
			turns++
			continue
		}
		if attrs == nil {
			continue
		}
		if model == "" {
			if m := attrs["gen_ai.request.model"]; m != "" {
				model = m
			} else if m := attrs["gen_ai.response.model"]; m != "" {
				model = m
			}
		}
		if attrs["gen_ai.prompt"] != "" || attrs["gen_ai.content.prompt"] != "" {
			turns++
			continue
		}
		if raw := attrs["gen_ai.input.messages"]; raw != "" {
			low := strings.ToLower(raw)
			if strings.Contains(low, `"role":"user"`) || strings.Contains(low, `"role": "user"`) ||
				strings.Contains(low, `"role":"user_prompt"`) {
				turns++
			}
		}
	}
	if turns == 0 {
		for _, sp := range spans {
			name := strings.ToLower(sp.Name)
			if name == "coding_agent.llm.turn" || strings.Contains(name, "completion") {
				turns++
			}
		}
	}
	return turns, model, SpansHaveActivity(spans)
}

func (s *Store) matchQuery(item ListItem, needle string) string {
	if strings.Contains(strings.ToLower(item.Title), needle) {
		return "title"
	}
	if strings.Contains(strings.ToLower(item.PromptPreview), needle) {
		return "prompt"
	}
	if strings.Contains(strings.ToLower(item.ID), needle) {
		return "id"
	}
	if strings.Contains(strings.ToLower(item.Vendor), needle) {
		return "vendor"
	}
	if strings.Contains(strings.ToLower(item.Model), needle) {
		return "model"
	}
	if strings.Contains(strings.ToLower(item.User), needle) {
		return "user"
	}
	for _, path := range item.Files {
		base := strings.ToLower(filepath.Base(path))
		full := strings.ToLower(path)
		if strings.Contains(base, needle) || strings.Contains(full, needle) {
			return "file:" + filepath.Base(path)
		}
	}
	if hit := s.transcriptContains(item.ID, needle); hit != "" {
		return hit
	}
	return ""
}

func (s *Store) transcriptContains(id, needle string) string {
	f, err := os.Open(filepath.Join(s.Paths.SessionDir(id), "events.jsonl"))
	if err != nil {
		return ""
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	for {
		var sp struct {
			Name       string            `json:"name"`
			Attributes map[string]string `json:"attributes"`
		}
		if err := dec.Decode(&sp); err != nil {
			break
		}
		for _, key := range []string{
			"gen_ai.prompt",
			"gen_ai.content.prompt",
			"gen_ai.completion",
			"gen_ai.content.completion",
			"coding_agent.llm.thought.text",
			"coding_agent.shell.command",
			"coding_agent.file_path",
			"code.file.path",
			"gen_ai.input.messages",
			"gen_ai.output.messages",
		} {
			v := sp.Attributes[key]
			if v == "" {
				continue
			}
			if strings.Contains(strings.ToLower(v), needle) {
				snippet := truncate(v, 80)
				return "chat:" + snippet
			}
		}
	}
	return ""
}

// FillMissingTitles assigns vendor AI names and (optionally) LLM short titles
// for sessions that still only have a raw prompt_preview. Limited per call so
// list endpoints stay responsive.
func (s *Store) FillMissingTitles(client *llm.Client, limit int) int {
	if limit <= 0 {
		limit = 3
	}
	entries, err := s.List()
	if err != nil {
		return 0
	}
	filled := 0
	for _, e := range entries {
		if filled >= limit {
			break
		}
		if !IsPlaceholderTitle(e.Title, e.ID) {
			continue
		}
		meta, err := s.Get(e.ID)
		if err != nil || !IsPlaceholderTitle(meta.Title, meta.ID) {
			continue
		}
		EnsureTitle(&meta, client)
		if IsPlaceholderTitle(meta.Title, meta.ID) {
			continue
		}
		if err := s.UpdateMeta(meta); err != nil {
			continue
		}
		filled++
	}
	return filled
}

func (s *Store) Get(id string) (Meta, error) {
	path := filepath.Join(s.Paths.SessionDir(id), "session.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Meta{}, err
	}
	var d Document
	return d.Meta, json.Unmarshal(data, &d)
}

// GetFootprint loads the footprint embedded in session.json.
func (s *Store) GetFootprint(id string) (Footprint, error) {
	data, err := os.ReadFile(filepath.Join(s.Paths.SessionDir(id), "session.json"))
	if err != nil {
		return Footprint{}, err
	}
	var d Document
	return d.Footprint, json.Unmarshal(data, &d)
}

// VendorFromAttrs picks the coding-agent identity from span attributes.
// Prefer coding_agent.client (what hooks stamp) over the older
// coding_agent.vendor key, which is often absent on live Claude Code spans.
func VendorFromAttrs(attrs map[string]string) string {
	if attrs == nil {
		return ""
	}
	for _, k := range []string{
		"coding_agent.client",
		"coding_agent.vendor",
		"gen_ai.agent.name",
	} {
		if v := strings.TrimSpace(attrs[k]); v != "" {
			return v
		}
	}
	if v := strings.TrimSpace(attrs["service.name"]); v != "" &&
		!strings.EqualFold(v, "opentelemetry") &&
		!strings.EqualFold(v, "unknown") {
		return v
	}
	return ""
}

// VendorFromSpans returns the first non-empty vendor across spans.
func VendorFromSpans(spans []tracestore.Span) string {
	for _, sp := range spans {
		if v := VendorFromAttrs(sp.Attributes); v != "" {
			return v
		}
	}
	return ""
}

// StartTimeFromSpans returns the earliest plausible telemetry timestamp. Some
// vendor hooks omit timestamps or emit zero/Unix-epoch values; those must not
// turn an active session into a decades-long session in the UI.
func StartTimeFromSpans(spans []tracestore.Span) time.Time {
	now := time.Now().UTC()
	var earliest time.Time
	for _, sp := range spans {
		if sp.StartTimeUnixN <= 0 {
			continue
		}
		candidate := time.Unix(0, sp.StartTimeUnixN).UTC()
		if !validSessionTime(candidate, now) {
			continue
		}
		if earliest.IsZero() || candidate.Before(earliest) {
			earliest = candidate
		}
	}
	if earliest.IsZero() {
		return now
	}
	return earliest
}

func validSessionTime(value, now time.Time) bool {
	oldest := time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
	return !value.IsZero() && !value.Before(oldest) && !value.After(now.Add(24*time.Hour))
}

// mergeMetaSticky keeps previously detected identity fields when a later
// Start/upsert arrives with blanks (e.g. finalize reading coding_agent.vendor
// while spans only set coding_agent.client). Once vendor/model/user are known,
// empty updates must not clear them.
func mergeMetaSticky(existing, incoming Meta) Meta {
	out := incoming
	if out.ID == "" {
		out.ID = existing.ID
	}
	if strings.TrimSpace(out.Vendor) == "" {
		out.Vendor = existing.Vendor
	}
	if strings.TrimSpace(out.Model) == "" {
		out.Model = existing.Model
	}
	if strings.TrimSpace(out.User) == "" {
		out.User = existing.User
	}
	if strings.TrimSpace(out.Title) == "" {
		out.Title = existing.Title
	}
	if strings.TrimSpace(out.PromptPreview) == "" {
		out.PromptPreview = existing.PromptPreview
	}
	if strings.TrimSpace(out.ProjectID) == "" {
		out.ProjectID = existing.ProjectID
	}
	if strings.TrimSpace(out.RepoRoot) == "" {
		out.RepoRoot = existing.RepoRoot
	}
	if strings.TrimSpace(out.Branch) == "" {
		out.Branch = existing.Branch
	}
	if strings.TrimSpace(out.ParentID) == "" {
		out.ParentID = existing.ParentID
	}
	if !out.IsSubagent && existing.IsSubagent && out.ParentID != "" {
		out.IsSubagent = true
	}
	if !validSessionTime(out.StartedAt, time.Now().UTC()) {
		out.StartedAt = existing.StartedAt
	}
	if out.Tokens == 0 {
		out.Tokens = existing.Tokens
	}
	if out.CostUSD == 0 {
		out.CostUSD = existing.CostUSD
	}
	if out.DurationMs == 0 {
		out.DurationMs = existing.DurationMs
	}
	return out
}

// UpdateMeta writes session.json and refreshes the sessions index.
func (s *Store) UpdateMeta(meta Meta) error {
	dir := s.Paths.SessionDir(meta.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := s.writeDocument(meta.ID, func(d *Document) { d.Meta = meta }); err != nil {
		return err
	}
	return s.upsertIndex(meta)
}

func (s *Store) Start(meta Meta) error {
	if meta.ID == "" {
		meta.ID = fmt.Sprintf("ses_%d", time.Now().UnixNano())
	}
	if existing, err := s.Get(meta.ID); err == nil {
		meta = mergeMetaSticky(existing, meta)
	}
	if !validSessionTime(meta.StartedAt, time.Now().UTC()) {
		meta.StartedAt = time.Now().UTC()
	}
	meta.Status = StatusActive
	dir := s.Paths.SessionDir(meta.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := s.writeDocument(meta.ID, func(d *Document) { d.Meta = meta }); err != nil {
		return err
	}
	return s.upsertIndex(meta)
}

// UpsertActiveFromSpans creates/refreshes an active session row so the UI shows
// live traffic before sessionEnd finalize. Does not overwrite ended sessions.
//
// Session identity prefers the stable chat-thread id (conversation.id). Subagent
// traffic keeps its own id with parent_id set so the Sessions list shows one row
// per chat and nested agents appear under the parent when viewing that chat.
func (s *Store) UpsertActiveFromSpans(spans []tracestore.Span) {
	if len(spans) == 0 {
		return
	}
	type bucket struct {
		spans  []tracestore.Span
		parent string
		nested bool
	}
	by := map[string]*bucket{}
	for _, sp := range spans {
		sid := otlp.ResolveSessionID(sp.Attributes, "")
		if sid == "" {
			continue
		}
		parent := otlp.ResolveParentID(sp.Attributes)
		if parent == sid {
			parent = "" // self-parent echo must not nest the parent chat
		}
		b := by[sid]
		if b == nil {
			b = &bucket{}
			by[sid] = b
		}
		b.spans = append(b.spans, sp)
		if parent != "" {
			b.parent = parent
			b.nested = true
		}
	}
	for id, b := range by {
		if !SpansHaveActivity(b.spans) {
			// Do not create empty sessions from identity-only telemetry.
			// Still refresh existing rows if they already exist.
			if _, err := s.Get(id); err != nil {
				continue
			}
		}
		existing, err := s.Get(id)
		if err == nil && existing.Status == StatusEnded {
			continue
		}
		meta := existing
		if err != nil {
			meta = Meta{ID: id, Status: StatusActive}
		}
		meta.Status = StatusActive
		if b.parent == "" {
			if p := agentlinks.ResolveParent(s.Paths.SessionsDir, id, meta.Vendor); p != "" {
				b.parent = p
				b.nested = true
			}
		}
		// Never nest a top-level Cursor chat (own transcripts folder), and
		// drop links to empty parent stubs left by greedy pending-claim.
		if b.parent != "" && (agentlinks.IsTopLevelCursorChat(id) || s.isEmptyParentStub(b.parent)) {
			_ = agentlinks.Unregister(s.Paths.SessionsDir, id)
			b.parent = ""
			b.nested = false
			meta.ParentID = ""
			meta.IsSubagent = false
		}
		if b.parent != "" {
			meta.ParentID = b.parent
			meta.IsSubagent = true
			_ = agentlinks.Register(s.Paths.SessionsDir, id, b.parent, meta.Vendor, "event-upsert")
		} else if meta.IsSubagent && meta.ParentID != "" {
			// Parent cleared above - ensure meta stays top-level.
			meta.ParentID = ""
			meta.IsSubagent = false
		} else if meta.IsSubagent && meta.ParentID == "" {
			// Clear poison: subagent-typed spans on a parent session id.
			meta.IsSubagent = false
		}
		for _, sp := range b.spans {
			if meta.Vendor == "" {
				meta.Vendor = VendorFromAttrs(sp.Attributes)
			}
			if meta.Model == "" {
				if v := sp.Attributes["gen_ai.request.model"]; v != "" {
					meta.Model = v
				} else if v := sp.Attributes["gen_ai.response.model"]; v != "" {
					meta.Model = v
				}
			}
			if meta.User == "" {
				if v := sp.Attributes["gen_ai.user.name"]; v != "" {
					meta.User = v
				}
			}
			if meta.PromptPreview == "" {
				if v := sp.Attributes["gen_ai.prompt"]; v != "" {
					meta.PromptPreview = truncateRunes(humanizePromptPreview(v), 160)
				} else if v := sp.Attributes["gen_ai.content.prompt"]; v != "" {
					meta.PromptPreview = truncateRunes(humanizePromptPreview(v), 160)
				} else if v := sp.Attributes["gen_ai.input.messages"]; v != "" {
					meta.PromptPreview = truncateRunes(humanizePromptPreview(v), 160)
				}
			}
			if meta.Branch == "" {
				if v := sp.Attributes["vcs.ref.head.name"]; v != "" {
					meta.Branch = v
				}
			}
			if meta.ParentID == "" {
				if p := otlp.ResolveParentID(sp.Attributes); p != "" && p != id {
					if !agentlinks.IsTopLevelCursorChat(id) {
						meta.ParentID = p
						meta.IsSubagent = true
					}
				}
			}
		}
		if meta.IsSubagent && meta.ParentID == "" {
			meta.IsSubagent = false
		}
		if IsPlaceholderTitle(meta.Title, meta.ID) {
			EnsureTitle(&meta, nil)
		}
		if !validSessionTime(meta.StartedAt, time.Now().UTC()) {
			meta.StartedAt = StartTimeFromSpans(b.spans)
		}
		_ = s.Start(meta)
		// Do NOT create empty parent stubs here. Empty parents hide nested
		// children from the Sessions list (and recreate test pollution like
		// cur-chat-2). Parents materialize from their own hook traffic.
	}
}

func truncateRunes(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// MaterializeFromSpans builds events, footprint, and updates session.json post-session.
func (s *Store) MaterializeFromSpans(id string, spans []tracestore.Span, tokens int64, cost float64) (Meta, error) {
	dir := s.Paths.SessionDir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Meta{}, err
	}

	meta, err := s.Get(id)
	if err != nil {
		meta = Meta{ID: id, StartedAt: StartTimeFromSpans(spans)}
	} else if !validSessionTime(meta.StartedAt, time.Now().UTC()) {
		meta.StartedAt = StartTimeFromSpans(spans)
	}
	// Always (re)fill empty vendor from spans — never leave "" after materialize,
	// and never rely on a prior Start that only read coding_agent.vendor.
	if strings.TrimSpace(meta.Vendor) == "" {
		if v := VendorFromSpans(spans); v != "" {
			meta.Vendor = v
		} else {
			meta.Vendor = "unknown"
		}
	}
	if meta.Model == "" && len(spans) > 0 {
		if v := spans[0].Attributes["gen_ai.request.model"]; v != "" {
			meta.Model = v
		} else if v := spans[0].Attributes["gen_ai.response.model"]; v != "" {
			meta.Model = v
		}
	}

	now := time.Now().UTC()
	meta.Status = StatusEnded
	meta.EndedAt = &now
	meta.DurationMs = max(0, now.Sub(meta.StartedAt).Milliseconds())
	meta.Tokens = tokens
	meta.CostUSD = cost

	// Authoritative normalized event stream.
	eventsPath := filepath.Join(dir, "events.jsonl")
	if err := os.Remove(eventsPath); err != nil && !os.IsNotExist(err) {
		return Meta{}, err
	}
	if err := artifactmeta.EnsureJSONL(eventsPath, artifactmeta.About{
		Purpose:   "Normalized prompts, responses, tool calls, file activity, usage, lifecycle, and audit events for this session.",
		Authority: "authoritative session event stream", UpdatedBy: "vendor telemetry adapter",
	}); err != nil {
		return Meta{}, err
	}
	tf, err := os.OpenFile(eventsPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return Meta{}, err
	}
	enc := json.NewEncoder(tf)
	foot := map[string]*FootprintFile{}
	retrievals := []MemoryRetrieval{}
	for _, sp := range spans {
		safe := sp
		if len(sp.Attributes) > 0 {
			safe.Attributes = make(map[string]string, len(sp.Attributes))
			for k, v := range sp.Attributes {
				safe.Attributes[k] = redact.String(v)
			}
		}
		_ = enc.Encode(safe)
		if sp.Name == "superopen.memory.retrieved" {
			retrievals = append(retrievals, MemoryRetrieval{
				PatternIDs:      parseStringList(safe.Attributes["superopen.memory.pattern_ids"]),
				Scores:          parseStringList(safe.Attributes["superopen.memory.scores"]),
				Reasons:         parseStringList(safe.Attributes["superopen.memory.reasons"]),
				TargetPaths:     parseStringList(safe.Attributes["superopen.memory.target_paths"]),
				EstimatedTokens: parseInt64(safe.Attributes["superopen.memory.estimated_tokens"]),
				TurnID:          safe.Attributes["superopen.memory.turn_id"], Delivery: safe.Attributes["superopen.memory.delivery"],
				At: time.Unix(0, sp.StartTimeUnixN).UTC(),
			})
		}
		if meta.Vendor == "" || meta.Vendor == "unknown" {
			if v := VendorFromAttrs(sp.Attributes); v != "" {
				meta.Vendor = v
			}
		}
		if meta.Model == "" {
			if m := sp.Attributes["gen_ai.request.model"]; m != "" {
				meta.Model = m
			} else if m := sp.Attributes["gen_ai.response.model"]; m != "" {
				meta.Model = m
			}
		}
		if meta.PromptPreview == "" {
			if p := safe.Attributes["gen_ai.prompt"]; p != "" {
				meta.PromptPreview = truncate(humanizePromptPreview(p), 120)
			} else if p := safe.Attributes["gen_ai.content.prompt"]; p != "" {
				meta.PromptPreview = truncate(humanizePromptPreview(p), 120)
			} else if raw := safe.Attributes["gen_ai.input.messages"]; raw != "" {
				meta.PromptPreview = truncate(humanizePromptPreview(raw), 120)
			}
		}
		if path := sp.Attributes["coding_agent.file_path"]; path != "" {
			state := "seen"
			name := strings.ToLower(sp.Name)
			switch {
			case strings.Contains(name, "edit") || strings.Contains(name, "write"):
				state = "edited"
			case strings.Contains(name, "read"):
				state = "read"
			}
			if existing, ok := foot[path]; ok {
				existing.Count++
				if rank(state) > rank(existing.State) {
					existing.State = state
				}
			} else {
				foot[path] = &FootprintFile{Path: path, State: state, Count: 1}
			}
		}
		if path := sp.Attributes["code.file.path"]; path != "" {
			state := "edited"
			if existing, ok := foot[path]; ok {
				existing.Count++
				if rank(state) > rank(existing.State) {
					existing.State = state
				}
			} else {
				foot[path] = &FootprintFile{Path: path, State: state, Count: 1}
			}
		}
	}
	tf.Close()

	fp := Footprint{}
	for _, f := range foot {
		fp.Files = append(fp.Files, *f)
	}
	sort.Slice(fp.Files, func(i, j int) bool { return fp.Files[i].Path < fp.Files[j].Path })
	ApplyVCSFromSpans(&meta, spans)

	// Persist parent / subagent linkage from span attributes so nested agents
	// stay under the spawning chat in the Sessions UI.
	if meta.ParentID == "" || !meta.IsSubagent {
		for _, sp := range spans {
			if meta.ParentID == "" {
				if p := otlp.ResolveParentID(sp.Attributes); p != "" && p != id {
					meta.ParentID = p
					meta.IsSubagent = true
				}
			}
			if meta.ParentID != "" && meta.IsSubagent {
				break
			}
		}
	}
	if meta.IsSubagent && meta.ParentID == "" {
		meta.IsSubagent = false
	}

	// Prefer Claude aiTitle / Codex thread_name / OpenCode AI title when the id matches.
	if IsPlaceholderTitle(meta.Title, meta.ID) {
		EnsureTitle(&meta, nil) // vendor lookup only; LLM fill happens via FillMissingTitles
	}

	if err := s.writeDocument(id, func(d *Document) { d.Meta = meta; d.Footprint = fp; d.MemoryRetrievals = retrievals }); err != nil {
		return Meta{}, err
	}
	if err := s.upsertIndex(meta); err != nil {
		return Meta{}, err
	}
	return meta, nil
}

func parseStringList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var values []string
	if json.Unmarshal([]byte(raw), &values) == nil {
		return values
	}
	return []string{raw}
}

func parseInt64(raw string) int64 {
	var value int64
	_, _ = fmt.Sscan(raw, &value)
	return value
}

func (s *Store) upsertIndex(meta Meta) error {
	entries, _ := s.List()
	found := false
	for i := range entries {
		if entries[i].ID == meta.ID {
			entries[i] = meta
			found = true
			break
		}
	}
	if !found {
		entries = append(entries, meta)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].StartedAt.After(entries[j].StartedAt)
	})
	idx := indexFile{About: indexAbout, Sessions: entries}
	if data, err := os.ReadFile(s.Paths.SessionsIndex); err == nil {
		var prev indexFile
		if json.Unmarshal(data, &prev) == nil {
			idx.PendingSpawns = prev.PendingSpawns
			idx.Links = prev.Links
		}
	}
	return writeJSON(s.Paths.SessionsIndex, idx)
}

func (s *Store) ReadDocument(id string) (Document, error) {
	var d Document
	data, err := os.ReadFile(filepath.Join(s.Paths.SessionDir(id), "session.json"))
	if err != nil {
		return d, err
	}
	err = json.Unmarshal(data, &d)
	return d, err
}

func (s *Store) WriteDocument(id string, mutate func(*Document)) error {
	return s.writeDocument(id, mutate)
}

// PreviousReviewContext returns context for only the latest preceding session
// owned by vendor. It never leaks another vendor's review into a new session.
func (s *Store) PreviousReviewContext(vendor, exceptID string) string {
	entries, _ := s.List()
	kind := harness.NormalizeVendorKind(vendor)
	for _, m := range entries {
		if m.ID == exceptID || harness.NormalizeVendorKind(m.Vendor) != kind {
			continue
		}
		d, err := s.ReadDocument(m.ID)
		if err != nil {
			continue
		}
		switch d.Review.Status {
		case "pending", "running", "failed":
			return fmt.Sprintf("## Previous %s session review\n\nReview for `%s` is pending in the background; normal work may continue.", kind, m.ID)
		case "complete":
			var ev struct {
				Score float64  `json:"score"`
				Badge string   `json:"badge"`
				Notes []string `json:"notes"`
			}
			_ = json.Unmarshal(d.Evaluation, &ev)
			text := fmt.Sprintf("## Previous %s session review\n\nSession `%s`: %s (%.2f).", kind, m.ID, ev.Badge, ev.Score)
			if len(ev.Notes) > 0 {
				text += "\n\n- " + strings.Join(ev.Notes, "\n- ")
			}
			return text
		}
		return ""
	}
	return ""
}

// ClaimReview uses an OS-runtime lock so concurrent end/start hooks cannot
// review or apply documentation for the same session twice.
func (s *Store) ClaimReview(id, trigger string) (func(), bool) {
	sum := sha256.Sum256([]byte(s.Paths.RepoRoot + "\x00" + id))
	lock := filepath.Join(os.TempDir(), fmt.Sprintf("superopen-review-%x.lock", sum[:12]))
	if err := os.Mkdir(lock, 0o700); err != nil {
		if st, statErr := os.Stat(lock); statErr == nil && time.Since(st.ModTime()) > 10*time.Minute {
			_ = os.RemoveAll(lock)
			if err = os.Mkdir(lock, 0o700); err == nil {
				goto claimed
			}
		}
		return func() {}, false
	}
claimed:
	now := time.Now().UTC()
	_ = s.WriteDocument(id, func(d *Document) {
		d.Review.Status = "running"
		d.Review.Trigger = trigger
		d.Review.StartedAt = &now
		d.Review.Error = ""
	})
	return func() { _ = os.RemoveAll(lock) }, true
}

func (s *Store) writeDocument(id string, mutate func(*Document)) error {
	d, _ := s.ReadDocument(id)
	d.About = sessionAbout
	mutate(&d)
	if d.ID == "" {
		d.ID = id
	}
	path := filepath.Join(s.Paths.SessionDir(id), "session.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeJSON(path, d)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// humanizePromptPreview turns Cursor/Claude message JSON (or plain text) into a short label.
func humanizePromptPreview(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "[") || strings.HasPrefix(raw, "{") {
		if t := previewFromMessages(raw); t != "" {
			return t
		}
	}
	return raw
}

func previewFromMessages(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// Cursor often stores a single message object; normalize to an array.
	if strings.HasPrefix(raw, "{") {
		raw = "[" + raw + "]"
	}
	var msgs []map[string]any
	if err := json.Unmarshal([]byte(raw), &msgs); err != nil {
		return ""
	}
	for _, msg := range msgs {
		role, _ := msg["role"].(string)
		if role != "" && role != "user" && role != "user_prompt" {
			continue
		}
		if content, ok := msg["content"].(string); ok && content != "" {
			return content
		}
		if parts, ok := msg["parts"].([]any); ok {
			var b strings.Builder
			for _, p := range parts {
				pm, _ := p.(map[string]any)
				if pm == nil {
					continue
				}
				if c, ok := pm["content"].(string); ok {
					b.WriteString(c)
				} else if c, ok := pm["text"].(string); ok {
					b.WriteString(c)
				}
			}
			if b.Len() > 0 {
				return b.String()
			}
		}
	}
	return ""
}

func rank(state string) int {
	switch state {
	case "edited":
		return 3
	case "read":
		return 2
	default:
		return 1
	}
}
