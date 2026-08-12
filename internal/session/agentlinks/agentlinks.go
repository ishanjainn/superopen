// Package agentlinks persists child-agent → parent-session mappings so
// Task / subagent traffic does not appear as top-level Sessions rows.
//
// Coding agents often give child Task runs their own conversation id
// (Cursor) or agent id (Claude Code) without stamping parent_id on
// every hook payload. We record agentId from Task tool results and
// also discover Cursor's on-disk layout:
//
//	~/.cursor/projects/<proj>/agent-transcripts/<parent>/subagents/<child>.jsonl
package agentlinks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const fileName = "index.json"

// pendingTTL is how long an unclaimed subagentStart can be matched to
// the next orphan child session that hooks in without a parent id.
const pendingTTL = 3 * time.Minute

var (
	uuidRe = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	// agentId in Task tool results: JSON "agentId":"…" or "agentId: …"
	agentIDJSONRe = regexp.MustCompile(`(?i)"agentId"\s*:\s*"([^"]+)"`)
	agentIDTextRe = regexp.MustCompile(`(?i)\bagentId\s*:\s*([A-Za-z0-9_-]+)`)
	mu            sync.Mutex
)

// Entry is one child → parent link.
type Entry struct {
	ParentID string    `json:"parent_id"`
	Vendor   string    `json:"vendor,omitempty"`
	Source   string    `json:"source,omitempty"`
	At       time.Time `json:"at,omitempty"`
}

type fileDoc struct {
	About    json.RawMessage  `json:"_about,omitempty"`
	Sessions json.RawMessage  `json:"sessions,omitempty"`
	Links    map[string]Entry `json:"links,omitempty"`
	Pending  []PendingSpawn   `json:"pending_spawns,omitempty"`
}

// PendingSpawn records a parent that just emitted subagentStart /
// Task so the child's first hooks can claim the link before
// agentId / transcript files exist.
type PendingSpawn struct {
	ParentID   string    `json:"parent_id"`
	Vendor     string    `json:"vendor,omitempty"`
	ToolCallID string    `json:"tool_call_id,omitempty"`
	At         time.Time `json:"at"`
	ClaimedBy  string    `json:"claimed_by,omitempty"`
}

type pendingDoc struct {
	Pending []PendingSpawn `json:"pending"`
}

// Path returns the durable links file under .so/sessions/.
func Path(sessionsDir string) string {
	if sessionsDir == "" {
		return ""
	}
	return filepath.Join(sessionsDir, fileName)
}

// AllowRegister reports whether id is safe to store as a child session
// key (not a tool-call id / path / empty).
func AllowRegister(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" || len(id) < 4 {
		return false
	}
	if strings.ContainsAny(id, "\n/\t") {
		return false
	}
	lower := strings.ToLower(id)
	if strings.HasPrefix(lower, "call-") || strings.HasPrefix(lower, "toolu_") {
		return false
	}
	return true
}

// IsChildSessionID reports whether id looks like a UUID-shaped (or
// agent-/task-prefixed) child conversation - used for Cursor transcript
// discovery and UI orphan heuristics.
func IsChildSessionID(id string) bool {
	id = strings.TrimSpace(id)
	if !AllowRegister(id) {
		return false
	}
	if uuidRe.MatchString(id) {
		return true
	}
	lower := strings.ToLower(id)
	return strings.HasPrefix(lower, "agent-") || strings.HasPrefix(lower, "task-")
}

// SessionsDir resolves .so/sessions for a working directory.
// Requires an explicit cwd - never falls back to process getwd, so unit
// tests (and hooks without a workspace path) cannot pollute a real
// repo's .so/sessions via pending-spawns / agent-links.
func SessionsDir(cwd string) string {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return ""
	}
	root, err := findRepoRoot(cwd)
	if err != nil || root == "" {
		return ""
	}
	return filepath.Join(root, ".so", "sessions")
}

func findRepoRoot(start string) (string, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	dir := abs
	for {
		if info, err := os.Stat(filepath.Join(dir, ".so")); err == nil && info.IsDir() {
			return dir, nil
		}
		if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil && (info.IsDir() || info.Mode().IsRegular()) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return abs, nil
		}
		dir = parent
	}
}

// ExtractAgentID pulls an agent id from Task/Agent tool output text
// .
func ExtractAgentID(blobs ...string) string {
	for _, blob := range blobs {
		blob = strings.TrimSpace(blob)
		if blob == "" {
			continue
		}
		if m := agentIDJSONRe.FindStringSubmatch(blob); len(m) == 2 {
			if id := strings.TrimSpace(m[1]); id != "" {
				return id
			}
		}
		if m := agentIDTextRe.FindStringSubmatch(blob); len(m) == 2 {
			if id := strings.TrimSpace(m[1]); id != "" {
				return id
			}
		}
		// Bare JSON object with agentId
		var obj map[string]any
		if err := json.Unmarshal([]byte(blob), &obj); err == nil {
			for _, k := range []string{"agentId", "agent_id", "subagent_id", "id"} {
				if v, ok := obj[k].(string); ok {
					if id := strings.TrimSpace(v); id != "" {
						return id
					}
				}
			}
		}
	}
	return ""
}

