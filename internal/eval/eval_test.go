package eval

import (
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/session"
	"github.com/ishanjainn/superopen/internal/tracestore"
)

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
	res, err := Run(paths, config.Config{Evals: config.EvalsConfig{Backend: "heuristics"}}, "reviewed", spans, nil)
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
