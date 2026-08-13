package memory

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/ishanjainn/superopen/internal/userpaths"
)

const stateLockWait = 2 * time.Second

// mutateState serializes read-modify-write operations across CLI, hook,
// review-worker, and UI processes. A directory lock is portable across the
// three supported desktop operating systems and cannot leave a partial JSON
// document behind.
func (s *Store) mutateState(fn func(*stateFile) error) error {
	if err := os.MkdirAll(s.Paths.MemoryDir, 0o755); err != nil {
		return err
	}
	unlock, err := acquireDirLock(s.stateLockPath(), stateLockWait)
	if err != nil {
		return err
	}
	defer unlock()
	st, err := s.readState()
	if err != nil {
		return err
	}
	if err := fn(&st); err != nil {
		return err
	}
	prunePatterns(&st, time.Now().UTC())
	return s.writeState(st)
}

func (s *Store) stateLockPath() string {
	if dir, err := userpaths.RuntimeDir(s.Paths.RepoRoot); err == nil {
		return filepath.Join(dir, "memory-state.lock")
	}
	sum := sha256.Sum256([]byte(filepath.Clean(s.Paths.RepoRoot)))
	return filepath.Join(os.TempDir(), "superopen", "runtime", fmt.Sprintf("%x", sum[:12]), "memory-state.lock")
}

func prunePatterns(st *stateFile, now time.Time) {
	const maxPatterns = 1000
	keep := st.Patterns[:0]
	for _, p := range st.Patterns {
		protected := p.ExplicitWorkflow || p.Status == "applied" || len(p.VerifiedSessions) > 0
		inactive := now.Sub(p.LastObservedAt) >= 180*24*time.Hour
		if !protected && inactive && p.Confidence < .70 && p.RetrievalCount == 0 {
			continue
		}
		keep = append(keep, p)
	}
	st.Patterns = keep
	if len(st.Patterns) <= maxPatterns {
		return
	}
	sort.SliceStable(st.Patterns, func(i, j int) bool {
		priority := func(p Pattern) int {
			if p.ExplicitWorkflow || p.Status == "applied" || len(p.VerifiedSessions) > 0 {
				return 2
			}
			if p.Status == "dismissed" || p.Status == "superseded" || p.Status == "obsolete" {
				return 0
			}
			return 1
		}
		pi, pj := priority(st.Patterns[i]), priority(st.Patterns[j])
		if pi != pj {
			return pi > pj
		}
		return st.Patterns[i].LastObservedAt.After(st.Patterns[j].LastObservedAt)
	})
	st.Patterns = st.Patterns[:maxPatterns]
}

func acquireDirLock(path string, wait time.Duration) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(wait)
	for {
		err := os.Mkdir(path, 0o700)
		if err == nil {
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if info, statErr := os.Stat(path); statErr == nil && time.Since(info.ModTime()) > 30*time.Second {
			_ = os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			return nil, errors.New("memory state lock timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	// Windows does not replace an existing destination with Rename. Removing
	// the old file happens only after the complete temporary file is durable.
	if err := os.Rename(tmp, path); err == nil {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(tmp, path)
}
