// Package runtimestate consolidates small machine-local coordination markers.
package runtimestate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/ishanjainn/superopen/internal/artifactmeta"
	"github.com/ishanjainn/superopen/internal/userpaths"
)

const fileName = "state.json"

var about = artifactmeta.About{
	Purpose:   "Machine-local throttles and debounce markers for one Superopen repository.",
	Authority: "temporary runtime coordination state",
	UpdatedBy: "Superopen CLI and coding-agent hooks",
}

type fileState struct {
	About   artifactmeta.About   `json:"_about"`
	Markers map[string]time.Time `json:"markers,omitempty"`
}

// Path returns the one consolidated runtime-state file for repoRoot.
func Path(repoRoot string) (string, error) {
	dir, err := userpaths.RuntimeDir(repoRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fileName), nil
}

// TouchIfStale atomically records key when it is absent or older than window.
// It returns true only to the caller that should perform the throttled work.
func TouchIfStale(repoRoot, key string, window time.Duration) (bool, error) {
	path, err := Path(repoRoot)
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	release, err := acquire(path + ".lock")
	if err != nil {
		return false, err
	}
	defer release()

	st := fileState{About: about, Markers: map[string]time.Time{}}
	if data, readErr := os.ReadFile(path); readErr == nil {
		_ = json.Unmarshal(data, &st)
	}
	if st.Markers == nil {
		st.Markers = map[string]time.Time{}
	}
	now := time.Now().UTC()
	if previous := st.Markers[key]; !previous.IsZero() && window > 0 && now.Sub(previous) < window {
		return false, nil
	}
	st.About = about
	st.Markers[key] = now
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return false, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".runtime-state-*")
	if err != nil {
		return false, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err = tmp.Write(append(data, '\n')); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return false, err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return false, err
	}
	return true, nil
}

func acquire(lock string) (func(), error) {
	for attempt := 0; attempt < 25; attempt++ {
		if err := os.Mkdir(lock, 0o700); err == nil {
			return func() { _ = os.Remove(lock) }, nil
		} else if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if st, err := os.Stat(lock); err == nil && time.Since(st.ModTime()) > 30*time.Second {
			_ = os.Remove(lock)
			continue
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil, errors.New("runtime state is busy")
}
