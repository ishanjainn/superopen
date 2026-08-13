package seed_test

import (
	"os"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/discover"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/llm"
	"github.com/ishanjainn/superopen/internal/seed"
)

func TestExtractJSONAndApplyUpgrade(t *testing.T) {
	raw := "```json\n{\"architecture_md\":\"# Arch\\nGo monorepo\",\"conventions_md\":\"# Conv\\n- Always run tests\",\"guardrails\":{\"rules\":[{\"id\":\"no-secrets\",\"description\":\"Never commit secrets\",\"severity\":\"block\"},{\"id\":\"no-race\",\"description\":\"Do not share mutable state across goroutines\",\"severity\":\"block\"}]},\"evals\":{\"checks\":[\"tests\",\"go_build\",\"race_patterns\"],\"agent_rules\":[\"Always run tests with -race\"],\"judge_rubric\":\"Prefer verification and harness use\"},\"brief\":\"Prefer .so/ first\"}\n```"
	js := llm.ExtractJSON(raw)
	if !strings.Contains(js, `"architecture_md"`) {
		t.Fatalf("extract failed: %s", js)
	}

	dir := t.TempDir()
	paths := harness.Resolve(dir)
	_ = paths.EnsureDirs()
	p := discover.Profile{Stack: "Go", DerivedRules: []string{"Always run tests"}}

	res, err := seed.UpgradeWithLLM(paths, p, nil, false)
	if err != nil || res.Used {
		t.Fatalf("expected skip without key: %+v %v", res, err)
	}
	if _, err := seed.UpgradeWithLLM(paths, p, nil, true); err == nil {
		t.Fatal("expected error when require=true and no key")
	}

	if err := seed.ApplyUpgradeJSON(paths, p, raw); err != nil {
		t.Fatal(err)
	}
	agents, _ := os.ReadFile(paths.AgentsMD)
	if !strings.Contains(string(agents), "Go monorepo") {
		t.Fatalf("AGENTS.md not written: %s", agents)
	}
	g, _ := os.ReadFile(paths.GuardrailsFile)
	if !strings.Contains(string(g), "no-secrets") {
		t.Fatalf("guardrails missing: %s", g)
	}
	ev, _ := os.ReadFile(paths.EvalsConfig)
	if !strings.Contains(string(ev), "race_patterns") {
		t.Fatalf("evals missing: %s", ev)
	}
}
