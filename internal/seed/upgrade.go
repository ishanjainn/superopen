package seed

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/discover"
	"github.com/ishanjainn/superopen/internal/guardrails"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/llm"
	"github.com/ishanjainn/superopen/internal/mcp"
	"github.com/ishanjainn/superopen/internal/nativedocs"
	"github.com/ishanjainn/superopen/internal/recommend"
	"gopkg.in/yaml.v3"
)

// UpgradeResult describes what the LLM rewrite produced.
type UpgradeResult struct {
	Used   bool
	Reason string // skipped reason or "ok"
	Rules  int
	Checks int
	MCP    int
	Skills int
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
	Brief  string         `json:"brief,omitempty"`
	MCP    []llmMCPServer `json:"mcp,omitempty"`
	Skills []llmSkillPick `json:"skills,omitempty"`
}

type llmMCPServer struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

type llmSkillPick struct {
	Name string `json:"name"`
	Body string `json:"body,omitempty"`
}

const UpgradeSystemPrompt = `You are a Superopen engineer. Given a compact repository profile (graph summary + stack signals + automation candidates + existing agent instruction excerpts), produce a high-quality Superopen seed.

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
  "brief": "short shared AGENTS.md learned-section guidance pointing agents at vendor-native rules/skills and so graph query",
  "mcp": [
    {"name": "context7", "command": "npx", "args": ["-y", "@upstash/context7-mcp@1.0.0"]}
  ],
  "skills": [
    {"name": "gen-test", "body": "# gen-test\n\n..."}
  ]
}

Rules for quality:
- Prefer 6-12 guardrails; dedupe.
- severity **block** ONLY for secrets/credentials (max 2). All other rules must be **warn** (style, tests, concurrency, SQL, PR titles, rate limits).
- Invent NO secrets or fake paths. Stay faithful to the profile.
- Include baseline: no-secrets (block), run-tests (warn), avoid-unrelated (warn).
- checks should include stack-appropriate ones (e.g. go_build, race_patterns, sql_parameterized, pr_title_convention).
- architecture_md and conventions_md should be useful to a coding agent in <4000 chars each.
- From "## Automation candidates", pick the top 1-2 MCP servers and top 1-2 skills that fit this repo. Omit empty arrays.
- MCP entries must use pinned package versions (never @latest). No env blocks or secrets.
- Never recommend Memory MCP — Superopen memory already covers cross-session recall.
- Skill bodies should be short SKILL.md markdown. Prefer candidate names from the catalog.
- Include catalog guardrail candidates (block-env-edits, block-lockfile-edits) when their evidence matches.
`

