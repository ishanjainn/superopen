package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/cli"
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

func TestGraphQueryUnmanagedDoesNotCreateSO(t *testing.T) {
	dir := t.TempDir()
	cmd := newRootCommand()
	cmd.SetArgs([]string{"--root", dir, "graph", "query", "marker"})
	err := cmd.Execute()
	if cli.ExitCode(err) != 0 {
		t.Fatalf("exit %d: %v", cli.ExitCode(err), err)
	}
	if err == nil || !strings.Contains(err.Error(), "not a Superopen repo") || strings.Contains(err.Error(), "so init") {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".so")); !os.IsNotExist(statErr) {
		t.Fatal("graph query created .so in an uninited repo")
	}
}
