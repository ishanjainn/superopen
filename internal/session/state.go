package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/superopen/so/internal/harness"
)

// Phase is the lifecycle phase of an active coding session.
type Phase string

const (
	PhaseActive Phase = "active"
	PhaseIdle   Phase = "idle"
	PhaseEnded  Phase = "ended"
)

// State tracks live session machine state (branch, worktree, base SHA).
// Persisted under .so/session-state/<id>.json (not the transcript store).
type State struct {
	SessionID  string    `json:"session_id"`
	Vendor     string    `json:"vendor"`
	Phase      Phase     `json:"phase"`
	Branch     string    `json:"branch,omitempty"`
	BaseSHA    string    `json:"base_sha,omitempty"`
	HeadSHA    string    `json:"head_sha,omitempty"`
	WorktreeID string    `json:"worktree_id,omitempty"`
	RepoRoot   string    `json:"repo_root,omitempty"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// StateStore reads/writes session state files for one harness.
type StateStore struct {
	Dir string
}

func NewStateStore(paths harness.Paths) *StateStore {
	return &StateStore{Dir: filepath.Join(paths.Root, "session-state")}
}

func (s *StateStore) path(id string) string {
	return filepath.Join(s.Dir, id+".json")
}

func (s *StateStore) Save(st State) error {
	if st.SessionID == "" {
		return fmt.Errorf("session_id required")
	}
	if st.Phase == "" {
		st.Phase = PhaseActive
	}
	st.UpdatedAt = time.Now().UTC()
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path(st.SessionID), data, 0o600)
}

func (s *StateStore) Get(id string) (State, error) {
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		return State{}, err
	}
	var st State
	return st, json.Unmarshal(data, &st)
}

func (s *StateStore) List() ([]State, error) {
	ents, err := os.ReadDir(s.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []State
	for _, e := range ents {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		st, err := s.Get(e.Name()[:len(e.Name())-5])
		if err != nil {
			continue
		}
		out = append(out, st)
	}
	return out, nil
}

func (s *StateStore) ListActive() ([]State, error) {
	all, err := s.List()
	if err != nil {
		return nil, err
	}
	var out []State
	for _, st := range all {
		if st.Phase == PhaseActive || st.Phase == PhaseIdle {
			out = append(out, st)
		}
	}
	return out, nil
}

func (s *StateStore) End(id string) error {
	st, err := s.Get(id)
	if err != nil {
		return err
	}
	st.Phase = PhaseEnded
	return s.Save(st)
}

func (s *StateStore) Delete(id string) error {
	err := os.Remove(s.path(id))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// WarnConcurrent returns a warning if other active sessions share the worktree.
func (s *StateStore) WarnConcurrent(exceptID, worktreeID string) string {
	active, err := s.ListActive()
	if err != nil {
		return ""
	}
	n := 0
	for _, st := range active {
		if st.SessionID == exceptID {
			continue
		}
		if worktreeID == "" || st.WorktreeID == worktreeID || (st.WorktreeID == "" && worktreeID == "") {
			n++
		}
	}
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("warning: %d other active session(s) in this worktree", n)
}
