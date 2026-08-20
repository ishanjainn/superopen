package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ishanjainn/superopen/internal/memory"
)

// Publish builds and verifies a database beside the live database, then swaps
// it into place while holding the per-repository process lock. build must close
// every database handle before returning.
func Publish(ctx context.Context, repoRoot string, build func(context.Context, string) error) (string, error) {
	return publish(ctx, repoRoot, build, false)
}

// PublishNonBlocking is like Publish but returns ErrBuildInProgress immediately
// when another build already holds the lock (used by detached refresh / watch).
func PublishNonBlocking(ctx context.Context, repoRoot string, build func(context.Context, string) error) (string, error) {
	return publish(ctx, repoRoot, build, true)
}

func publish(ctx context.Context, repoRoot string, build func(context.Context, string) error, nonBlocking bool) (string, error) {
	paths, err := CachePaths(repoRoot)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(paths.Root, 0o700); err != nil {
		return "", err
	}
	var unlock func()
	if nonBlocking {
		unlock, err = tryAcquireBuildLock(paths.Lock)
		if errors.Is(err, ErrBuildInProgress) {
			return "", ErrBuildInProgress
		}
	} else {
		unlock, err = acquireBuildLock(paths.Lock)
	}
	if err != nil {
		return "", err
	}
	defer unlock()

	stage, err := os.CreateTemp(paths.Root, ".graph-*.db")
	if err != nil {
		return "", err
	}
	stagePath := stage.Name()
	if err := stage.Close(); err != nil {
		_ = os.Remove(stagePath)
		return "", err
	}
	if err := os.Remove(stagePath); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	defer removeDatabaseFamily(stagePath)

	if err := build(ctx, stagePath); err != nil {
		return "", fmt.Errorf("build staged graph: %w", err)
	}
	staged, err := OpenReadOnly(stagePath)
	if err != nil {
		return "", fmt.Errorf("open staged graph: %w", err)
	}
	verifyErr := staged.Verify(ctx)
	closeErr := staged.Close()
	if verifyErr != nil {
		return "", fmt.Errorf("verify staged graph: %w", verifyErr)
	}
	if closeErr != nil {
		return "", closeErr
	}

	if _, err := os.Stat(paths.Database); err == nil {
		if err := memory.CopyInto(paths.Database, stagePath); err != nil {
			return "", fmt.Errorf("preserve memory across graph publish: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}

	backup := paths.Database + ".previous"
	removeDatabaseFamily(backup)
	hadPrior := false
	if _, err := os.Stat(paths.Database); err == nil {
		hadPrior = true
		if err := os.Rename(paths.Database, backup); err != nil {
			return "", fmt.Errorf("preserve live graph: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.Rename(stagePath, paths.Database); err != nil {
		if hadPrior {
			_ = os.Rename(backup, paths.Database)
		}
		return "", fmt.Errorf("publish graph: %w", err)
	}
	if _, err := os.Stat(stagePath + ".key"); err == nil {
		_ = os.Rename(stagePath+".key", paths.Database+".key")
	}
	removeDatabaseFamily(backup)
	return paths.Database, nil
}

func removeDatabaseFamily(path string) {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = os.Remove(filepath.Clean(path) + suffix)
	}
}
