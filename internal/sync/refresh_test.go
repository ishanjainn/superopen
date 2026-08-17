package sync

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/graph"
	"github.com/ishanjainn/superopen/internal/harness"
)

func TestSyncGraphStartsAgentContinuationInsteadOfDirectRefresh(t *testing.T) {
	repo := t.TempDir()
	paths := harness.Resolve(repo)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	previousExisting := existingSemanticGraph
	previousStart := startSemanticGraph
	previousRefresh := refreshGraph
	existingSemanticGraph = func(harness.Paths) (graph.Result, bool) { return graph.Result{}, false }
	startSemanticGraph = func(_ context.Context, _ string, opts graph.SemanticStartOptions) (graph.SemanticRun, error) {
		if !opts.Deep {
			t.Fatal("configured deep mode was not propagated through sync")
		}
		return graph.SemanticRun{RunID: "run-test"}, nil
	}
	refreshGraph = func(string, harness.Paths, bool, string) (graph.Result, error) {
		t.Fatal("agent-backed sync called direct RefreshAtomic")
		return graph.Result{}, nil
	}
	t.Cleanup(func() {
		existingSemanticGraph = previousExisting
		startSemanticGraph = previousStart
		refreshGraph = previousRefresh
	})
	err := syncGraph(context.Background(), repo, paths, false, "agent", "deep")
	var continuation *GraphContinuationError
	if !errors.As(err, &continuation) || continuation.RunID != "run-test" {
		t.Fatalf("syncGraph error = %v, want run-test continuation", err)
	}
}

func TestIsIndexablePath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{".so/graph/graph.json", false},
		{".so/memory/context.md", false},
		{".git/hooks/post-commit", false},
		{"README.md", false},
		{"docs/guide.md", false},
		{"go.sum", false},
		{"web/package-lock.json", false},
		{"internal/graph/graph.go", true},
		{"cmd/so/main.go", true},
		{"web/src/app/page.tsx", true},
	}
	for _, c := range cases {
		if got := isIndexablePath(c.path); got != c.want {
			t.Errorf("isIndexablePath(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func headSHA(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestIndexableFilesChangedIgnoresSOOnlyCommits(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-q", "-m", "initial")
	firstSHA := trimNL(headSHA(t, root))

	// A commit that only touches .so/ (e.g. a prior graph rebuild, or a
	// session-port commit) must not be treated as a reason to rebuild again.
	if err := os.MkdirAll(filepath.Join(root, ".so", "graph"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".so", "graph", "graph.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-q", "-m", "graph rebuild")
	secondSHA := trimNL(headSHA(t, root))

	if indexableFilesChanged(root, firstSHA, secondSHA) {
		t.Fatal("expected a .so/-only commit to report no indexable changes")
	}

	// A real source change must still trigger a rebuild.
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-q", "-m", "add main func")
	thirdSHA := trimNL(headSHA(t, root))

	if !indexableFilesChanged(root, secondSHA, thirdSHA) {
		t.Fatal("expected a source change to report indexable changes")
	}
}

func trimNL(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
