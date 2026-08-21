// Package buildpool caps concurrent graph builds across repositories.
package buildpool

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/paths"
)

const (
	defaultSlots = 2
	slotEnv      = "SUPEROPEN_BUILD_SLOTS"
	staleAge     = 4 * time.Hour
)

// ErrFull is returned when every slot is held and acquire is non-blocking.
var ErrFull = errors.New("graph build pool full")

// Slot is one occupied global build slot.
type Slot struct {
	Index     int
	PID       int
	Repo      string
	StartedAt time.Time
}

// SlotCount is the configured pool size. 0 means unlimited.
func SlotCount() int {
	raw := strings.TrimSpace(os.Getenv(slotEnv))
	if raw == "" {
		return defaultSlots
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return defaultSlots
	}
	return n
}

// TryAcquire takes a slot without waiting. Caller must run the returned unlock.
func TryAcquire(repoRoot string) (func(), error) {
	n := SlotCount()
	if n == 0 {
		return func() {}, nil
	}
	dir, err := slotDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	for i := 0; i < n; i++ {
		path := filepath.Join(dir, fmt.Sprintf("%d.lock", i))
		unlock, err := tryLockSlot(path)
		if err != nil {
			continue
		}
		writeMeta(path, os.Getpid(), repoRoot)
		return func() {
			clearMeta(path)
			unlock()
		}, nil
	}
	return nil, ErrFull
}

// Acquire waits until a slot is free. Used only by explicit user builds (so init).
func Acquire(repoRoot string) (func(), error) {
	for {
		unlock, err := TryAcquire(repoRoot)
		if err == nil {
			return unlock, nil
		}
		if !errors.Is(err, ErrFull) {
			return nil, err
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Full reports whether every slot is occupied. It never holds a slot.
func Full() bool {
	if SlotCount() == 0 {
		return false
	}
	unlock, err := TryAcquire("")
	if err == nil {
		unlock()
		return false
	}
	return errors.Is(err, ErrFull)
}

// List returns occupied slots for `so graph builds status`.
func List() ([]Slot, error) {
	n := SlotCount()
	if n == 0 {
		return nil, nil
	}
	dir, err := slotDir()
	if err != nil {
		return nil, err
	}
	var out []Slot
	for i := 0; i < n; i++ {
		path := filepath.Join(dir, fmt.Sprintf("%d.lock", i))
		unlock, err := tryLockSlot(path)
		if err == nil {
			unlock()
			continue
		}
		meta := readMeta(path)
		if meta.PID > 0 && !processAlive(meta.PID) {
			continue
		}
		if !meta.StartedAt.IsZero() && time.Since(meta.StartedAt) > staleAge && (meta.PID <= 0 || !processAlive(meta.PID)) {
			continue
		}
		meta.Index = i
		out = append(out, meta)
	}
	return out, nil
}

func slotDir() (string, error) {
	dir, err := paths.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "build-slots"), nil
}

func writeMeta(path string, pid int, repo string) {
	started := time.Now().UTC().Format(time.RFC3339)
	body := fmt.Sprintf("pid=%d\nrepo=%s\nstarted=%s\n", pid, repo, started)
	_ = os.WriteFile(metaPath(path), []byte(body), 0o600)
}

func clearMeta(path string) {
	_ = os.Remove(metaPath(path))
}

func metaPath(lockPath string) string {
	return lockPath + ".meta"
}

func readMeta(lockPath string) Slot {
	body, err := os.ReadFile(metaPath(lockPath))
	if err != nil {
		return Slot{}
	}
	var s Slot
	for _, line := range strings.Split(string(body), "\n") {
		key, val, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case "pid":
			s.PID, _ = strconv.Atoi(val)
		case "repo":
			s.Repo = val
		case "started":
			s.StartedAt, _ = time.Parse(time.RFC3339, val)
		}
	}
	return s
}
