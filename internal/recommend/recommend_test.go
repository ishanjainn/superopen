package recommend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/superopen/so/internal/harness"
)

func TestFingerprintDedupeAcrossSessions(t *testing.T) {
	root := t.TempDir()
	paths := harness.Resolve(root)
	_ = os.MkdirAll(filepath.Dir(paths.PendingRecs), 0o755)
	_ = os.MkdirAll(paths.SkillsDir, 0o755)
	_ = os.MkdirAll(paths.MemoryDir, 0o755)

	path := filepath.Join(paths.SkillsDir, "prefer-harness-before-search.md")
	r1 := Recommendation{
		ID: "a", Type: "skill", Title: "Follow guides",
		Fingerprint: FingerprintKey("skill", path, "prefer-harness"),
		ProposedPath: path, ProposedBody: "# x\n", Evidence: []string{"search=1"},
		RelatedSessions: []string{"s1"}, Status: "pending",
	}
	r2 := Recommendation{
		ID: "b", Type: "skill", Title: "Follow guides",
		Fingerprint: FingerprintKey("skill", path, "prefer-harness"),
		ProposedPath: path, ProposedBody: "# x\n", Evidence: []string{"search=2"},
		RelatedSessions: []string{"s2"}, Status: "pending",
	}
	if _, err := MergePending(paths, []Recommendation{r1}); err != nil {
		t.Fatal(err)
	}
	if _, err := MergePending(paths, []Recommendation{r2}); err != nil {
		t.Fatal(err)
	}
	pending, err := LoadPending(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("want 1 pending, got %d", len(pending))
	}
	if len(pending[0].RelatedSessions) != 2 {
		t.Fatalf("want 2 sessions, got %v", pending[0].RelatedSessions)
	}
}

func TestApplyAndRevert(t *testing.T) {
	root := t.TempDir()
	paths := harness.Resolve(root)
	_ = os.MkdirAll(filepath.Dir(paths.PendingRecs), 0o755)
	_ = os.MkdirAll(paths.SkillsDir, 0o755)
	_ = os.MkdirAll(paths.MemoryDir, 0o755)
	_ = paths.EnsureDirs()

	skill := filepath.Join(paths.SkillsDir, "new-skill.md")
	r := Recommendation{
		ID: "rec1", Type: "skill", Title: "Add skill", Rationale: "because",
		Fingerprint: FingerprintKey("skill", skill, "new"),
		ProposedPath: skill, ProposedBody: "# New skill\n\nDo X.\n",
		Evidence: []string{"harness_use=0.1"}, Status: "pending",
	}
	if _, err := MergePending(paths, []Recommendation{r}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(paths, "rec1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(skill); err != nil {
		t.Fatal("skill should exist", err)
	}
	pending, _ := LoadPending(paths)
	if len(pending) != 0 {
		t.Fatal("pending should be empty after apply")
	}
	if err := Revert(paths, "rec1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(skill); !os.IsNotExist(err) {
		t.Fatal("skill should be removed on revert when it did not exist")
	}
}

func TestSuppressAfterDismiss(t *testing.T) {
	root := t.TempDir()
	paths := harness.Resolve(root)
	_ = os.MkdirAll(filepath.Dir(paths.PendingRecs), 0o755)
	_ = os.MkdirAll(paths.SkillsDir, 0o755)
	path := filepath.Join(paths.SkillsDir, "x.md")
	fp := FingerprintKey("skill", path, "x")
	r := Recommendation{
		ID: "d1", Type: "skill", Title: "t", Fingerprint: fp,
		ProposedPath: path, ProposedBody: "# x\n", Evidence: []string{"e"}, Status: "pending",
	}
	_, _ = MergePending(paths, []Recommendation{r})
	_ = Dismiss(paths, "d1")
	again := Recommendation{
		ID: "d2", Type: "skill", Title: "t", Fingerprint: fp,
		ProposedPath: path, ProposedBody: "# x\n", Evidence: []string{"e2"}, Status: "pending",
	}
	ups, _ := MergePending(paths, []Recommendation{again})
	if len(ups) != 0 {
		t.Fatal("should suppress dismissed fingerprint", ups)
	}
}
