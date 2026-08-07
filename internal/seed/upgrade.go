package seed

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/discover"
	"github.com/ishanjainn/superopen/internal/guardrails"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/llm"
	"github.com/ishanjainn/superopen/internal/nativedocs"
	"gopkg.in/yaml.v3"
)

// UpgradeResult describes what the LLM rewrite produced.
type UpgradeResult struct {
	Used   bool
	Reason string // skipped reason or "ok"
	Rules  int
	Checks int
}

type llmHarnessOut struct {
	ArchitectureMD string `json:"architecture_md"`
	ConventionsMD  string `json:"conventions_md"`
	Guardrails     struct {
		Rules []guardrails.Rule `json:"rules"`
	} `json:"guardrails"`
	Evals struct {
		Checks      []string `json:"checks"`
		AgentRules  []string `json:"agent_rules"`
		JudgeRubric string   `json:"judge_rubric"`
	} `json:"evals"`
	Brief string `json:"brief,omitempty"`
}

const UpgradeSystemPrompt = `You are a Superopen engineer. Given a compact repository profile (graph summary + existing agent instruction excerpts), produce a high-quality Superopen seed.

Return ONLY valid JSON (no markdown fences) with this shape:
{
  "architecture_md": "markdown doc: what the repo is, top packages/dirs, how services relate, where agents should look first",
  "conventions_md": "markdown doc: coding/PR/test conventions distilled from agent docs - imperative bullets, no DO/DON'T noise",
  "guardrails": {
    "rules": [
      {"id": "kebab-case-id", "description": "clear imperative rule", "severity": "block|warn", "source": "llm"}
    ]
  },
  "evals": {
    "checks": ["tests", "lint", "..."],
    "agent_rules": ["top rules for an LLM judge"],
    "judge_rubric": "short paragraph for session scoring"
  },
  "brief": "short AGENT.md brief pointing agents at AGENTS.md, the repo's discovered rules/skills dirs (Cursor/Claude/Codex/Gemini/…), and so graph query"
}

Rules for quality:
- Prefer 6-12 guardrails; dedupe.
- severity **block** ONLY for secrets/credentials (max 2). All other rules must be **warn** (style, tests, concurrency, SQL, PR titles, rate limits).
- Invent NO secrets or fake paths. Stay faithful to the profile.
- Include baseline: no-secrets (block), run-tests (warn), avoid-unrelated (warn).
- checks should include stack-appropriate ones (e.g. go_build, race_patterns, sql_parameterized, pr_title_convention).
- architecture_md and conventions_md should be useful to a coding agent in <4000 chars each.
`

// BuildUpgradePrompt returns the user-turn profile blob for harness upgrade.
func BuildUpgradePrompt(p discover.Profile) string {
	return buildUpgradePrompt(p)
}

// WriteUpgradeBrief writes .so/upgrade-brief.md for assistant-driven upgrade
// (Graphify-style: the coding agent is the LLM; no API key required).
func WriteUpgradeBrief(paths harness.Paths, p discover.Profile) error {
	var b strings.Builder
	b.WriteString("# Superopen upgrade brief\n\n")
	b.WriteString("Coding agents: use **your own model** to upgrade AGENTS.md / guardrails / evals.\n")
	b.WriteString("Do **not** ask the user for an API key. Produce the JSON below, then run:\n\n")
	b.WriteString("```bash\nso apply-upgrade <<'EOF'\n{ ... }\nEOF\n```\n\n")
	b.WriteString("Or write a file and run `so apply-upgrade path/to.json`.\n")
	b.WriteString("**Never** pass a file path and a heredoc together — the CLI rejects that so a stale file cannot override fresh JSON.\n\n")
	b.WriteString("## System instructions\n\n")
	b.WriteString(UpgradeSystemPrompt)
	b.WriteString("\n\n## Repository profile\n\n")
	b.WriteString(buildUpgradePrompt(p))
	return os.WriteFile(filepath.Join(paths.Root, "upgrade-brief.md"), []byte(b.String()), 0o644)
}