// BuildUpgradePrompt returns the user-turn profile blob for harness upgrade.
func BuildUpgradePrompt(p discover.Profile) string {
	return buildUpgradePrompt(p)
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
		MCP:    len(parsed.MCP),
		Skills: len(parsed.Skills),
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
	if len(p.Signals.Manifests) > 0 || len(p.Signals.Deps) > 0 || len(p.Signals.Configs) > 0 {
		b.WriteString("\n## Stack signals\n")
		if len(p.Signals.Manifests) > 0 {
			b.WriteString("- manifests: " + strings.Join(p.Signals.Manifests, ", ") + "\n")
		}
		if len(p.Signals.Deps) > 0 {
			deps := p.Signals.Deps
			if len(deps) > 24 {
				deps = deps[:24]
			}
			b.WriteString("- deps: " + strings.Join(deps, ", ") + "\n")
		}
		if len(p.Signals.Configs) > 0 {
			b.WriteString("- configs: " + strings.Join(p.Signals.Configs, ", ") + "\n")
		}
		if len(p.Signals.Dirs) > 0 {
			b.WriteString("- dirs: " + strings.Join(p.Signals.Dirs, ", ") + "\n")
		}
		if p.Signals.GitRemote != "" {
			b.WriteString("- git_remote: " + p.Signals.GitRemote + "\n")
		}
		if p.Signals.HasEnvFiles {
			b.WriteString("- has_env_or_credentials_files: true\n")
		}
		if p.Signals.HasLockfiles {
			b.WriteString("- has_lockfiles: true\n")
		}
	}
	if len(p.Candidates) > 0 {
		b.WriteString("\n## Automation candidates (pick top 1-2 MCP and 1-2 skills)\n")
		b.WriteString("Do not invent MCP packages outside this list unless evidence is overwhelming. Never pick Memory MCP.\n")
		for _, c := range p.Candidates {
			b.WriteString(fmt.Sprintf("- [%s] id=%s name=%s score=%d — %s\n", c.Kind, c.ID, c.Name, c.Score, c.Title))
			if c.Rationale != "" {
				b.WriteString("  rationale: " + c.Rationale + "\n")
			}
			if len(c.Evidence) > 0 {
				b.WriteString("  evidence: " + strings.Join(c.Evidence, "; ") + "\n")
			}
			if c.Kind == discover.CandidateMCP && c.Command != "" {
				b.WriteString(fmt.Sprintf("  install: command=%s args=%v\n", c.Command, c.Args))
			}
		}
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
	if brief := strings.TrimSpace(out.Brief); brief != "" {
		_ = nativedocs.AppendLearned(paths, brief)
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
		DeniedTools:    def.DeniedTools,
		DeniedCommands: def.DeniedCommands,
		SensitivePaths: def.SensitivePaths,
	}
	if prev, err := os.ReadFile(paths.GuardrailsFile); err == nil {
		var existing guardrails.File
		if yaml.Unmarshal(prev, &existing) == nil {
			if existing.DeniedTools != nil {
				gf.DeniedTools = existing.DeniedTools
			}
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
	gHeader := "# Superopen guardrails. These are shared project safety rules enforced at coding-agent hook boundaries.\n# Authoritative project policy updated by project maintainers and approved Superopen upgrades.\n"
	if err := os.WriteFile(paths.GuardrailsFile, append([]byte(gHeader), data...), 0o644); err != nil {
		return err
	}

	checks := out.Evals.Checks
	if len(checks) == 0 {
		checks = []string{"tests", "lint", "scope", "harness_usage"}
	}
	var eb strings.Builder
	eb.WriteString("# Superopen evaluation policy. This defines how completed coding sessions are scored and which reviewer backends may be used.\n")
	eb.WriteString("# Authoritative project policy updated by project maintainers and approved Superopen upgrades.\n")
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

	pickedMCP, pickedSkills, err := applyAutomationPicks(paths, out)
	if err != nil {
		return err
	}
	_ = EnqueueLeftoverCandidates(paths, p, pickedMCP, pickedSkills)
	return nil
}

func applyAutomationPicks(paths harness.Paths, out llmHarnessOut) (map[string]bool, map[string]bool, error) {
	pickedMCP := map[string]bool{}
	pickedSkills := map[string]bool{}

	cfg, err := config.Load(paths.Config)
	if err != nil {
		cfg = config.Default()
	}

	var incoming []config.MCPServer
	for _, s := range out.MCP {
		name := strings.TrimSpace(s.Name)
		cmd := strings.TrimSpace(s.Command)
		if name == "" || cmd == "" {
			continue
		}
		if strings.EqualFold(name, "memory") || strings.EqualFold(name, "memory-mcp") {
			continue
		}
		skip := false
		for _, a := range s.Args {
			if strings.Contains(a, "@latest") {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		args := filterPinnedArgs(s.Args)
		incoming = append(incoming, config.MCPServer{Name: name, Command: cmd, Args: args})
		pickedMCP[strings.ToLower(name)] = true
		if len(incoming) >= 2 {
			break
		}
	}
	if len(incoming) > 0 {
		cfg.MCP.Servers = mcp.MergeServers(cfg.MCP.Servers, incoming)
		if err := config.Save(paths.Config, cfg); err != nil {
			return pickedMCP, pickedSkills, err
		}
		if err := mcp.Project(paths.RepoRoot, cfg); err != nil {
			return pickedMCP, pickedSkills, err
		}
	}

	vendors := enabledVendors(cfg)
	skillCount := 0
	for _, sk := range out.Skills {
		name := strings.TrimSpace(sk.Name)
		if name == "" || name == "so" || name == "superopen" {
			continue
		}
		body := strings.TrimSpace(sk.Body)
		if body == "" {
			body = discover.DefaultSkillBody(name)
		}
		for _, vendor := range vendors {
			_ = nativedocs.UpsertSkill(paths, name, body, nativedocs.WriteOpts{Vendor: vendor})
		}
		pickedSkills[strings.ToLower(name)] = true
		skillCount++
		if skillCount >= 2 {
			break
		}
	}
	return pickedMCP, pickedSkills, nil
}

func filterPinnedArgs(args []string) []string {
	var out []string
	for _, a := range args {
		a = strings.TrimSpace(a)
		if a == "" || strings.Contains(a, "@latest") {
			continue
		}
		out = append(out, a)
	}
	return out
}

func enabledVendors(cfg config.Config) []string {
	seen := map[string]bool{}
	var out []string
	add := func(v string) {
		k := harness.NormalizeVendorKind(v)
		if k == "" || seen[k] {
			return
		}
		seen[k] = true
		out = append(out, k)
	}
	for _, v := range cfg.Vendors.Enabled {
		add(v)
	}
	if len(out) == 0 {
		for _, v := range []string{"claude", "cursor", "codex", "gemini", "opencode", "copilot", "pi"} {
			add(v)
		}
	}
	return out
}

// EnqueueLeftoverCandidates queues high-signal skill/guardrail candidates that
// the upgrade JSON omitted. MCP leftovers stay for the next /so init refresh.
func EnqueueLeftoverCandidates(paths harness.Paths, p discover.Profile, pickedMCP, pickedSkills map[string]bool) error {
	_ = pickedMCP // MCP is not a recommendation type; next upgrade-brief surfaces them.
	now := time.Now().UTC()
	var draft []recommend.Recommendation
	skillCount := 0
	guardCount := 0
	for _, c := range p.Candidates {
		switch c.Kind {
		case discover.CandidateSkill:
			if pickedSkills[strings.ToLower(c.Name)] || skillCount >= 2 {
				continue
			}
			vendors := enabledVendors(mustLoadConfig(paths))
			if len(vendors) == 0 {
				continue
			}
			vendor := vendors[0]
			body := c.Body
			if body == "" {
				body = discover.DefaultSkillBody(c.Name)
			}
			path := filepath.Join(harness.SkillsDirForVendor(paths.RepoRoot, vendor), c.Name, "SKILL.md")
			draft = append(draft, recommend.Recommendation{
				ID:           fmt.Sprintf("rec_setup_%d_%s", now.UnixNano(), c.Name),
				Fingerprint:  recommend.FingerprintKey("skill", path, c.Name),
				SessionID:    "_system",
				Type:         "skill",
				Title:        c.Title,
				Rationale:    c.Rationale,
				Why:          c.Rationale,
				Evidence:     c.Evidence,
				ProposedPath: path,
				ProposedBody: body,
				Status:       "pending",
				CreatedAt:    now,
				Vendor:       vendor,
				ChangeKind:   "create",
			})
			skillCount++
		case discover.CandidateGuardrail:
			if guardCount >= 2 {
				continue
			}
			id := c.GuardrailID
			if id == "" {
				id = c.Name
			}
			// Skip if already present in written guardrails.
			if data, err := os.ReadFile(paths.GuardrailsFile); err == nil && strings.Contains(string(data), "id: "+id) {
				continue
			}
			sev := c.Severity
			if sev == "" {
				sev = "warn"
			}
			body := fmt.Sprintf("rules:\n  - id: %s\n    description: %s\n    severity: %s\n    source: recommend\n", id, yamlQuote(c.Rationale), sev)
			draft = append(draft, recommend.Recommendation{
				ID:           fmt.Sprintf("rec_setup_%d_%s", now.UnixNano(), id),
				Fingerprint:  recommend.FingerprintKey("guardrail", paths.GuardrailsFile, id),
				SessionID:    "_system",
				Type:         "guardrail",
				Title:        c.Title,
				Rationale:    c.Rationale,
				Why:          c.Rationale,
				Evidence:     c.Evidence,
				ProposedPath: paths.GuardrailsFile,
				ProposedBody: body,
				Status:       "pending",
				CreatedAt:    now,
				ChangeKind:   "update",
			})
			guardCount++
		}
	}
	if len(draft) == 0 {
		return nil
	}
	_, err := recommend.MergePending(paths, draft)
	return err
}

func mustLoadConfig(paths harness.Paths) config.Config {
	cfg, err := config.Load(paths.Config)
	if err != nil {
		return config.Default()
	}
	return cfg
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
