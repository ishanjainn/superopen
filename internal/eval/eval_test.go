package eval

import (
	"strings"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/session"
	"github.com/ishanjainn/superopen/internal/tracestore"
)

type staticCompleter struct{ output string }

func (s staticCompleter) Available() bool                      { return true }
func (s staticCompleter) Complete(_, _ string) (string, error) { return s.output, nil }
func (s staticCompleter) Backend() string                      { return "test-vendor" }

func TestRunRecognizesCodexToolCallsAndHarnessUse(t *testing.T) {
	paths := harness.Resolve(t.TempDir())
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	spans := []tracestore.Span{
		{
			Name:      "coding_agent.tool.call",
			SessionID: "codex-session",
			Attributes: map[string]string{
				"gen_ai.tool.name":           "Bash",
				"gen_ai.tool.call.arguments": `{"cmd":"so graph query \"how does auth work?\""}`,
			},
		},
		{
			Name:      "coding_agent.tool.call",
			SessionID: "codex-session",
			Attributes: map[string]string{
				"gen_ai.tool.name":           "apply_patch",
				"gen_ai.tool.call.arguments": `{"patch":"*** Begin Patch"}`,
			},
		},
	}

	res, err := Run(paths, config.Config{Evals: config.EvalsConfig{Backend: "heuristics"}}, "codex-session", spans, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.EvidenceStatus != "sufficient" {
		t.Fatalf("evidence status = %q, want sufficient", res.EvidenceStatus)
	}
	if got := res.Dimensions["harness_use"]; got != 0.9 {
		t.Fatalf("harness_use = %.2f, want 0.9", got)
	}
}

func TestRunMarksMissingActivityEvidenceUnknown(t *testing.T) {
	paths := harness.Resolve(t.TempDir())
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	res, err := Run(paths, config.Config{Evals: config.EvalsConfig{Backend: "heuristics"}}, "empty-session", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Badge != "unknown" || res.EvidenceStatus != "insufficient" {
		t.Fatalf("got badge=%q evidence=%q", res.Badge, res.EvidenceStatus)
	}
	if len(res.Dimensions) != 0 {
		t.Fatalf("missing evidence must not create scored dimensions: %+v", res.Dimensions)
	}
}

func TestHotAreasFromFiles(t *testing.T) {
	files := map[string]bool{
		"internal/recommend/recommend.go":      true,
		"internal/recommend/recommend_test.go": true,
		"internal/recommend/merge.go":          true,
		"internal/eval/eval.go":                true,
		"cmd/so/main.go":                       true,
	}
	got := hotAreasFromFiles(files)
	if len(got) == 0 || got[0] != "internal/recommend" {
		t.Fatalf("hot areas = %v, want internal/recommend first", got)
	}
}

func TestGuidanceArea(t *testing.T) {
	cases := map[string]string{
		"internal/foo/bar.go": "internal/foo",
		"cmd/so/main.go":      "cmd/so",
		"web/src/app.tsx":     "web/src",
		"README.md":           "",
		".so/config.json":     "",
	}
	for in, want := range cases {
		if got := guidanceArea(in); got != want {
			t.Fatalf("guidanceArea(%q)=%q want %q", in, got, want)
		}
	}
}

func TestRunPersistsSessionReviewEvidence(t *testing.T) {
	root := t.TempDir()
	paths := harness.Resolve(root)
	_ = paths.EnsureDirs()
	ss := session.NewStore(paths)
	_ = ss.Start(session.Meta{ID: "reviewed", Vendor: "codex"})
	spans := []tracestore.Span{
		{Name: "coding_agent.llm.turn", SpanID: "prompt-1", SessionID: "reviewed", Attributes: map[string]string{"gen_ai.prompt": "Always run focused tests after changing this package."}},
		{Name: "coding_agent.tool.call", SpanID: "edit-1", SessionID: "reviewed", Attributes: map[string]string{"gen_ai.tool.name": "apply_patch"}},
		{Name: "coding_agent.tool.call", SpanID: "test-1", SessionID: "reviewed", Attributes: map[string]string{"gen_ai.tool.name": "Bash", "gen_ai.tool.call.arguments": `{"cmd":"go test ./internal/eval"}`}},
	}
	res, err := Run(paths, config.Config{Evals: config.EvalsConfig{Backend: "heuristics"}}, "reviewed", spans, nil, RunOptions{Final: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) == 0 || !res.Findings[0].ExplicitWorkflow {
		t.Fatalf("durable correction finding missing: %+v", res.Findings)
	}
	doc, err := ss.ReadDocument("reviewed")
	if err != nil || len(doc.Review.Findings) == 0 {
		t.Fatalf("session evidence not persisted: %+v err=%v", doc.Review, err)
	}
	if len(doc.Evaluation) == 0 || strings.Contains(string(doc.Evaluation), "Always run focused") {
		t.Fatalf("evaluation should exist without duplicating prompt text: %s", doc.Evaluation)
	}
}

func TestReadOnlyEvaluationDoesNotBecomePoor(t *testing.T) {
	paths := harness.Resolve(t.TempDir())
	_ = paths.EnsureDirs()
	_ = session.NewStore(paths).Start(session.Meta{ID: "read-only", Vendor: "codex", StartedAt: time.Now().UTC()})
	spans := []tracestore.Span{{
		Name: "coding_agent.tool.call", SessionID: "read-only",
		Attributes: map[string]string{
			"gen_ai.tool.name": "Bash", "gen_ai.tool.call.arguments": `{"cmd":"sed -n '1,80p' AGENTS.md"}`,
		},
	}}
	model := staticCompleter{output: `{"exploration":0,"scope":0,"wandering":0,"verification":0,"findings":[],"memory":{}}`}
	res, err := Run(paths, config.Config{Evals: config.EvalsConfig{Backend: "auto"}}, "read-only", spans, model, RunOptions{Final: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Badge != "unknown" {
		t.Fatalf("read-only badge=%q, want unknown", res.Badge)
	}
	if _, ok := res.Dimensions["scope"]; ok {
		t.Fatalf("scope must be inapplicable for read-only evidence: %+v", res.Dimensions)
	}
	if _, ok := res.Dimensions["verification"]; ok {
		t.Fatalf("verification must be inapplicable for read-only evidence: %+v", res.Dimensions)
	}
}

func TestActiveSnapshotDoesNotPersistFindingsOrCompleteReview(t *testing.T) {
	paths := harness.Resolve(t.TempDir())
	_ = paths.EnsureDirs()
	ss := session.NewStore(paths)
	_ = ss.Start(session.Meta{ID: "active", Vendor: "codex", StartedAt: time.Now().UTC()})
	_ = ss.WriteDocument("active", func(doc *session.Document) { doc.Review.Status = "pending" })
	spans := []tracestore.Span{{Name: "coding_agent.tool.call", SessionID: "active", Attributes: map[string]string{"gen_ai.tool.name": "apply_patch"}}}
	model := staticCompleter{output: `{"exploration":0.8,"scope":0.8,"wandering":0.1,"verification":0.8,"findings":[{"kind":"workflow","change_kind":"create","target_type":"skill","target_path":".codex/skills/x/SKILL.md","summary":"Create x","confidence":0.9,"proposed_body":"# X"}],"memory":{"lessons":["x"]}}`}
	res, err := Run(paths, config.Config{Evals: config.EvalsConfig{Backend: "auto"}}, "active", spans, model)
	if err != nil {
		t.Fatal(err)
	}
	if res.EvaluationScope != "snapshot" || len(res.Findings) != 0 || len(res.Drafts) != 0 || len(res.Memory.Lessons) != 0 {
		t.Fatalf("active evaluation mutated durable review data: %+v", res)
	}
	doc, err := ss.ReadDocument("active")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Review.Status != "pending" || len(doc.Review.Findings) != 0 {
		t.Fatalf("snapshot completed or populated review: %+v", doc.Review)
	}
}

func TestRepeatedGenericShellCallsAreNotAWorkflow(t *testing.T) {
	var steps []string
	for range 10 {
		if step := workflowStep("Bash", `{"cmd":"custom-command"}`); step != "" {
			steps = append(steps, step)
		}
	}
	if got := repeatedWorkflow(steps); got != "" {
		t.Fatalf("generic shell calls produced workflow %q", got)
	}
}
