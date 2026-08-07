package recommend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/eval"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/memory"
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
	if err := Apply(paths, "rec1", Decision{Reason: "The proposed skill now covers the recurring workflow.", Actor: "agent"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(skill); err != nil {
		t.Fatal("skill should exist", err)
	}
	pending, _ := LoadPending(paths)
	if len(pending) != 0 {
		t.Fatal("pending should be empty after apply")
	}
	history, err := LoadHistory(paths)
	if err != nil || len(history) != 1 {
		t.Fatalf("load applied history: len=%d err=%v", len(history), err)
	}
	if history[0].DecisionActor != "agent" || history[0].DecisionReason == "" || history[0].DecisionAt.IsZero() {
		t.Fatalf("decision metadata not persisted: %+v", history[0])
	}
	if err := Revert(paths, "rec1", Decision{Reason: "The skill conflicted with existing guidance.", Actor: "human"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(skill); !os.IsNotExist(err) {
		t.Fatal("skill should be removed on revert when it did not exist")
	}
	history, _ = LoadHistory(paths)
	if history[0].Status != "reverted" || history[0].DecisionActor != "human" || !strings.Contains(history[0].DecisionReason, "conflicted") {
		t.Fatalf("revert decision metadata not persisted: %+v", history[0])
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
	if err := Dismiss(paths, "d1", Decision{Reason: "This duplicates an existing skill.", Actor: "human"}); err != nil {
		t.Fatal(err)
	}
	history, err := LoadHistory(paths)
	if err != nil || len(history) != 1 {
		t.Fatalf("load dismissed history: len=%d err=%v", len(history), err)
	}
	if history[0].DecisionActor != "human" || history[0].DecisionReason != "This duplicates an existing skill." || history[0].DecisionAt.IsZero() {
		t.Fatalf("dismiss decision metadata not persisted: %+v", history[0])
	}
	lessons, err := memory.NewStore(paths).ListLessons()
	if err != nil || len(lessons) == 0 {
		t.Fatalf("dismissal feedback was not retained as memory: len=%d err=%v", len(lessons), err)
	}
	if !strings.Contains(lessons[len(lessons)-1].Text, "duplicates an existing skill") {
		t.Fatalf("dismissal reason missing from memory: %+v", lessons[len(lessons)-1])
	}
	again := Recommendation{
		ID: "d2", Type: "skill", Title: "t", Fingerprint: fp,
		ProposedPath: path, ProposedBody: "# x\n", Evidence: []string{"e2"}, Status: "pending",
	}
	ups, _ := MergePending(paths, []Recommendation{again})
	if len(ups) != 0 {
		t.Fatal("should suppress dismissed fingerprint", ups)
	}
}

func TestDecisionReasonRequired(t *testing.T) {
	root := t.TempDir()
	paths := harness.Resolve(root)
	_ = paths.EnsureDirs()
	r := Recommendation{
		ID: "required", Type: "skill", Title: "t",
		ProposedPath: filepath.Join(paths.SkillsDir, "required.md"),
		ProposedBody: "# required\n", Evidence: []string{"e"}, Status: "pending",
	}
	_, _ = MergePending(paths, []Recommendation{r})
	if err := Apply(paths, "required", Decision{Actor: "agent"}); err == nil {
		t.Fatal("apply should reject an empty decision reason")
	}
	pending, _ := LoadPending(paths)
	if len(pending) != 1 {
		t.Fatalf("rejected decision should remain pending, got %d", len(pending))
	}
}

func TestInsufficientEvidenceDoesNotGenerateRecommendations(t *testing.T) {
	paths := harness.Resolve(t.TempDir())
	_ = paths.EnsureDirs()
	recs, err := Generate(paths, "empty", eval.Result{
		SessionID: "empty", EvidenceStatus: "insufficient",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 0 {
		t.Fatalf("insufficient evidence generated recommendations: %+v", recs)
	}
}
