package discover_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/discover"
	"github.com/ishanjainn/superopen/internal/harness"
)

func TestCollectAgentFilesAndProfile(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Agents\n\n- Never commit secrets to git\n- Always run tests before finishing\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Review\n\n## Concurrency\n\nAlways use mutex for shared caches. Never share mutable labels across goroutines.\n\n## Don't\n\n- Never ignore rate limit headers from APIs\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, ".cursor", "rules"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, ".cursor", "rules", "pr.mdc"), []byte("---\ntitle: PR\n---\n# PR Title\n\nPR titles must follow Plugin (feat): description\n"), 0o644)

	paths := harness.Resolve(dir)
	_ = paths.EnsureDirs()
	_ = os.WriteFile(paths.GraphJSON, []byte(`{"nodes":[{"id":"pkg/a.go","source_file":"pkg/a.go"},{"id":"cmd/main.go","source_file":"cmd/main.go"}],"edges":[]}`), 0o644)

	agents := discover.CollectAgentFiles(dir)
	if len(agents) < 2 {
		t.Fatalf("expected agent files, got %d", len(agents))
	}
	p := discover.BuildProfile(dir, paths, "Go", "- pkg")
	if len(p.DerivedRules) == 0 {
		t.Fatal("expected derived rules")
	}
	joined := strings.ToLower(strings.Join(p.DerivedRules, "\n"))
	if !strings.Contains(joined, "never commit secrets") {
		t.Fatalf("missing secrets rule: %v", p.DerivedRules)
	}
	if strings.Contains(joined, "ignore rate limit") {
		t.Fatalf("should skip DON'T section rules: %v", p.DerivedRules)
	}
	if p.Graph.NodeCount != 2 {
		t.Fatalf("graph nodes=%d", p.Graph.NodeCount)
	}
	if p.Graph.Languages["go"] != 2 {
		t.Fatalf("go lang count=%d", p.Graph.Languages["go"])
	}
}
