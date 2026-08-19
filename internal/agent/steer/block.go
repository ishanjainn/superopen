// Package steer installs user-level graph-first durable context and
// produces compact hook/MCP reminders for coding agents.
package steer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	beginMarker = "<!-- BEGIN SUPEROPEN -->"
	endMarker   = "<!-- END SUPEROPEN -->"
)

// Block is the durable graph-first instruction merged into user-level agent files.
func Block() string {
	return strings.TrimSpace(`
## Superopen code graph (automatic)

For structural codebase questions (architecture, callers/callees, where X is defined,
dependencies, impact, "how does Y work"), use the Superopen graph **before** broad
Read/Grep/Glob of source:

1. Prefer MCP tools `+"`graph_query` / `graph_search` / `graph_snippet` / `graph_trace` / `graph_architecture`"+` when connected.
2. Otherwise run `+"`so graph query|search|snippet|trace|architecture`"+` (binary from the /so skill).
3. Prefer one `+"`graph query`"+` first; stop when NODE/EDGE/snippet context answers the question. Use `+"`graph trace`"+` only with a qualified name when callers/callees are still needed.
4. Only fall back to reading files when graph/snippet context is insufficient.

For multi-step graph work, delegate to the `+"`so-verify`"+` subagent (or `+"`so-scout`"+` for a quick
lookup) so the exploration turns stay out of this conversation.

If this repository has no `+"`.so/`"+`, run `+"`so init`"+` once at the repository root.
Graph builds are local (no LLM). Live refresh runs on session boundaries and while `+"`so dev`"+` / MCP is up (~60s poll).
`) + "\n"
}

// HookReminder is a short additionalContext string for session/start hooks.
func HookReminder() string {
	return "Superopen: for structural code questions prefer graph_search/graph_trace/graph_snippet (or `so graph …`) before broad Read/Grep. Run `so init` if `.so/` is missing."
}

// GraphHit is one indexed symbol rendered into an explore-tool augment.
type GraphHit struct {
	QualifiedName string
	Label         string
	File          string
	Lines         string
}

// ExploreAugment renders indexed matches for the term an agent was about to
// grep for, so the hook spends its tokens on graph facts rather than a nudge.
// Returns "" when there is nothing worth injecting.
func ExploreAugment(term string, total int, hits []GraphHit) string {
	term = strings.TrimSpace(term)
	if term == "" || len(hits) == 0 {
		return ""
	}
	if total < len(hits) {
		total = len(hits)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Superopen graph: %d indexed match(es) for %q — prefer graph_search/graph_snippet/graph_trace over filesystem search here.\n", total, term)
	for _, hit := range hits {
		fmt.Fprintf(&b, "  %s %s %s %s\n", hit.QualifiedName, hit.Label, hit.File, hit.Lines)
	}
	b.WriteString("Call graph_snippet with a qualified name above to read a body without grepping.")
	return b.String()
}

// MergeBlock replaces or appends the Superopen sentinel block in content.
func MergeBlock(existing string) string {
	block := beginMarker + "\n" + Block() + endMarker + "\n"
	start := strings.Index(existing, beginMarker)
	end := strings.Index(existing, endMarker)
	if start >= 0 && end > start {
		end += len(endMarker)
		for end < len(existing) && (existing[end] == '\n' || existing[end] == '\r') {
			end++
		}
		return existing[:start] + block + existing[end:]
	}
	trimmed := strings.TrimRight(existing, " \t\r\n")
	if trimmed == "" {
		return block
	}
	return trimmed + "\n\n" + block
}

// StripBlock removes the Superopen sentinel block from content.
func StripBlock(existing string) string {
	start := strings.Index(existing, beginMarker)
	end := strings.Index(existing, endMarker)
	if start < 0 || end < start {
		return existing
	}
	end += len(endMarker)
	for end < len(existing) && (existing[end] == '\n' || existing[end] == '\r') {
		end++
	}
	out := existing[:start] + existing[end:]
	return strings.TrimSpace(out) + "\n"
}

// WriteMergedFile merges Block into path (creating parents as needed).
func WriteMergedFile(path string) error {
	prev, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	next := MergeBlock(string(prev))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(next), 0o644)
}

// RemoveFromFile strips the Superopen block; removes the file if empty afterward.
func RemoveFromFile(path string) error {
	prev, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	next := strings.TrimSpace(StripBlock(string(prev)))
	if next == "" {
		return os.Remove(path)
	}
	return os.WriteFile(path, []byte(next+"\n"), 0o644)
}
