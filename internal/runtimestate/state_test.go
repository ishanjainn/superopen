package runtimestate_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/artifactmeta"
	"github.com/ishanjainn/superopen/internal/runtimestate"
)

func TestTouchIfStaleUsesOneDescribedStateFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "cache"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	repo := t.TempDir()

	run, err := runtimestate.TouchIfStale(repo, "idle_sweep", time.Hour)
	if err != nil || !run {
		t.Fatalf("first touch = %v, %v", run, err)
	}
	run, err = runtimestate.TouchIfStale(repo, "idle_sweep", time.Hour)
	if err != nil || run {
		t.Fatalf("second touch = %v, %v", run, err)
	}
	run, err = runtimestate.TouchIfStale(repo, "approval_mismatch:test", time.Hour)
	if err != nil || !run {
		t.Fatalf("different marker = %v, %v", run, err)
	}
	path, err := runtimestate.Path(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := artifactmeta.Validate(path); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "state.json" {
		t.Fatalf("runtime artifacts = %v, want only state.json", entries)
	}
}
