package engine

import (
	"errors"
)

// ErrBuildInProgress is returned when a non-blocking build lock cannot be
// acquired because another process already holds it.
var ErrBuildInProgress = errors.New("graph build already in progress")

// BuildBusy reports whether another process holds the repository build lock.
// It briefly acquires and releases a non-blocking exclusive lock when free.
func BuildBusy(repoRoot string) bool {
	paths, err := CachePaths(repoRoot)
	if err != nil {
		return false
	}
	unlock, err := tryAcquireBuildLock(paths.Lock)
	if errors.Is(err, ErrBuildInProgress) {
		return true
	}
	if err != nil || unlock == nil {
		return false
	}
	unlock()
	return false
}