// ParentFromCursorTranscriptPath extracts parent + child when path is
// …/agent-transcripts/<parent>/subagents/<child>.jsonl
func ParentFromCursorTranscriptPath(path string) (parent, child string) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return "", ""
	}
	parts := strings.Split(path, string(os.PathSeparator))
	for i := 0; i+2 < len(parts); i++ {
		if parts[i] != "agent-transcripts" {
			continue
		}
		parent = parts[i+1]
		if i+3 < len(parts) && parts[i+2] == "subagents" {
			base := parts[i+3]
			child = strings.TrimSuffix(base, filepath.Ext(base))
			if IsChildSessionID(parent) && IsChildSessionID(child) && parent != child {
				return parent, child
			}
		}
	}
	return "", ""
}

// IsTopLevelCursorChat reports whether id has its own Cursor agent-transcripts
// folder (or top-level .jsonl). Real Task children live only under
// <parent>/subagents/<child>.jsonl - they must not claim pending spawns meant
// for subagents, and list filters must not hide them as nested.
func IsTopLevelCursorChat(id string) bool {
	id = strings.TrimSpace(id)
	if !uuidRe.MatchString(id) {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return false
	}
	projects := filepath.Join(home, ".cursor", "projects")
	entries, err := os.ReadDir(projects)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		root := filepath.Join(projects, e.Name(), "agent-transcripts")
		if st, err := os.Stat(filepath.Join(root, id)); err == nil && st.IsDir() {
			return true
		}
		if st, err := os.Stat(filepath.Join(root, id+".jsonl")); err == nil && !st.IsDir() {
			return true
		}
	}
	return false
}

// DiscoverCursorParent walks ~/.cursor/projects/*/agent-transcripts/*/subagents/<child>.jsonl
func DiscoverCursorParent(childID string) string {
	childID = strings.TrimSpace(childID)
	if !IsChildSessionID(childID) {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	projects := filepath.Join(home, ".cursor", "projects")
	entries, err := os.ReadDir(projects)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		root := filepath.Join(projects, e.Name(), "agent-transcripts")
		parents, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, p := range parents {
			if !p.IsDir() || !IsChildSessionID(p.Name()) {
				continue
			}
			candidate := filepath.Join(root, p.Name(), "subagents", childID+".jsonl")
			if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
				return p.Name()
			}
		}
	}
	return ""
}