// UpgradeWithLLM rewrites docs/guardrails/evals using an LLM when available.
// Heuristic Seed must already have run (or files may be empty). Failures return Used=false + error only if require=true.
func UpgradeWithLLM(paths harness.Paths, p discover.Profile, client *llm.Client, require bool) (UpgradeResult, error) {
	if client == nil || !client.Available() {
		res := UpgradeResult{Used: false, Reason: "no LLM configured (see so init setup guide)"}
		if require {
			return res, fmt.Errorf("LLM required but %s", res.Reason)
		}
		return res, nil
	}

	user := buildUpgradePrompt(p)
	out, err := client.CompleteOpts(UpgradeSystemPrompt, user, llm.Options{
		MaxTokens: 6000,
		Timeout:   120 * time.Second,
	})
	if err != nil {
		res := UpgradeResult{Used: false, Reason: err.Error()}
		if require {
			return res, fmt.Errorf("llm upgrade: %w", err)
		}
		return res, nil
	}

	var parsed llmHarnessOut
	raw := llm.ExtractJSON(out)
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		res := UpgradeResult{Used: false, Reason: "invalid llm json: " + err.Error()}
		if require {
			return res, fmt.Errorf("%s", res.Reason)
		}
		return res, nil
	}

	if err := applyLLMUpgrade(paths, p, parsed); err != nil {
		res := UpgradeResult{Used: false, Reason: err.Error()}
		if require {
			return res, err
		}
		return res, nil
	}

	return UpgradeResult{
		Used:   true,
		Reason: "ok",
		Rules:  len(parsed.Guardrails.Rules),
		Checks: len(parsed.Evals.Checks),
	}, nil
}

func buildUpgradePrompt(p discover.Profile) string {
	var b strings.Builder
	b.WriteString("## Stack\n")
	b.WriteString(p.Stack + "\n\n")
	b.WriteString("## Top-level structure\n")
	b.WriteString(p.Structure + "\n\n")
	b.WriteString("## Graph summary\n")
	b.WriteString(fmt.Sprintf("- nodes=%d edges=%d\n", p.Graph.NodeCount, p.Graph.EdgeCount))
	if len(p.Graph.TopDirs) > 0 {
		b.WriteString("- top_dirs: " + strings.Join(p.Graph.TopDirs, ", ") + "\n")
	}
	if len(p.Graph.Languages) > 0 {
		b.WriteString("- languages: ")
		var parts []string
		for k, v := range p.Graph.Languages {
			parts = append(parts, fmt.Sprintf("%s=%d", k, v))
		}
		b.WriteString(strings.Join(parts, ", ") + "\n")
	}
	if len(p.Graph.SampleFiles) > 0 {
		b.WriteString("- sample_files: " + strings.Join(p.Graph.SampleFiles, ", ") + "\n")
	}
	b.WriteString("\n## Themes\n")
	for _, t := range p.Themes {
		b.WriteString("- " + t + "\n")
	}
	b.WriteString("\n## Derived rules (heuristic)\n")
	for _, r := range p.DerivedRules {
		b.WriteString("- " + r + "\n")
	}
	b.WriteString("\n## Agent sources (excerpts)\n")
	for _, a := range p.Agents {
		b.WriteString(fmt.Sprintf("### %s (%s)\n", filepath.Base(a.Path), a.Kind))
		if len(a.Headings) > 0 {
			b.WriteString("Headings: " + strings.Join(limit(a.Headings, 12), "; ") + "\n")
		}
		excerpt := a.Excerpt
		if len(excerpt) > 1800 {
			excerpt = excerpt[:1800] + "…"
		}
		b.WriteString(excerpt + "\n\n")
	}
	return b.String()
}

