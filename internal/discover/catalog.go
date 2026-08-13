package discover

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/harness"
)

// CandidateKind is the automation category for a setup candidate.
type CandidateKind string

const (
	CandidateMCP       CandidateKind = "mcp"
	CandidateSkill     CandidateKind = "skill"
	CandidateGuardrail CandidateKind = "guardrail"
)

// Candidate is a ranked automation suggestion for upgrade / leftover HITL.
type Candidate struct {
	ID        string        `json:"id"`
	Kind      CandidateKind `json:"kind"`
	Name      string        `json:"name"`
	Title     string        `json:"title"`
	Rationale string        `json:"rationale"`
	Evidence  []string      `json:"evidence,omitempty"`
	// MCP fields (kind=mcp)
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	// Skill body (kind=skill); empty means use DefaultSkillBody(name)
	Body string `json:"body,omitempty"`
	// Guardrail fields
	GuardrailID string `json:"guardrail_id,omitempty"`
	Severity    string `json:"severity,omitempty"`
	Score       int    `json:"score,omitempty"`
}

// CatalogCandidates ranks MCP/skill/guardrail suggestions from stack signals.
// MCP is never auto-committed by heuristic seed — assistants pick via upgrade JSON.
func CatalogCandidates(repoRoot string, paths harness.Paths, sig StackSignals, graph GraphSummary) []Candidate {
	existingSkills := existingSkillNames(repoRoot)
	existingMCP := existingMCPNames(repoRoot, paths)
	existingGuards := existingGuardrailIDs(paths)

	var all []Candidate
	all = append(all, mcpCandidates(sig, existingMCP)...)
	all = append(all, skillCandidates(sig, graph, existingSkills)...)
	all = append(all, guardrailCandidates(sig, existingGuards)...)

	// Cap per kind: top 2 MCP, top 2 skills, top 2 guardrails (upgrade brief can show more)
	return capByKind(all, map[CandidateKind]int{
		CandidateMCP:       4, // show more in brief; upgrade JSON asks for 1-2
		CandidateSkill:     4,
		CandidateGuardrail: 2,
	})
}

func mcpCandidates(sig StackSignals, existing map[string]bool) []Candidate {
	var out []Candidate
	add := func(c Candidate) {
		if existing[strings.ToLower(c.Name)] {
			return
		}
		// Never recommend Memory MCP — Superopen memory already covers this.
		if strings.EqualFold(c.Name, "memory") || strings.EqualFold(c.Name, "memory-mcp") {
			return
		}
		out = append(out, c)
	}

	if sig.HasDep("react") || sig.HasDep("next") || sig.HasDep("vue") || sig.HasDep("express") ||
		sig.HasDep("fastapi") || sig.HasDep("django") || sig.HasDep("prisma") || sig.HasDep("stripe") ||
		sig.HasManifest("package.json") || sig.HasManifest("go.mod") {
		add(Candidate{
			ID: "mcp-context7", Kind: CandidateMCP, Name: "context7",
			Title:     "Add context7 MCP for live library docs",
			Rationale: "Popular libraries/SDKs benefit from live docs instead of training-data APIs.",
			Evidence:  evidenceFor(sig, "popular libraries or framework deps"),
			Command:   "npx", Args: []string{"-y", "@upstash/context7-mcp@1.0.0"},
			Score: 80,
		})
	}
	if sig.HasDep("playwright") || sig.HasConfig("playwright.config") || (sig.IsFrontend() && (sig.HasDir("tests") || sig.HasDir("e2e") || sig.HasConfig("vitest"))) {
		add(Candidate{
			ID: "mcp-playwright", Kind: CandidateMCP, Name: "playwright",
			Title:     "Add Playwright MCP for browser automation",
			Rationale: "Frontend/e2e stack detected — agents can drive the running app.",
			Evidence:  evidenceFor(sig, "playwright or frontend test setup"),
			Command:   "npx", Args: []string{"-y", "@playwright/mcp@0.0.10"},
			Score: 90,
		})
	}
	if sig.IsGitHub() {
		add(Candidate{
			ID: "mcp-github", Kind: CandidateMCP, Name: "github",
			Title:     "Add GitHub MCP for issues and PRs",
			Rationale: "Origin remote is GitHub — issue/PR workflows are available to agents.",
			Evidence:  []string{"git remote: " + sig.GitRemote},
			Command:   "npx", Args: []string{"-y", "@modelcontextprotocol/server-github@2025.4.8"},
			Score: 70,
		})
	}
	if sig.HasDep("sentry") {
		add(Candidate{
			ID: "mcp-sentry", Kind: CandidateMCP, Name: "sentry",
			Title:     "Add Sentry MCP for error investigation",
			Rationale: "Sentry SDK detected — agents can look up production errors.",
			Evidence:  evidenceFor(sig, "sentry dependency"),
			Command:   "npx", Args: []string{"-y", "@sentry/mcp-server@0.1.0"},
			Score: 85,
		})
	}
	return sortByScore(out)
}