// Register writes child → parent. No-op when ids are empty, equal, or
// child is not a session-shaped id.
func Register(sessionsDir, childID, parentID, vendor, source string) error {
	childID = strings.TrimSpace(childID)
	parentID = strings.TrimSpace(parentID)
	if sessionsDir == "" || childID == "" || parentID == "" || childID == parentID {
		return nil
	}
	if !AllowRegister(childID) {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	doc, err := loadLocked(sessionsDir)
	if err != nil {
		doc = fileDoc{Links: map[string]Entry{}}
	}
	if doc.Links == nil {
		doc.Links = map[string]Entry{}
	}
	if existing, ok := doc.Links[childID]; ok && existing.ParentID == parentID {
		return nil
	}
	doc.Links[childID] = Entry{
		ParentID: parentID,
		Vendor:   vendor,
		Source:   source,
		At:       time.Now().UTC(),
	}
	return saveLocked(sessionsDir, doc)
}

// Lookup returns the parent id for child, if known.
func Lookup(sessionsDir, childID string) (string, bool) {
	childID = strings.TrimSpace(childID)
	if sessionsDir == "" || childID == "" {
		return "", false
	}
	mu.Lock()
	defer mu.Unlock()
	doc, err := loadLocked(sessionsDir)
	if err != nil || doc.Links == nil {
		return "", false
	}
	e, ok := doc.Links[childID]
	if !ok || e.ParentID == "" || e.ParentID == childID {
		return "", false
	}
	return e.ParentID, true
}

// LoadMap returns child → parent for the sessions dir.
func LoadMap(sessionsDir string) map[string]string {
	out := map[string]string{}
	if sessionsDir == "" {
		return out
	}
	mu.Lock()
	defer mu.Unlock()
	doc, err := loadLocked(sessionsDir)
	if err != nil || doc.Links == nil {
		return out
	}
	for child, e := range doc.Links {
		if e.ParentID != "" && e.ParentID != child {
			out[child] = e.ParentID
		}
	}
	return out
}

// ResolveParent finds a parent via durable links, then Cursor transcript layout.
// When found via filesystem, the link is persisted for later list filters.
// Top-level Cursor chats (own agent-transcripts/<id>/) never resolve as children.
func ResolveParent(sessionsDir, childID, vendor string) string {
	childID = strings.TrimSpace(childID)
	if childID == "" {
		return ""
	}
	if IsTopLevelCursorChat(childID) {
		return ""
	}
	if p, ok := Lookup(sessionsDir, childID); ok {
		return p
	}
	if p := DiscoverCursorParent(childID); p != "" {
		_ = Register(sessionsDir, childID, p, vendor, "cursor-transcript")
		return p
	}
	return ""
}

// NotePending records that parent just spawned a subagent so the next
// orphan child session can claim the link.
func NotePending(sessionsDir, parentID, vendor, toolCallID string) error {
	parentID = strings.TrimSpace(parentID)
	if sessionsDir == "" || parentID == "" {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	doc, _ := loadPendingLocked(sessionsDir)
	now := time.Now().UTC()
	kept := make([]PendingSpawn, 0, len(doc.Pending)+1)
	for _, p := range doc.Pending {
		if p.ClaimedBy != "" {
			continue
		}
		if now.Sub(p.At) > pendingTTL {
			continue
		}
		kept = append(kept, p)
	}
	kept = append(kept, PendingSpawn{
		ParentID:   parentID,
		Vendor:     vendor,
		ToolCallID: toolCallID,
		At:         now,
	})
	doc.Pending = kept
	return savePendingLocked(sessionsDir, doc)
}

// ClaimPending assigns the oldest unclaimed pending spawn to childID
// and registers the durable link. Returns parent id or "".
//
// Refuses top-level Cursor chats (their own agent-transcripts/<id>/ folder):
// greedy claims previously nested real parent conversations under empty
// stubs left by tests or unrelated subagentStart events.
func ClaimPending(sessionsDir, childID, vendor string) string {
	childID = strings.TrimSpace(childID)
	if sessionsDir == "" || !IsChildSessionID(childID) {
		return ""
	}
	if IsTopLevelCursorChat(childID) {
		return ""
	}
	mu.Lock()
	defer mu.Unlock()
	if existing, err := loadLocked(sessionsDir); err == nil && existing.Links != nil {
		if e, ok := existing.Links[childID]; ok && e.ParentID != "" && e.ParentID != childID {
			return e.ParentID
		}
	}
	doc, err := loadPendingLocked(sessionsDir)
	if err != nil || len(doc.Pending) == 0 {
		return ""
	}
	now := time.Now().UTC()
	idx := -1
	for i, p := range doc.Pending {
		if p.ClaimedBy != "" || p.ParentID == "" || p.ParentID == childID {
			continue
		}
		if now.Sub(p.At) > pendingTTL {
			continue
		}
		if vendor != "" && p.Vendor != "" && !strings.EqualFold(p.Vendor, vendor) {
			continue
		}
		idx = i
		break
	}
	if idx < 0 {
		return ""
	}
	parent := doc.Pending[idx].ParentID
	doc.Pending[idx].ClaimedBy = childID
	_ = savePendingLocked(sessionsDir, doc)
	// Register without re-taking mu: inline write
	links, _ := loadLocked(sessionsDir)
	if links.Links == nil {
		links.Links = map[string]Entry{}
	}
	links.Links[childID] = Entry{
		ParentID: parent,
		Vendor:   vendor,
		Source:   "pending-claim",
		At:       now,
	}
	_ = saveLocked(sessionsDir, links)
	return parent
}

// Unregister removes a child → parent link (false-nesting repair).
func Unregister(sessionsDir, childID string) error {
	childID = strings.TrimSpace(childID)
	if sessionsDir == "" || childID == "" {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	doc, err := loadLocked(sessionsDir)
	if err != nil || doc.Links == nil {
		return nil
	}
	if _, ok := doc.Links[childID]; !ok {
		return nil
	}
	delete(doc.Links, childID)
	return saveLocked(sessionsDir, doc)
}

func loadLocked(sessionsDir string) (fileDoc, error) {
	path := Path(sessionsDir)
	raw, err := os.ReadFile(path)
	if err != nil {
		return fileDoc{Links: map[string]Entry{}}, err
	}
	var doc fileDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fileDoc{Links: map[string]Entry{}}, err
	}
	if doc.Links == nil {
		doc.Links = map[string]Entry{}
	}
	return doc, nil
}

func saveLocked(sessionsDir string, doc fileDoc) error {
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		return err
	}
	if len(doc.About) == 0 {
		doc.About = json.RawMessage(`{"purpose":"Rebuildable catalog of sessions, parent-child links, pending reviews, and the latest session for each vendor.","authority":"derived from session.json files with temporary coordination state","updated_by":"session ingestion and review workers"}`)
	}
	if len(doc.Sessions) == 0 {
		doc.Sessions = json.RawMessage(`[]`)
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	tmp := Path(sessionsDir) + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, Path(sessionsDir))
}

func pendingPath(sessionsDir string) string {
	return Path(sessionsDir)
}

func loadPendingLocked(sessionsDir string) (pendingDoc, error) {
	raw, err := os.ReadFile(pendingPath(sessionsDir))
	if err != nil {
		return pendingDoc{}, err
	}
	var idx fileDoc
	if err := json.Unmarshal(raw, &idx); err != nil {
		return pendingDoc{}, err
	}
	return pendingDoc{Pending: idx.Pending}, nil
}

func savePendingLocked(sessionsDir string, doc pendingDoc) error {
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		return err
	}
	idx, _ := loadLocked(sessionsDir)
	idx.Pending = doc.Pending
	raw, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	tmp := pendingPath(sessionsDir) + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, pendingPath(sessionsDir))
}