func applyLLMUpgrade(paths harness.Paths, p discover.Profile, out llmHarnessOut) error {
	arch := strings.TrimSpace(out.ArchitectureMD)
	conv := strings.TrimSpace(out.ConventionsMD)
	if arch != "" || conv != "" {
		body := nativedocs.DefaultAgentsBody(arch, conv, "")
		if err := nativedocs.EnsureAgentsMD(paths, body, true); err != nil {
			return err
		}
	}

	rules := out.Guardrails.Rules
	if len(rules) == 0 {
		return fmt.Errorf("llm returned zero guardrails")
	}
	for i := range rules {
		if rules[i].Source == "" {
			rules[i].Source = "llm"
		}
		if rules[i].Severity == "" {
			rules[i].Severity = "warn"
		}
		if rules[i].ID == "" {
			rules[i].ID = fmt.Sprintf("llm-%d", i+1)
		}
	}
	def := guardrails.DefaultPolicy()
	gf := guardrails.File{
		Rules:          dedupeRules(rules),
		Approval:       def.Approval,
		RedactOutput:   def.RedactOutput,
		DeniedCommands: def.DeniedCommands,
		SensitivePaths: def.SensitivePaths,
	}
	if prev, err := os.ReadFile(paths.GuardrailsFile); err == nil {
		var existing guardrails.File
		if yaml.Unmarshal(prev, &existing) == nil {
			if len(existing.DeniedCommands) > 0 {
				gf.DeniedCommands = existing.DeniedCommands
			}
			if len(existing.SensitivePaths) > 0 {
				gf.SensitivePaths = existing.SensitivePaths
			}
			if existing.Approval != "" {
				gf.Approval = existing.Approval
			}
			gf.RedactOutput = existing.RedactOutput
		}
	}
	data, err := yaml.Marshal(gf)
	if err != nil {
		return err
	}
	gHeader := "# Generated by so init (LLM upgrade) - advisory rules + enforcement.\n# Edit freely; so sync will not overwrite this file.\n"
	if err := os.WriteFile(paths.GuardrailsFile, append([]byte(gHeader), data...), 0o644); err != nil {
		return err
	}

	checks := out.Evals.Checks
	if len(checks) == 0 {
		checks = []string{"tests", "lint", "scope", "harness_usage"}
	}
	var eb strings.Builder
	eb.WriteString("# Generated by so init (LLM upgrade).\n")
	eb.WriteString("checks:\n")
	seen := map[string]bool{}
	for _, c := range checks {
		c = strings.TrimSpace(c)
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		eb.WriteString("  - " + c + "\n")
	}
	agentRules := out.Evals.AgentRules
	if len(agentRules) == 0 {
		agentRules = p.DerivedRules
	}
	if len(agentRules) > 0 {
		eb.WriteString("\nagent_rules:\n")
		for i, r := range agentRules {
			if i >= 20 {
				break
			}
			eb.WriteString("  - " + yamlQuote(r) + "\n")
		}
	}
	if strings.TrimSpace(out.Evals.JudgeRubric) != "" {
		eb.WriteString("\njudge_rubric: |\n")
		for _, line := range strings.Split(strings.TrimSpace(out.Evals.JudgeRubric), "\n") {
			eb.WriteString("  " + line + "\n")
		}
	}
	if len(p.Agents) > 0 {
		eb.WriteString("\nsources:\n")
		for _, a := range p.Agents {
			eb.WriteString("  - " + a.Path + "\n")
		}
	}
	if err := os.WriteFile(paths.EvalsConfig, []byte(eb.String()), 0o644); err != nil {
		return err
	}

	brief := out.Brief
	if strings.TrimSpace(brief) == "" {
		brief = `Prefer AGENTS.md, project rules/skills, and so graph query before broad search. Guardrails: .so/guardrails/guardrails.yaml.`
	}
	_ = os.WriteFile(paths.AgentBrief, []byte(strings.TrimSpace(brief)+"\n"), 0o644)

	meta := map[string]any{
		"upgraded_at": time.Now().UTC().Format(time.RFC3339),
		"source":      "llm",
		"rules":       len(gf.Rules),
		"checks":      len(seen),
	}
	if b, err := json.MarshalIndent(meta, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(paths.Root, "upgrade.json"), b, 0o644)
	}
	return nil
}

// ApplyUpgradeJSON applies a previously parsed LLM harness JSON payload (for tests / offline).
func ApplyUpgradeJSON(paths harness.Paths, p discover.Profile, rawJSON string) error {
	var parsed llmHarnessOut
	if err := json.Unmarshal([]byte(llm.ExtractJSON(rawJSON)), &parsed); err != nil {
		return err
	}
	return applyLLMUpgrade(paths, p, parsed)
}

func limit(in []string, n int) []string {
	if len(in) <= n {
		return in
	}
	return in[:n]
}
