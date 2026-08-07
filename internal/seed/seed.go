package seed

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/discover"
	"github.com/ishanjainn/superopen/internal/guardrails"
	"github.com/ishanjainn/superopen/internal/harness"
	"gopkg.in/yaml.v3"
)

// SeedOptions controls harness seeding after graph + agent discovery.
type SeedOptions struct {
	TemplateRoot string
	Profile      discover.Profile
	// Force regenerates discovery-driven files (guardrails, evals, discovery.json, agent brief).
	Force bool
}

// Seed writes context/skills/guardrails/evals/rules from templates + discovered repo profile.
func Seed(paths harness.Paths, opts SeedOptions) error {
	p := opts.Profile
	replacements := map[string]string{
		"{{STRUCTURE}}": p.Structure,
		"{{STACK}}":     p.Stack,
	}

	pairs := []struct{ src, dst string }{
		{"knowledge/architecture.md", filepath.Join(paths.KnowledgeDir, "architecture.md")},
		{"knowledge/conventions.md", filepath.Join(paths.KnowledgeDir, "conventions.md")},
		{"knowledge/decisions.md", filepath.Join(paths.KnowledgeDir, "decisions.md")},
		{"rules/coding.md", filepath.Join(paths.RulesDir, "coding.md")},
		{"skills/create-api.md", filepath.Join(paths.SkillsDir, "create-api.md")},
		{"skills/debugging.md", filepath.Join(paths.SkillsDir, "debugging.md")},
		{"skills/testing.md", filepath.Join(paths.SkillsDir, "testing.md")},
	}
	for _, pair := range pairs {
		if !opts.Force {
			if _, err := os.Stat(pair.dst); err == nil {
				continue
			}
		}
		data, err := os.ReadFile(filepath.Join(opts.TemplateRoot, pair.src))
		if err != nil {
			data = []byte("# " + filepath.Base(pair.dst) + "\n")
		}
		content := string(data)
		for k, v := range replacements {
			content = strings.ReplaceAll(content, k, v)
		}
		base := filepath.Base(pair.dst)
		if base == "architecture.md" {
			content = enrichArchitecture(content, p)
		}
		if base == "conventions.md" {
			content = enrichConventions(content, p)
		}
		if err := os.MkdirAll(filepath.Dir(pair.dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(pair.dst, []byte(content), 0o644); err != nil {
			return err
		}
	}

	if err := seedGuardrails(paths, p, opts.TemplateRoot, opts.Force); err != nil {
		return err
	}
	if err := seedEvals(paths, p, opts.Force); err != nil {
		return err
	}

	brief := `# Superopen agent brief

Prefer .so/ before raw exploration.

- Graph: .so/graph/graph.json - query with ` + "`so graph query`" + `
- Knowledge: .so/knowledge/
- Rules: .so/rules/
- Skills: .so/skills/
- Guardrails: .so/guardrails/guardrails.yaml
- Active Context: .so/memory/active-context.md (SessionStart inject)
`
	if len(p.Agents) > 0 {
		brief += "\n## Existing agent instructions discovered\n\n"
		for _, a := range p.Agents {
			brief += fmt.Sprintf("- %s (%s)\n", a.Path, a.Kind)
		}
	}
	if opts.Force {
		_ = os.WriteFile(paths.AgentBrief, []byte(brief), 0o644)
	} else if _, err := os.Stat(paths.AgentBrief); err != nil {
		_ = os.WriteFile(paths.AgentBrief, []byte(brief), 0o644)
	}

	if _, err := os.Stat(paths.EvalsHistory); err != nil {
		_ = os.WriteFile(paths.EvalsHistory, []byte("[]\n"), 0o644)
	}
	if _, err := os.Stat(paths.PendingRecs); err != nil {
		_ = os.WriteFile(paths.PendingRecs, []byte("[]\n"), 0o644)
	}
	if _, err := os.Stat(paths.RecsHistory); err != nil {
		_ = os.WriteFile(paths.RecsHistory, []byte("[]\n"), 0o644)
	}
	if _, err := os.Stat(paths.Lessons); err != nil {
		_ = os.WriteFile(paths.Lessons, []byte("# Lessons\n\nApproved recommendations land here over time.\n"), 0o644)
	}
	_ = os.MkdirAll(filepath.Join(paths.MemoryDir, "history"), 0o755)
	_ = os.MkdirAll(paths.AuditDir, 0o755)
	if _, err := os.Stat(paths.SessionsIndex); err != nil {
		_ = os.WriteFile(paths.SessionsIndex, []byte("[]\n"), 0o644)
	}

	if data, err := json.MarshalIndent(p, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(paths.Root, "discovery.json"), data, 0o644)
	}
	_ = seedSOGitignore(paths, opts.TemplateRoot)
	return nil
}

func seedSOGitignore(paths harness.Paths, templateRoot string) error {
	dst := filepath.Join(paths.Root, ".gitignore")
	if _, err := os.Stat(dst); err == nil {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(templateRoot, "so.gitignore"))
	if err != nil {
		data = []byte(`*
!.gitignore
!knowledge/
!knowledge/**
!rules/
!rules/**
!skills/
!skills/**
!guardrails/
!guardrails/**
!evals/
!evals/**
!AGENT.md
!config.yaml
`)
	}
	return os.WriteFile(dst, data, 0o644)
}


func seedGuardrails(paths harness.Paths, p discover.Profile, templateRoot string, force bool) error {
	if !force {
		if _, err := os.Stat(paths.GuardrailsFile); err == nil {
			return nil
		}
	}
	rules := []guardrails.Rule{
		{ID: "no-secrets", Description: "Never commit secrets, API keys, or credentials", Severity: "block"},
		{ID: "no-generated", Description: "Never edit generated files (dist/, vendor/, *.generated.*)", Severity: "block"},
		{ID: "run-tests", Description: "Always run relevant tests before finishing", Severity: "warn"},
		{ID: "respect-formatting", Description: "Respect project formatting and lint rules", Severity: "warn"},
		{ID: "avoid-unrelated", Description: "Avoid changing unrelated code", Severity: "warn"},
	}

	for i, r := range p.DerivedRules {
		if i >= 12 {
			break
		}
		sev := "warn"
		lower := strings.ToLower(r)
		if strings.Contains(lower, "never") || strings.Contains(lower, "must not") || strings.Contains(lower, "forbidden") {
			sev = "block"
		}
		rules = append(rules, guardrails.Rule{
			ID: fmt.Sprintf("from-agents-%d", i+1), Description: r, Severity: sev, Source: "agent-md",
		})
	}

	themes := strings.ToLower(strings.Join(p.Themes, " ") + " " + strings.Join(p.DerivedRules, " "))
	if strings.Contains(themes, "race") || strings.Contains(themes, "concurren") || strings.Contains(themes, "goroutine") {
		rules = append(rules, guardrails.Rule{
			ID: "no-shared-mutable", Description: "Do not share mutable state across goroutines without synchronization", Severity: "block", Source: "theme",
		})
	}
	if strings.Contains(themes, "sql") {
		rules = append(rules, guardrails.Rule{
			ID: "no-sql-concat", Description: "Never build SQL with string concatenation; use parameterized queries", Severity: "block", Source: "theme",
		})
	}
	if strings.Contains(themes, "secret") || strings.Contains(themes, "credential") {
		rules = append(rules, guardrails.Rule{
			ID: "no-credential-logs", Description: "Never log credentials, tokens, or secrets", Severity: "block", Source: "theme",
		})
	}
	if strings.Contains(themes, "pr title") {
		rules = append(rules, guardrails.Rule{
			ID: "pr-title-convention", Description: "Follow repository PR title conventions when creating pull requests", Severity: "warn", Source: "theme",
		})
	}

	def := guardrails.DefaultPolicy()
	out := guardrails.File{
		Rules:          dedupeRules(rules),
		Approval:       def.Approval,
		RedactOutput:   def.RedactOutput,
		DeniedCommands: def.DeniedCommands,
		SensitivePaths: def.SensitivePaths,
	}
	// Prefer template enforcement block when present.
	if data, err := os.ReadFile(filepath.Join(templateRoot, "guardrails", "guardrails.yaml")); err == nil {
		var tmpl guardrails.File
		if yaml.Unmarshal(data, &tmpl) == nil {
			if len(tmpl.DeniedCommands) > 0 {
				out.DeniedCommands = tmpl.DeniedCommands
			}
			if len(tmpl.SensitivePaths) > 0 {
				out.SensitivePaths = tmpl.SensitivePaths
			}
			if tmpl.Approval != "" {
				out.Approval = tmpl.Approval
			}
			out.RedactOutput = tmpl.RedactOutput
			if force && len(tmpl.Rules) > 0 && len(p.DerivedRules) == 0 {
				out.Rules = tmpl.Rules
			}
		}
	}
	data, err := yaml.Marshal(out)
	if err != nil {
		return err
	}
	header := "# Generated by so init - advisory rules + enforcement.\n# Edit freely; so sync will not overwrite this file.\n"
	return os.WriteFile(paths.GuardrailsFile, append([]byte(header), data...), 0o644)
}

func seedEvals(paths harness.Paths, p discover.Profile, force bool) error {
	if !force {
		if _, err := os.Stat(paths.EvalsConfig); err == nil {
			return nil
		}
	}
	checks := []string{"tests", "lint", "scope", "retries", "harness_usage"}
	stack := strings.ToLower(p.Stack)
	themes := strings.ToLower(strings.Join(p.Themes, " ") + " " + strings.Join(p.DerivedRules, " "))

	if p.Graph.Languages["go"] > 0 || strings.Contains(stack, "go") {
		checks = append(checks, "go_build")
	}
	if strings.Contains(themes, "race") || strings.Contains(themes, "concurren") {
		checks = append(checks, "race_patterns")
	}
	if strings.Contains(themes, "secret") || strings.Contains(themes, "credential") {
		checks = append(checks, "no_secrets_in_diff")
	}
	if strings.Contains(themes, "pr title") {
		checks = append(checks, "pr_title_convention")
	}
	if strings.Contains(themes, "github") || strings.Contains(themes, "gh ") {
		checks = append(checks, "prefer_gh_cli")
	}
	if strings.Contains(themes, "sql") {
		checks = append(checks, "sql_parameterized")
	}

	var b strings.Builder
	b.WriteString("# Generated by so init from graph + existing agent instruction files.\n")
	b.WriteString("checks:\n")
	seen := map[string]bool{}
	for _, c := range checks {
		if seen[c] {
			continue
		}
		seen[c] = true
		b.WriteString("  - " + c + "\n")
	}
	if len(p.DerivedRules) > 0 {
		b.WriteString("\n# Rules mirrored from agent markdown (for LLM judge context)\nagent_rules:\n")
		for i, r := range p.DerivedRules {
			if i >= 15 {
				break
			}
			b.WriteString("  - " + yamlQuote(r) + "\n")
		}
	}
	if len(p.Agents) > 0 {
		b.WriteString("\nsources:\n")
		for _, a := range p.Agents {
			b.WriteString("  - " + a.Path + "\n")
		}
	}
	return os.WriteFile(paths.EvalsConfig, []byte(b.String()), 0o644)
}

func enrichArchitecture(content string, p discover.Profile) string {
	var extra strings.Builder
	extra.WriteString("\n## Graph snapshot\n\n")
	extra.WriteString(fmt.Sprintf("- Nodes: %d\n- Edges: %d\n", p.Graph.NodeCount, p.Graph.EdgeCount))
	if len(p.Graph.TopDirs) > 0 {
		extra.WriteString("- Top directories: " + strings.Join(p.Graph.TopDirs, ", ") + "\n")
	}
	if len(p.Graph.Languages) > 0 {
		var langs []string
		for k, v := range p.Graph.Languages {
			langs = append(langs, fmt.Sprintf("%s (%d)", k, v))
		}
		extra.WriteString("- Languages: " + strings.Join(langs, ", ") + "\n")
	}
	if len(p.Agents) > 0 {
		extra.WriteString("\n## From existing agent docs\n\n")
		for _, a := range p.Agents {
			if len(a.Headings) == 0 {
				continue
			}
			extra.WriteString(fmt.Sprintf("### %s\n\n", filepath.Base(a.Path)))
			for i, h := range a.Headings {
				if i >= 8 {
					break
				}
				extra.WriteString("- " + h + "\n")
			}
			extra.WriteString("\n")
		}
	}
	return strings.TrimRight(content, "\n") + "\n" + extra.String()
}

func enrichConventions(content string, p discover.Profile) string {
	if len(p.DerivedRules) == 0 {
		return content
	}
	var extra strings.Builder
	extra.WriteString("\n## Extracted from AGENTS.md / CLAUDE.md / Cursor rules\n\n")
	for i, r := range p.DerivedRules {
		if i >= 20 {
			break
		}
		extra.WriteString("- " + r + "\n")
	}
	return strings.TrimRight(content, "\n") + "\n" + extra.String()
}

func dedupeRules(in []guardrails.Rule) []guardrails.Rule {
	seen := map[string]bool{}
	var out []guardrails.Rule
	for _, r := range in {
		k := strings.ToLower(r.Description)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, r)
	}
	return out
}

func yamlQuote(s string) string {
	s = strings.ReplaceAll(s, `"`, `'`)
	return `"` + s + `"`
}
