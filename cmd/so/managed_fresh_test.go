package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/ishanjainn/superopen/internal/graph/client"
)

func TestEnsureFreshGraphSkipsUnmanaged(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if stale, err := ensureFreshGraph(cmd, t.TempDir()); err != nil || stale != "" {
		t.Fatalf("an unmanaged directory must not report a stale graph: %q %v", stale, err)
	}
}

func TestEnsureFreshGraphReturnsRefreshError(t *testing.T) {
	t.Setenv("SUPEROPEN_NO_REFRESH", "")
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".so"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	stale, err := ensureFreshGraph(cmd, root)
	if err == nil {
		t.Fatal("a failed refresh must abort the read")
	}
	if stale != "" {
		t.Fatalf("stale header must not hide the refresh error: %q", stale)
	}
}

func TestEnsureFreshGraphHonorsNoRefresh(t *testing.T) {
	t.Setenv("SUPEROPEN_NO_REFRESH", "1")
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".so"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if stale, err := ensureFreshGraph(cmd, root); err != nil || stale != "" {
		t.Fatalf("SUPEROPEN_NO_REFRESH must skip the index update: %q %v", stale, err)
	}
}

func TestEnsureFreshGraphIndexesEditBeforeRead(t *testing.T) {
	t.Setenv("SUPEROPEN_NO_REFRESH", "")
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required")
	}
	root := t.TempDir()
	git := exec.Command("git", "init")
	git.Dir = root
	if out, err := git.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(root, ".so"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(root, "marker.go")
	body := "package marker\n\nfunc UniqueMarker() {}\n"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if stale, err := ensureFreshGraph(cmd, root); err != nil || stale != "" {
		t.Fatalf("first index should finish, stale=%q err=%v", stale, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".so", "db", "so.db")); err != nil {
		t.Fatalf("index did not write the graph: %v", err)
	}
	if err := os.WriteFile(src, []byte(body+"func SecondMarker() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if stale, err := ensureFreshGraph(cmd, root); err != nil || stale != "" {
		t.Fatalf("the edit should be indexed before the read, stale=%q err=%v", stale, err)
	}
	c, err := client.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	var result api.SearchResult
	if err := c.Call(context.Background(), api.OpSearch, api.SearchRequest{RepoRoot: root, Query: "SecondMarker", Limit: 5}, &result); err != nil {
		t.Fatal(err)
	}
	for _, match := range result.Matches {
		if match.Name == "SecondMarker" {
			return
		}
	}
	t.Fatalf("search missed the edit: %+v", result.Matches)
}
