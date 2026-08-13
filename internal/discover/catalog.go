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
	// Skill body (kind=skill). Catalog does not emit template skills;
	// upgrade JSON may include repo-learned skill bodies.
	Body string `json:"body,omitempty"`
	// Guardrail fields
	GuardrailID string `json:"guardrail_id,omitempty"`
	Severity    string `json:"severity,omitempty"`
	Score       int    `json:"score,omitempty"`
}

// CatalogCandidates ranks MCP and guardrail suggestions from stack signals.
// MCP is never auto-committed by heuristic seed — assistants pick via upgrade JSON.
// Skills are not catalogued as templates; upgrade may include repo-learned skills only.
func CatalogCandidates(repoRoot string, paths harness.Paths, sig StackSignals, graph GraphSummary) []Candidate {
	_ = graph
	existingMCP := existingMCPNames(repoRoot, paths)
	existingGuards := existingGuardrailIDs(paths)

	var all []Candidate
	all = append(all, mcpCandidates(sig, existingMCP)...)
	all = append(all, guardrailCandidates(sig, existingGuards)...)

	// Cap per kind: show more MCP in the brief; upgrade JSON asks for 1-2.
	return capByKind(all, map[CandidateKind]int{
		CandidateMCP:       4,
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