func skillCandidates(sig StackSignals, graph GraphSummary, existing map[string]bool) []Candidate {
	var out []Candidate
	add := func(c Candidate) {
		if existing[strings.ToLower(c.Name)] || c.Name == "so" || c.Name == "superopen" {
			return
		}
		if c.Body == "" {
			c.Body = DefaultSkillBody(c.Name)
		}
		out = append(out, c)
	}

	if sig.HasDir("tests") || sig.HasDir("test") || sig.HasDir("__tests__") ||
		sig.HasConfig("jest") || sig.HasConfig("pytest") || sig.HasConfig("vitest") ||
		sig.HasManifest("go.mod") {
		add(Candidate{
			ID: "skill-gen-test", Kind: CandidateSkill, Name: "gen-test",
			Title:     "Add gen-test skill for project test conventions",
			Rationale: "Test suite or test runner config detected.",
			Evidence:  evidenceFor(sig, "tests directory or test runner"),
			Score:     85,
		})
	}
	if sig.IsGitHub() || strings.Contains(strings.ToLower(strings.Join(sig.Deps, " ")), "github") {
		add(Candidate{
			ID: "skill-pr-check", Kind: CandidateSkill, Name: "pr-check",
			Title:     "Add pr-check skill for PR review checklist",
			Rationale: "GitHub-hosted repo — PR review workflows are common.",
			Evidence:  evidenceFor(sig, "github remote or tooling"),
			Score:     75,
		})
	}
	if sig.IsFrontend() {
		add(Candidate{
			ID: "skill-frontend-design", Kind: CandidateSkill, Name: "frontend-design",
			Title:     "Add frontend-design skill for UI work",
			Rationale: "Frontend framework or UI directories detected.",
			Evidence:  evidenceFor(sig, "frontend stack"),
			Score:     80,
		})
	}
	if sig.HasDep("prisma") || sig.HasDep("alembic") || sig.HasDir("migrations") ||
		sig.HasDep("sqlalchemy") || sig.HasDep("gorm") {
		add(Candidate{
			ID: "skill-create-migration", Kind: CandidateSkill, Name: "create-migration",
			Title:     "Add create-migration skill for schema changes",
			Rationale: "ORM/migration tooling detected.",
			Evidence:  evidenceFor(sig, "prisma/alembic/migrations"),
			Score:     88,
		})
	}
	if sig.HasDep("auth") || sig.HasDep("stripe") || sig.HasDep("passport") || sig.HasDep("jwt") {
		add(Candidate{
			ID: "skill-security-reviewer", Kind: CandidateSkill, Name: "security-reviewer",
			Title:     "Add security-reviewer skill for auth/payments code",
			Rationale: "Auth or payments dependencies detected.",
			Evidence:  evidenceFor(sig, "auth or payments deps"),
			Score:     92,
		})
	}
	if graph.NodeCount > 500 {
		add(Candidate{
			ID: "skill-code-reviewer", Kind: CandidateSkill, Name: "code-reviewer",
			Title:     "Add code-reviewer skill for large codebases",
			Rationale: "Large graph — specialized review skill reduces wandering.",
			Evidence:  []string{"graph nodes=" + itoa(graph.NodeCount)},
			Score:     60,
		})
	}
	// Prefer unused templates when stack is generic Go/Node without stronger signals.
	if len(out) == 0 && (sig.HasManifest("go.mod") || sig.HasManifest("package.json")) {
		add(Candidate{
			ID: "skill-testing", Kind: CandidateSkill, Name: "testing",
			Title:     "Add testing skill from project templates",
			Rationale: "Baseline testing guidance for this stack.",
			Evidence:  evidenceFor(sig, "go.mod or package.json"),
			Score:     50,
		})
	}
	return sortByScore(out)
}

func guardrailCandidates(sig StackSignals, existing map[string]bool) []Candidate {
	var out []Candidate
	if sig.HasEnvFiles && !existing["block-env-edits"] && !existing["no-secrets"] {
		out = append(out, Candidate{
			ID: "guard-block-env", Kind: CandidateGuardrail, Name: "block-env-edits",
			Title:       "Warn on edits to env/secret files",
			Rationale:   "Environment or credentials files are present.",
			Evidence:    []string{"env or credentials files detected"},
			GuardrailID: "block-env-edits",
			Severity:    "warn",
			Score:       90,
		})
	}
	if sig.HasLockfiles && !existing["block-lockfile-edits"] {
		out = append(out, Candidate{
			ID: "guard-block-lockfile", Kind: CandidateGuardrail, Name: "block-lockfile-edits",
			Title:       "Warn on direct lockfile edits",
			Rationale:   "Lockfiles should change via the package manager.",
			Evidence:    []string{"lockfile present"},
			GuardrailID: "block-lockfile-edits",
			Severity:    "warn",
			Score:       70,
		})
	}
	return sortByScore(out)
}

