package seed_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/discover"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/llm"
	"github.com/ishanjainn/superopen/internal/recommend"
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

func TestApplyUpgradeJSONMCPAndSkills(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"next":"14.0.0","react":"18.0.0","@sentry/nextjs":"7.0.0"}}`), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, "tests"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, ".env.example"), []byte("X=\n"), 0o644)

	paths := harness.Resolve(dir)
	_ = paths.EnsureDirs()
	cfg := config.Default()
	cfg.Vendors.Enabled = []string{"claude", "cursor"}
	if err := config.Save(paths.Config, cfg); err != nil {
		t.Fatal(err)
	}

	p := discover.BuildProfile(dir, paths, "Node/TypeScript", "- tests")
	if len(p.Candidates) == 0 {
		t.Fatal("expected candidates")
	}
	brief := seed.BuildUpgradePrompt(p)
	if !strings.Contains(brief, "Automation candidates") {
		t.Fatalf("brief missing candidates:\n%s", brief)
	}

	raw := `{
  "architecture_md": "# Arch\nNext app",
  "conventions_md": "# Conv\n- Always run tests",
  "guardrails": {"rules": [
    {"id": "no-secrets", "description": "Never commit secrets", "severity": "block"},
    {"id": "run-tests", "description": "Run tests", "severity": "warn"},
    {"id": "avoid-unrelated", "description": "Keep diffs focused", "severity": "warn"}
  ]},
  "evals": {"checks": ["tests", "lint"]},
  "brief": "Prefer so graph query",
  "mcp": [
    {"name": "context7", "command": "npx", "args": ["-y", "@upstash/context7-mcp@1.0.0"]},
    {"name": "playwright", "command": "npx", "args": ["-y", "@playwright/mcp@0.0.10"]}
  ],
  "skills": [
    {"name": "web-vitest", "body": "# Web tests\n\nThis repo uses tests/. Run npm test after UI changes.\n"}
  ]
}`
	if err := seed.ApplyUpgradeJSON(paths, p, raw); err != nil {
		t.Fatal(err)
	}

	loaded, err := config.Load(paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.MCP.Servers) < 2 {
		t.Fatalf("expected mcp servers in config, got %#v", loaded.MCP.Servers)
	}
	mcpRoot, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mcpRoot), "context7") {
		t.Fatalf(".mcp.json missing context7: %s", mcpRoot)
	}
	cursorMCP := filepath.Join(dir, ".cursor", "mcp.json")
	if _, err := os.Stat(cursorMCP); err != nil {
		t.Fatal(err)
	}
	for _, vendor := range []string{".claude", ".cursor"} {
		skill := filepath.Join(dir, vendor, "skills", "web-vitest", "SKILL.md")
		if _, err := os.Stat(skill); err != nil {
			t.Fatalf("expected repo-learned skill at %s: %v", skill, err)
		}
	}

	// Second apply merges extra MCP without dropping.
	raw2 := `{
  "architecture_md": "# Arch\nNext app",
  "conventions_md": "# Conv\n- Always run tests",
  "guardrails": {"rules": [
    {"id": "no-secrets", "description": "Never commit secrets", "severity": "block"},
    {"id": "run-tests", "description": "Run tests", "severity": "warn"},
    {"id": "avoid-unrelated", "description": "Keep diffs focused", "severity": "warn"}
  ]},
  "evals": {"checks": ["tests"]},
  "mcp": [{"name": "sentry", "command": "npx", "args": ["-y", "@sentry/mcp-server@0.1.0"]}],
  "skills": [{"name": "web-vitest", "body": "# Web tests\n\nUpdated from this repo.\n"}]
}`
	if err := seed.ApplyUpgradeJSON(paths, p, raw2); err != nil {
		t.Fatal(err)
	}
	loaded, _ = config.Load(paths.Config)
	names := map[string]bool{}
	for _, s := range loaded.MCP.Servers {
		names[s.Name] = true
	}
	if !names["context7"] || !names["playwright"] || !names["sentry"] {
		t.Fatalf("expected merge of mcp servers, got %#v", loaded.MCP.Servers)
	}
}

func TestApplyUpgradeJSONSkipsEmptySkillBody(t *testing.T) {
	dir := t.TempDir()
	paths := harness.Resolve(dir)
	_ = paths.EnsureDirs()
	cfg := config.Default()
	cfg.Vendors.Enabled = []string{"cursor"}
	if err := config.Save(paths.Config, cfg); err != nil {
		t.Fatal(err)
	}
	raw := `{
  "architecture_md": "# A",
  "conventions_md": "# C",
  "guardrails": {"rules": [
    {"id": "no-secrets", "description": "Never commit secrets", "severity": "block"},
    {"id": "run-tests", "description": "Run tests", "severity": "warn"},
    {"id": "avoid-unrelated", "description": "Keep diffs focused", "severity": "warn"}
  ]},
  "evals": {"checks": ["tests"]},
  "skills": [{"name": "gen-test"}]
}`
	if err := seed.ApplyUpgradeJSON(paths, discover.Profile{Stack: "Go"}, raw); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".cursor", "skills", "gen-test", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("empty skill body must not write a catalog template")
	}
}

func TestEnqueueLeftoverDoesNotQueueSkills(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"react":"18.0.0","stripe":"14.0.0"}}`), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, "tests"), 0o755)
	paths := harness.Resolve(dir)
	_ = paths.EnsureDirs()
	cfg := config.Default()
	cfg.Vendors.Enabled = []string{"codex"}
	_ = config.Save(paths.Config, cfg)

	p := discover.BuildProfile(dir, paths, "Node", "- tests")
	raw := `{
  "architecture_md": "# A",
  "conventions_md": "# C",
  "guardrails": {"rules": [
    {"id": "no-secrets", "description": "Never commit secrets", "severity": "block"},
    {"id": "run-tests", "description": "Run tests", "severity": "warn"},
    {"id": "avoid-unrelated", "description": "Keep diffs focused", "severity": "warn"}
  ]},
  "evals": {"checks": ["tests"]},
  "mcp": [{"name": "context7", "command": "npx", "args": ["-y", "@upstash/context7-mcp@1.0.0"]}],
  "skills": []
}`
	if err := seed.ApplyUpgradeJSON(paths, p, raw); err != nil {
		t.Fatal(err)
	}
	pending, err := recommend.LoadPending(paths)
	if err != nil {
		t.Fatal(err)
	}
	var sawSkill bool
	for _, r := range pending {
		if r.Type == "mcp" {
			t.Fatal("must not enqueue mcp recommendations")
		}
		if r.Type == "skill" {
			sawSkill = true
		}
	}
	if sawSkill {
		t.Fatalf("must not enqueue catalog skill templates, got %#v", pending)
	}
}
