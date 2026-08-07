package eval

import (
	"testing"

	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/harness"
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
		"internal/recommend/recommend.go": true,
		"internal/recommend/recommend_test.go": true,
		"internal/recommend/merge.go":     true,
		"internal/eval/eval.go":           true,
		"cmd/so/main.go":                  true,
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