// DefaultSkillBody returns a short SKILL.md for known catalog skills.
func DefaultSkillBody(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "gen-test", "testing":
		return `# Generate tests

Use when adding or updating tests for a changed file.

1. Prefer existing test frameworks and helpers in the repo.
2. Cover happy path + one failure case for new behavior.
3. Do not skip tests to "finish faster".
4. Run the relevant test target before claiming done.
`
	case "pr-check":
		return "# PR check\n\nUse when reviewing a pull request or preparing one for review.\n\n1. Diff: run `gh pr diff` (or `git diff main...HEAD`).\n2. Checklist:\n   - Tests added for new behavior\n   - No secrets or credentials in the diff\n   - Scope stays on the requested task\n   - Conventional Commit PR title\n3. Mark each item pass/fail with a one-line reason.\n"
	case "frontend-design":
		return `# Frontend design

Use when building or refining UI components.

1. Match existing design tokens, spacing, and component patterns in this repo.
2. Prefer accessible markup (labels, focus, contrast) over decorative complexity.
3. Avoid generic AI aesthetics; reuse project primitives.
4. Add or update a focused component test when behavior changes.
`
	case "create-migration":
		return `# Create migration

Use when changing database schema.

1. Follow the project's existing migration tool and naming.
2. Include reversible up/down steps when the tool supports them.
3. Validate with the project's migrate/validate command before finishing.
4. Never invent production credentials; use local/.env.example only.
`
	case "security-reviewer":
		return `# Security reviewer

Use when touching auth, payments, credentials, or user data.

1. Check for secrets in code, logs, and fixtures.
2. Prefer parameterized queries and existing auth helpers.
3. Flag missing authorization checks on new endpoints.
4. Keep diffs focused; do not expand scope into unrelated refactors.
`
	case "code-reviewer":
		return "# Code reviewer\n\nUse for a focused review of a change set.\n\n1. Run `so graph query` for the touched packages before broad search.\n2. Check tests, error handling, and scope against the request.\n3. Prefer concrete file:line findings over vague advice.\n"
	case "debugging":
		return `# Debugging

1. Reproduce the failure with the smallest command.
2. Check recent Superopen sessions for similar failures.
3. Prefer reading AGENTS.md and graph query over broad greps.
4. Fix root cause; avoid drive-by refactors.
5. Add or update a regression test when practical.
`
	case "create-api":
		return "# Create API\n\nUse when adding a REST/HTTP endpoint.\n\n1. Locate the existing API router via `so graph query \"api router\"`.\n2. Match existing auth, validation, and error patterns.\n3. Add tests next to similar handlers.\n4. Update docs if the public API surface changes.\n5. Run lint/tests before finishing.\n"
	default:
		return "# " + name + "\n\nProject-specific skill. Prefer AGENTS.md, guardrails, and `so graph query` before broad exploration.\n"
	}
}

func existingSkillNames(repoRoot string) map[string]bool {
	out := map[string]bool{}
	for _, dir := range harness.SkillsCandidates(repoRoot) {
		ents, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if e.IsDir() {
				out[strings.ToLower(e.Name())] = true
			}
		}
	}
	return out
}

func existingMCPNames(repoRoot string, paths harness.Paths) map[string]bool {
	out := map[string]bool{}
	// From committed Superopen policy
	if data, err := os.ReadFile(paths.Config); err == nil {
		lower := strings.ToLower(string(data))
		for _, name := range []string{"context7", "playwright", "github", "sentry"} {
			if strings.Contains(lower, "name: "+name) || strings.Contains(lower, "name: \""+name+"\"") {
				out[name] = true
			}
		}
	}
	for _, rel := range []string{".mcp.json", ".cursor/mcp.json"} {
		data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		var doc map[string]any
		if json.Unmarshal(data, &doc) != nil {
			continue
		}
		servers, _ := doc["mcpServers"].(map[string]any)
		if servers == nil {
			servers, _ = doc["mcp"].(map[string]any)
		}
		for name := range servers {
			out[strings.ToLower(name)] = true
		}
	}
	return out
}

func existingGuardrailIDs(paths harness.Paths) map[string]bool {
	out := map[string]bool{}
	data, err := os.ReadFile(paths.GuardrailsFile)
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- id:") || strings.HasPrefix(line, "id:") {
			id := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "- id:"), "id:"))
			id = strings.Trim(id, `"'`)
			if id != "" {
				out[id] = true
			}
		}
	}
	return out
}

func evidenceFor(sig StackSignals, label string) []string {
	var parts []string
	parts = append(parts, label)
	if len(sig.Manifests) > 0 {
		parts = append(parts, "manifests: "+strings.Join(sig.Manifests, ", "))
	}
	var deps []string
	for _, d := range sig.Deps {
		if len(deps) >= 8 {
			break
		}
		deps = append(deps, d)
	}
	if len(deps) > 0 {
		parts = append(parts, "deps: "+strings.Join(deps, ", "))
	}
	return parts
}

func sortByScore(in []Candidate) []Candidate {
	out := append([]Candidate(nil), in...)
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].Score > out[i].Score {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func capByKind(in []Candidate, limits map[CandidateKind]int) []Candidate {
	counts := map[CandidateKind]int{}
	var out []Candidate
	for _, c := range in {
		lim := limits[c.Kind]
		if lim <= 0 {
			lim = 2
		}
		if counts[c.Kind] >= lim {
			continue
		}
		counts[c.Kind]++
		out = append(out, c)
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
