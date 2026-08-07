package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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

// Meta is stored at .so/sessions/<id>/meta.json.
type Meta struct {
	ID            string     `json:"id"`
	Vendor        string     `json:"vendor"`
	Model         string     `json:"model,omitempty"`
	User          string     `json:"user,omitempty"` // gen_ai.user.name (email / identity)
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
	ProjectID     string              `json:"project_id,omitempty"`
	RepoRoot      string              `json:"repo_root,omitempty"`
	Branch        string              `json:"branch,omitempty"`
	BaseSHA       string              `json:"base_sha,omitempty"`
	HeadSHA       string              `json:"head_sha,omitempty"`
	Commits       []CommitRef         `json:"commits,omitempty"`
	PullRequests  []PRRef             `json:"pull_requests,omitempty"`
	Attribution   *AttributionSummary `json:"attribution,omitempty"`
	Summary       string              `json:"summary,omitempty"`
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

// Store manages session materialization under .so/sessions/.
type Store struct {
	Paths harness.Paths
}

func NewStore(paths harness.Paths) *Store {
	return &Store{Paths: paths}
}

func (s *Store) List() ([]IndexEntry, error) {
	data, err := os.ReadFile(s.Paths.SessionsIndex)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var entries []IndexEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
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
// repairing meta.json / agent-links when discovery finds a link that
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

	if data, err := os.ReadFile(filepath.Join(dir, "footprint.json")); err == nil {
		var fp Footprint
		if json.Unmarshal(data, &fp) == nil {
			for _, f := range fp.Files {
				item.Files = append(item.Files, f.Path)
			}
		}
	}

	item.Turns, item.Model = s.scanTranscript(dir, meta.Model)
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

func (s *Store) scanTranscript(dir, existingModel string) (checkpoints int, model string) {
	model = existingModel
	f, err := os.Open(filepath.Join(dir, "transcript.jsonl"))
	if err != nil {
		return 0, model
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
		attrs := sp.Attributes
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
			checkpoints++
			continue
		}
		if raw := attrs["gen_ai.input.messages"]; raw != "" {
			low := strings.ToLower(raw)
			if strings.Contains(low, `"role":"user"`) || strings.Contains(low, `"role": "user"`) ||
				strings.Contains(low, `"role":"user_prompt"`) {
				checkpoints++
			}
		}
	}
	return checkpoints, model
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
	f, err := os.Open(filepath.Join(s.Paths.SessionDir(id), "transcript.jsonl"))
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
		if strings.TrimSpace(e.Title) != "" {
			continue
		}
		meta, err := s.Get(e.ID)
		if err != nil || strings.TrimSpace(meta.Title) != "" {
			continue
		}
		EnsureTitle(&meta, client)
		if strings.TrimSpace(meta.Title) == "" {
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
	path := filepath.Join(s.Paths.SessionDir(id), "meta.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Meta{}, err
	}
	var m Meta
	return m, json.Unmarshal(data, &m)
}

// GetFootprint loads footprint.json for a session.
func (s *Store) GetFootprint(id string) (Footprint, error) {
	data, err := os.ReadFile(filepath.Join(s.Paths.SessionDir(id), "footprint.json"))
	if err != nil {
		return Footprint{}, err
	}
	var fp Footprint
	return fp, json.Unmarshal(data, &fp)
}

// UpdateMeta writes meta.json and refreshes the sessions index.
func (s *Store) UpdateMeta(meta Meta) error {
	dir := s.Paths.SessionDir(meta.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "meta.json"), meta); err != nil {
		return err
	}
	return s.upsertIndex(meta)
}

func (s *Store) Start(meta Meta) error {
	if meta.ID == "" {
		meta.ID = fmt.Sprintf("ses_%d", time.Now().UnixNano())
	}
	if meta.StartedAt.IsZero() {
		meta.StartedAt = time.Now().UTC()
	}
	meta.Status = StatusActive
	dir := s.Paths.SessionDir(meta.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "meta.json"), meta); err != nil {
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
			// Do not create empty sessions from identity-only OTLP.
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
			_ = agentlinks.Register(s.Paths.SessionsDir, id, b.parent, meta.Vendor, "otlp-upsert")
		} else if meta.IsSubagent && meta.ParentID != "" {
			// Parent cleared above - ensure meta stays top-level.
			meta.ParentID = ""
			meta.IsSubagent = false
		} else if meta.IsSubagent && meta.ParentID == "" {
			// Clear poison: subagent-typed spans on a parent session id.
			meta.IsSubagent = false
		}
		for _, sp := range b.spans {
			if meta.StartedAt.IsZero() && sp.StartTimeUnixN > 0 {
				meta.StartedAt = time.Unix(0, sp.StartTimeUnixN).UTC()
			}
			if meta.Vendor == "" {
				if v := sp.Attributes["coding_agent.client"]; v != "" {
					meta.Vendor = v
				} else if v := sp.Attributes["coding_agent.vendor"]; v != "" {
					meta.Vendor = v
				} else if v := sp.Attributes["gen_ai.agent.name"]; v != "" {
					meta.Vendor = v
				}
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
		if strings.TrimSpace(meta.Title) == "" {
			EnsureTitle(&meta, nil)
		}
		if meta.StartedAt.IsZero() {
			meta.StartedAt = time.Now().UTC()
		}
		_ = s.Start(meta)
		// Do NOT create empty parent stubs here. Empty parents hide nested
		// children from the Sessions list (and recreate test pollution like
		// cur-chat-2). Parents materialize from their own OTLP traffic.
	}
}

func truncateRunes(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "…"
}


// MaterializeFromSpans builds transcript, footprint, and updates meta post-session.
func (s *Store) MaterializeFromSpans(id string, spans []tracestore.Span, tokens int64, cost float64) (Meta, error) {
	dir := s.Paths.SessionDir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Meta{}, err
	}

	meta, err := s.Get(id)
	if err != nil {
		meta = Meta{ID: id, Vendor: "unknown", StartedAt: time.Now().UTC()}
		if len(spans) > 0 {
			meta.StartedAt = time.Unix(0, spans[0].StartTimeUnixN).UTC()
			if v := spans[0].Attributes["coding_agent.vendor"]; v != "" {
				meta.Vendor = v
			} else if v := spans[0].Attributes["coding_agent.client"]; v != "" {
				meta.Vendor = v
			}
			if v := spans[0].Attributes["gen_ai.request.model"]; v != "" {
				meta.Model = v
			} else if v := spans[0].Attributes["gen_ai.response.model"]; v != "" {
				meta.Model = v
			}
		}
	}

	now := time.Now().UTC()
	meta.Status = StatusEnded
	meta.EndedAt = &now
	meta.DurationMs = now.Sub(meta.StartedAt).Milliseconds()
	meta.Tokens = tokens
	meta.CostUSD = cost

	// Transcript
	tf, err := os.Create(filepath.Join(dir, "transcript.jsonl"))
	if err != nil {
		return Meta{}, err
	}
	enc := json.NewEncoder(tf)
	foot := map[string]*FootprintFile{}
	for _, sp := range spans {
		safe := sp
		if len(sp.Attributes) > 0 {
			safe.Attributes = make(map[string]string, len(sp.Attributes))
			for k, v := range sp.Attributes {
				safe.Attributes[k] = redact.String(v)
			}
		}
		_ = enc.Encode(safe)
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
	if err := writeJSON(filepath.Join(dir, "footprint.json"), fp); err != nil {
		return Meta{}, err
	}

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

	// Prefer Claude aiTitle / Codex thread_name when the session id matches.
	if strings.TrimSpace(meta.Title) == "" {
		EnsureTitle(&meta, nil) // vendor lookup only; LLM fill happens via FillMissingTitles
	}

	if err := writeJSON(filepath.Join(dir, "meta.json"), meta); err != nil {
		return Meta{}, err
	}
	if err := s.upsertIndex(meta); err != nil {
		return Meta{}, err
	}
	return meta, nil
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
	return writeJSON(s.Paths.SessionsIndex, entries)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
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
