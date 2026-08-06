// Package checkpoint stores restorable file snapshots for a session under
// .so/sessions/<id>/checkpoints/<n>/.
package checkpoint

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/superopen/so/internal/harness"
	"github.com/superopen/so/internal/redact"
)

// Meta describes one checkpoint.
type Meta struct {
	ID               int       `json:"id"`
	SessionID        string    `json:"session_id"`
	CreatedAt        time.Time `json:"created_at"`
	Label            string    `json:"label,omitempty"`
	TranscriptOffset int64     `json:"transcript_offset,omitempty"`
	Files            []string  `json:"files"`
}

// Store manages checkpoints for sessions in one harness.
type Store struct {
	Paths harness.Paths
}

func NewStore(paths harness.Paths) *Store {
	return &Store{Paths: paths}
}

func (s *Store) dir(sessionID string) string {
	return filepath.Join(s.Paths.SessionDir(sessionID), "checkpoints")
}

func (s *Store) nextID(sessionID string) (int, error) {
	ents, err := os.ReadDir(s.dir(sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return 1, nil
		}
		return 0, err
	}
	max := 0
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		n, err := strconv.Atoi(e.Name())
		if err == nil && n > max {
			max = n
		}
	}
	return max + 1, nil
}

// Create snapshots the given relative file paths from repoRoot into a new checkpoint.
func (s *Store) Create(sessionID, repoRoot, label string, relFiles []string, transcriptOffset int64) (Meta, error) {
	id, err := s.nextID(sessionID)
	if err != nil {
		return Meta{}, err
	}
	base := filepath.Join(s.dir(sessionID), strconv.Itoa(id))
	filesDir := filepath.Join(base, "files")
	if err := os.MkdirAll(filesDir, 0o755); err != nil {
		return Meta{}, err
	}
	var saved []string
	for _, rel := range relFiles {
		rel = filepath.Clean(rel)
		if rel == "." || stringsHasDotDot(rel) {
			continue
		}
		src := filepath.Join(repoRoot, rel)
		data, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		// Redact secrets before persisting snapshot bytes.
		text := redact.String(string(data))
		dst := filepath.Join(filesDir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			continue
		}
		if err := os.WriteFile(dst, []byte(text), 0o644); err != nil {
			continue
		}
		saved = append(saved, rel)
	}
	sort.Strings(saved)
	m := Meta{
		ID:               id,
		SessionID:        sessionID,
		CreatedAt:        time.Now().UTC(),
		Label:            label,
		TranscriptOffset: transcriptOffset,
		Files:            saved,
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(base, "meta.json"), b, 0o644); err != nil {
		return Meta{}, err
	}
	return m, nil
}

func stringsHasDotDot(p string) bool {
	for _, part := range filepath.SplitList(p) {
		_ = part
	}
	cleaned := filepath.ToSlash(p)
	return cleaned == ".." || len(cleaned) >= 3 && (cleaned[:3] == "../" || containsDotDot(cleaned))
}

func containsDotDot(p string) bool {
	for _, seg := range splitSlash(p) {
		if seg == ".." {
			return true
		}
	}
	return false
}

func splitSlash(p string) []string {
	var out []string
	cur := ""
	for _, r := range p {
		if r == '/' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// List returns checkpoints for a session (newest first).
func (s *Store) List(sessionID string) ([]Meta, error) {
	ents, err := os.ReadDir(s.dir(sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Meta
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		m, err := s.Get(sessionID, e.Name())
		if err != nil {
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

// Get loads checkpoint meta by id (string or int).
func (s *Store) Get(sessionID, id string) (Meta, error) {
	path := filepath.Join(s.dir(sessionID), id, "meta.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Meta{}, err
	}
	var m Meta
	return m, json.Unmarshal(data, &m)
}

// Restore writes checkpoint files back into repoRoot (overwrites).
func (s *Store) Restore(sessionID, id, repoRoot string) error {
	m, err := s.Get(sessionID, id)
	if err != nil {
		return err
	}
	filesDir := filepath.Join(s.dir(sessionID), id, "files")
	for _, rel := range m.Files {
		src := filepath.Join(filesDir, rel)
		dst := filepath.Join(repoRoot, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		in, err := os.Open(src)
		if err != nil {
			return err
		}
		out, err := os.Create(dst)
		if err != nil {
			in.Close()
			return err
		}
		_, copyErr := io.Copy(out, in)
		in.Close()
		out.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

// CreateFromFootprint snapshots edited files recorded in session footprint.
func (s *Store) CreateFromFootprint(sessionID, repoRoot, label string) (Meta, error) {
	fpPath := filepath.Join(s.Paths.SessionDir(sessionID), "footprint.json")
	data, err := os.ReadFile(fpPath)
	if err != nil {
		return Meta{}, fmt.Errorf("footprint: %w", err)
	}
	var fp struct {
		Files []struct {
			Path  string `json:"path"`
			State string `json:"state"`
		} `json:"files"`
	}
	if err := json.Unmarshal(data, &fp); err != nil {
		return Meta{}, err
	}
	var rels []string
	for _, f := range fp.Files {
		if f.State == "edited" || f.State == "read" {
			rels = append(rels, f.Path)
		}
	}
	return s.Create(sessionID, repoRoot, label, rels, 0)
}
