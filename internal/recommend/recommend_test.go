package recommend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/eval"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/session"
)

func TestFingerprintDedupeAcrossSessions(t *testing.T) {
	root := t.TempDir()
	paths := harness.Resolve(root)
	_ = os.MkdirAll(filepath.Dir(paths.PendingRecs), 0o755)
	_ = os.MkdirAll(paths.SkillsDir, 0o755)
	_ = os.MkdirAll(paths.MemoryDir, 0o755)

	path := paths.SkillSKILL("prefer-harness-before-search")
	r1 := Recommendation{
		ID: "a", Type: "skill", Title: "Follow guides",
		Fingerprint:  FingerprintKey("skill", path, "prefer-harness"),
		ProposedPath: path, ProposedBody: "# x\n", Evidence: []string{"search=1"},
		RelatedSessions: []string{"s1"}, Status: "pending",
	}
	r2 := Recommendation{
		ID: "b", Type: "skill", Title: "Follow guides",
		Fingerprint:  FingerprintKey("skill", path, "prefer-harness"),
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

	_ = session.NewStore(paths).Start(session.Meta{ID: "s1", Vendor: "codex"})
	skill := filepath.Join(root, ".codex", "skills", "new-skill", "SKILL.md")
	r := Recommendation{
		ID: "rec1", SessionID: "s1", Type: "skill", Title: "Add skill", Rationale: "because",
		Fingerprint:  FingerprintKey("skill", skill, "new"),
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
	path := paths.SkillSKILL("x")
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
		ProposedPath: paths.SkillSKILL("required"),
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

func TestNewSkillAutoApplyRequiresThreeVerifiedSessions(t *testing.T) {
	root := t.TempDir()
	paths := harness.Resolve(root)
	_ = paths.EnsureDirs()
	path := filepath.Join(root, ".codex", "skills", "focused-tests", "SKILL.md")
	base := Recommendation{Type: "skill", TargetType: "skill", ChangeKind: "create", Vendor: "codex", ProposedPath: path, ProposedBody: "# Focused tests\n", Evidence: []string{"workflow repeated"}, Verified: true, AutoApplyAfter: 3}
	for n := 1; n <= 3; n++ {
		base.OccurrenceCount = n
		allowed, _ := ShouldAutoApply(paths, base)
		if allowed != (n == 3) {
			t.Fatalf("occurrences=%d allowed=%v", n, allowed)
		}
	}
	base.OccurrenceCount = 3
	base.Verified = false
	if allowed, _ := ShouldAutoApply(paths, base); allowed {
		t.Fatal("unverified workflow must not auto-create a skill")
	}
	base.Verified = true
	base.OccurrenceCount = 1
	base.ExplicitWorkflow = true
	if allowed, _ := ShouldAutoApply(paths, base); !allowed {
		t.Fatal("explicit durable workflow should satisfy recurrence")
	}
}

func TestNewRulesAndDocsUseCreationThreshold(t *testing.T) {
	root := t.TempDir()
	paths := harness.Resolve(root)
	for _, rec := range []Recommendation{
		{Type: "rules", ChangeKind: "create", Vendor: "codex", ProposedPath: filepath.Join(root, ".codex", "rules", "testing.md")},
		{Type: "docs", ChangeKind: "create", Vendor: "codex", ProposedPath: filepath.Join(root, "internal", "AGENTS.md")},
	} {
		rec.Verified = true
		rec.OccurrenceCount = 1
		if allowed, _ := ShouldAutoApply(paths, rec); allowed {
			t.Fatalf("first occurrence auto-created %+v", rec)
		}
		rec.OccurrenceCount = 3
		if allowed, reason := ShouldAutoApply(paths, rec); !allowed {
			t.Fatalf("threshold rejected %+v: %s", rec, reason)
		}
		if got := autoApplyThreshold(rec.Type, rec.ChangeKind); got != 3 {
			t.Fatalf("threshold=%d for %s", got, rec.Type)
		}
	}
}

func TestAutoApplyProtectsPolicySharedAgentsAndManagedSkill(t *testing.T) {
	paths := harness.Resolve(t.TempDir())
	cases := []Recommendation{
		{Type: "guardrail", ChangeKind: "update", Vendor: "codex", ProposedPath: paths.GuardrailsFile},
		{Type: "skill", ChangeKind: "update", Vendor: "codex", ProposedPath: filepath.Join(paths.RepoRoot, ".agents", "skills", "x", "SKILL.md")},
		{Type: "skill", ChangeKind: "update", Vendor: "codex", ProposedPath: filepath.Join(paths.RepoRoot, ".codex", "skills", "so", "SKILL.md")},
		{Type: "rules", ChangeKind: "remove", Vendor: "codex", ProposedPath: filepath.Join(paths.RepoRoot, ".codex", "rules", "x.md")},
	}
	for _, rec := range cases {
		if allowed, _ := ShouldAutoApply(paths, rec); allowed {
			t.Fatalf("protected recommendation auto-applied: %+v", rec)
		}
	}
}

func TestApplyRejectsAnotherVendorTree(t *testing.T) {
	root := t.TempDir()
	paths := harness.Resolve(root)
	_ = paths.EnsureDirs()
	_ = session.NewStore(paths).Start(session.Meta{ID: "codex-session", Vendor: "codex"})
	r := Recommendation{
		ID: "cross-vendor", SessionID: "codex-session", Vendor: "codex", Type: "skill", ChangeKind: "create",
		ProposedPath: filepath.Join(root, ".cursor", "skills", "wrong", "SKILL.md"), ProposedBody: "# Wrong\n", Evidence: []string{"e"}, Status: "pending",
	}
	_, _ = MergePending(paths, []Recommendation{r})
	if err := Apply(paths, r.ID, Decision{Reason: "test ownership", Actor: "human"}); err == nil || !strings.Contains(err.Error(), "outside codex") {
		t.Fatalf("cross-vendor apply should fail, got %v", err)
	}
}

func TestRecurringDraftProgressesAcrossSameVendorSessions(t *testing.T) {
	root := t.TempDir()
	paths := harness.Resolve(root)
	_ = paths.EnsureDirs()
	path := filepath.Join(root, ".codex", "skills", "focused-tests", "SKILL.md")
	finding := session.ReviewFinding{Fingerprint: "pattern-focused", Kind: "workflow", ChangeKind: "create", Summary: "Use focused tests after edits.", Vendor: "codex", TargetType: "skill", TargetPath: ".codex/skills/focused-tests/SKILL.md", Confidence: 0.8, Verified: true, Evidence: []string{"verified focused test workflow"}}
	draft := eval.Draft{Fingerprint: finding.Fingerprint, Type: "skill", ChangeKind: "create", Title: "Add focused test skill", Rationale: finding.Summary, Path: path, Body: "# Focused tests\n", Evidence: finding.Evidence}
	for n, id := range []string{"s1", "s2", "s3"} {
		_ = session.NewStore(paths).Start(session.Meta{ID: id, Vendor: "codex"})
		recs, err := Generate(paths, id, eval.Result{SessionID: id, EvidenceStatus: "sufficient", Findings: []session.ReviewFinding{finding}, Drafts: []eval.Draft{draft}, Dimensions: map[string]float64{"harness_use": 0.5, "scope": 0.7, "wandering": 0.2}}, nil)
		if err != nil || len(recs) != 1 {
			t.Fatalf("session %s recs=%+v err=%v", id, recs, err)
		}
		if recs[0].OccurrenceCount != n+1 || recs[0].AutoApplyAfter != 3 {
			t.Fatalf("session %s progress=%d/%d", id, recs[0].OccurrenceCount, recs[0].AutoApplyAfter)
		}
	}
	pending, _ := LoadPending(paths)
	if len(pending) != 1 || len(pending[0].RelatedSessions) != 3 {
		t.Fatalf("expected one accumulated recommendation: %+v", pending)
	}
}

func TestGenerateNestedAgentsWhenHotArea(t *testing.T) {
	root := t.TempDir()
	paths := harness.Resolve(root)
	_ = paths.EnsureDirs()
	_ = session.NewStore(paths).Start(session.Meta{ID: "s1", Vendor: "codex"})

	recs, err := Generate(paths, "s1", eval.Result{
		SessionID:      "s1",
		EvidenceStatus: "sufficient",
		HotAreas:       []string{"internal/recommend"},
		Dimensions:     map[string]float64{"wandering": 0.8, "harness_use": 0.5, "scope": 0.7},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var docs *Recommendation
	for i := range recs {
		if recs[i].Type == "docs" {
			docs = &recs[i]
			break
		}
	}
	if docs == nil {
		t.Fatal("expected docs recommendation")
	}
	wantPath := filepath.Join(root, "internal", "recommend", "AGENTS.md")
	if docs.ProposedPath != wantPath {
		t.Fatalf("proposed path = %q, want %q", docs.ProposedPath, wantPath)
	}
	if !strings.Contains(docs.Why, "Why:") || !strings.Contains(docs.Why, "How it helps:") {
		t.Fatalf("why must spell out problem and benefit: %q", docs.Why)
	}
	if !strings.Contains(docs.ProposedBody, "internal/recommend") {
		t.Fatalf("body should mention hot area: %q", docs.ProposedBody)
	}

	if err := Apply(paths, docs.ID, Decision{Reason: "Nested AGENTS.md will cut rediscovery in this package.", Actor: "agent"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# Agent instructions") {
		t.Fatalf("nested AGENTS.md should be a full document: %s", data)
	}
}

func TestGenerateRootAgentsWithoutHotArea(t *testing.T) {
	paths := harness.Resolve(t.TempDir())
	_ = paths.EnsureDirs()
	recs, err := Generate(paths, "s2", eval.Result{
		SessionID:      "s2",
		EvidenceStatus: "sufficient",
		Dimensions:     map[string]float64{"wandering": 0.9, "harness_use": 0.5, "scope": 0.7},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range recs {
		if r.Type == "docs" && r.ProposedPath == paths.AgentsMD {
			found = true
			if !strings.Contains(r.Why, "How it helps:") {
				t.Fatalf("root docs why incomplete: %q", r.Why)
			}
		}
	}
	if !found {
		t.Fatalf("expected root AGENTS.md docs rec, got %+v", recs)
	}
}
