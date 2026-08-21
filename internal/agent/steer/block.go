// Package steer installs user-level graph-first durable context and
// produces compact hook reminders for coding agents.
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
If this workspace has no `+"`.so/`"+` directory (`+"`test -d .so`"+` is false), ignore Superopen entirely: do not run `+"`so`"+`, do not load `+"`/so`"+`, and do not run `+"`so init`"+` unless the user explicitly asks.

When `+"`.so/`"+` exists:

## Superopen code graph

This project has a code graph at `+"`.so/`"+` (query with `+"`so graph query`"+`).

Rules:
- For codebase questions, first run `+"`so graph query \"<question>\"`"+` when `+"`.so/db/so.db`"+` exists. Use `+"`so graph snippet \"<qn>\"`"+` for a known symbol and `+"`so graph trace \"<qn>\"`"+` for callers. These return a scoped subgraph, usually much smaller than raw grep output.
- Do not spawn Explore/Agent for codebase questions; run `+"`so graph query`"+` in this session. If you do spawn a subagent, it must run `+"`so graph query`"+` first.
- Only Grep/Read source after the graph has oriented you, or to modify/debug specific lines.

Graph builds are local (no LLM). Live refresh runs on session boundaries and while `+"`so dev`"+` is up (~60s poll).
`) + "\n"
}

// HookReminder is a short additionalContext string for session/start hooks.
func HookReminder() string {
	return "Superopen: this repo is indexed. Run `so graph query \"<question>\"` before Grep/Read."
}

// SearchNudge is injected on PreToolUse for Bash/Grep.
func SearchNudge() string {
	return "MANDATORY: .so/ exists. You MUST run `so graph query \"<question>\"` before grepping raw files. Only grep after Superopen has oriented you, or to modify/debug specific lines."
}

// ReadNudge is injected on PreToolUse for Read/Glob.
func ReadNudge() string {
	return "MANDATORY: .so/ exists. You MUST run `so graph query \"<question>\"` before reading source files. Use: `so graph query \"<question>\"` (scoped subgraph), `so graph snippet \"<qn>\"` for a known symbol, or `so graph trace \"<qn>\"` for a relationship. Only read raw files after Superopen has oriented you, or to modify/debug specific lines. This rule applies to subagents too — include it in every subagent prompt involving code exploration."
}

// ReadDenyReason is the permissionDecisionReason for strict-mode first Read deny.
func ReadDenyReason() string {
	return "Superopen strict mode: this project has an indexed code graph. Run `so graph query \"<your question>\"` FIRST to orient yourself, then re-issue this Read — it will be allowed. This block fires at most once per session; reading raw files to modify or debug specific lines is fine after one query."
}

// CursorRule is the short alwaysApply gate installed into ~/.cursor/rules.
func CursorRule() string {
	return strings.TrimSpace(`
If this workspace has no `+"`.so/`"+` directory, ignore Superopen entirely: do not run `+"`so`"+`, do not load the `+"`/so`"+` skill, and do not run `+"`so init`"+` unless the user explicitly asks.

If `+"`.so/`"+` exists, query the graph first (`+"`so graph query`"+`) before Grep/Read. This applies to you and to every subagent you spawn. Do not skip the graph by spawning Explore.
`) + "\n"
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
	fmt.Fprintf(&b, "Superopen graph: %d hit(s) for %q — prefer `so graph search` / `so graph snippet` / `so graph trace` over filesystem search here.\n", len(hits), term)
	for _, hit := range hits {
		fmt.Fprintf(&b, "  %s %s %s %s\n", hit.QualifiedName, hit.Label, hit.File, hit.Lines)
	}
	b.WriteString("Run `so graph snippet` with a qualified name above to read a body without grepping.")
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
