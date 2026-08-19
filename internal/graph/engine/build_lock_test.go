package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTryAcquireBuildLockBusy(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "build.lock")
	unlock, err := acquireBuildLock(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	_, err = tryAcquireBuildLock(lockPath)
	if !errors.Is(err, ErrBuildInProgress) {
		t.Fatalf("expected ErrBuildInProgress, got %v", err)
	}
}

func TestPublishNonBlockingWhenBusy(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".so", "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	paths, err := CachePaths(repo)
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := acquireBuildLock(paths.Lock)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	if !BuildBusy(repo) {
		t.Fatal("expected BuildBusy while lock held")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = PublishNonBlocking(ctx, repo, func(context.Context, string) error {
		t.Fatal("build should not run")
		return nil
	})
	if !errors.Is(err, ErrBuildInProgress) {
		t.Fatalf("expected ErrBuildInProgress, got %v", err)
	}
}

func TestBuildBusyFalseWhenFree(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".so", "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	if BuildBusy(repo) {
		t.Fatal("expected free lock")
	}
}
