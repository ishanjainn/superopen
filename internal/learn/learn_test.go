package learn

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/harness"
)

func TestMineTranscriptWritesLessonAndRec(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, ".so")
	paths := harness.Resolve(dir)
	_ = os.MkdirAll(paths.MemoryDir, 0o755)
	_ = os.MkdirAll(paths.SkillsDir, 0o755)
	_ = os.MkdirAll(filepath.Dir(paths.PendingRecs), 0o755)
	_ = os.WriteFile(filepath.Join(root, "config.yaml"), []byte("version: 1\n"), 0o644)

	lines := []string{
		"please always run tests before finishing a PR",
		"noise line without signal",
	}
	lessons, recs, err := MineTranscript(paths, "sess1", lines, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(lessons) == 0 {
		t.Fatal("expected at least one lesson")
	}
	if len(recs) == 0 {
		t.Fatal("expected skill recommendation")
	}
	if recs[0].ProposedBody == "" {
		t.Fatal("expected heuristic skill body")
	}
}
