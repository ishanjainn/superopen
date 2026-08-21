package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestGraphRefreshDetachEmptyStdout(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".so"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := newRootCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--root", dir, "graph", "refresh", "--detach"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("detach refresh stdout must be empty so Claude does not ingest it, got %q", got)
	}
}
