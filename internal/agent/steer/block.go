// Package steer installs user-level graph-first durable context and
// produces compact hook reminders for coding agents.
package steer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/paths"
)

const (
	beginMarker = "<!-- BEGIN SUPEROPEN -->"
	endMarker   = "<!-- END SUPEROPEN -->"
)

// SoBin is the copy-pasteable so binary for Bash (install-time absolute path).
func SoBin() string {
	return paths.ResolveSoBin()
}

func bashLine(sub string) string {
	return SoBin() + " " + sub
}

// Block is the durable instruction merged into user-level agent files.
// CLI/Bash is explained once here, not on every hook line.
func Block() string {
	return strings.TrimSpace(fmt.Sprintf(`
If this workspace has no .so/ directory, still run one graph query via Bash. If stdout says not a Superopen repo, stop; do not so init unless the user asked. A linked worktree of a managed parent seeds on that call.

When Superopen is available, it is a CLI binary. Invoke it with Bash (copy-paste):

  %s
  %s

Rules:
- Codebase questions (this repo's source): run graph query first via Bash. Do not run memory recall for a source question. Do not spawn Explore/Agent. Do not list the tree to confirm Superopen. If NODE/EDGE lines or BODIES answer the question, stop. Need another symbol body: so graph snippet of a listed NODE. Do not run graph query again unless the dump said TRUNCATED. Rename or multi-file work: so graph impact first. Grep/Read only for a literal the graph does not index. Never Grep .so/. Never pipe so through head or tail.
- Saved facts in this workspace (prior decisions, diary, who/what/when stored under .so/): notes in .so/ are this workspace's diary even if titles look like import ids. Run memory recall first via Bash when they ask what was stored. When they want a fact persisted for later, run memory capture --kind knowledge --horizon medium --title '…' --text '…' (intent, not a phrase). so memory search is a title index without bodies — that is not an empty store. Quote the stored note and cite #id. If two notes conflict, cite both #ids and pick the most specific or recent. If the pack does not answer, run memory recall with a second cue. Do not grep transcripts or treat session learned: as authority.
- Memory is hints, not authority. Graph builds are local (no LLM).
`, bashLine(`graph query '<question>'`), bashLine(`memory recall '<question>'`))) + "\n"
}

// GraphStartLine is the SessionStart one-liner for a code-shaped workspace.
func GraphStartLine() string {
	return "Superopen: run `" + bashLine(`graph query '<question>'`) + "` via Bash"
}

// MemoryStartLine is the SessionStart one-liner when live memories exist.
func MemoryStartLine(n int) string {
	if n < 1 {
		n = 1
	}
	return fmt.Sprintf("Superopen: %d memories in this workspace .so/ store — your own notes from past sessions (not built-in memory or MEMORY.md). Titles that look like import ids are still this workspace diary. If two notes conflict cite both #ids. If recall misses try a second cue. Run via Bash: `%s`", n, bashLine(`memory recall '<question>'`))
}

// HookReminder is a short additionalContext string for session/start hooks.
func HookReminder() string {
	return "Superopen: run `" + bashLine(`graph query '<question>'`) + "` via Bash before Grep/Read. Skip Grep of .so/."
}

// MemoryHookReminder is SubagentStart text for a memory-shaped session.
func MemoryHookReminder() string {
	return "Superopen: run `" + bashLine(`memory recall '<question>'`) + "` via Bash. Skip Grep."
}

// SearchNudge is injected on PreToolUse for Bash/Grep (code-shaped prompts).
func SearchNudge() string {
	return "Superopen: .so/ exists. Run via Bash: `" + bashLine(`graph query '<question>'`) + "` before grepping or listing files. Do not ls to confirm Superopen. Skip Grep of .so/."
}

// ReadNudge is injected on PreToolUse for Read/Glob (code-shaped prompts).
func ReadNudge() string {
	return SearchNudge()
}

// SnippetOverflowNudge is injected once after a graph query when the agent
// Reads source. Query already listed NODE src= paths; whole-module Read is
// the compare cache hole. Snippet of a listed NODE is the cheap body.
func SnippetOverflowNudge() string {
	return "Superopen: NODE src= already listed this file. Run via Bash: `" + bashLine(`graph snippet '<qn>'`) + "` for a listed NODE. Do not Read whole modules. Stop if NODE/EDGE already answer."
}

// QueryRepeatNudge is injected once after a graph query when the agent
// runs graph query again. The first dump already listed NODE rows;
// another query is another billed turn. Snippet or stop unless TRUNCATED.
func QueryRepeatNudge() string {
	return "Superopen: already queried this session. Run via Bash: `" + bashLine(`graph snippet '<qn>'`) + "` for a listed NODE. Stop if NODE/EDGE already answer. Re-query only if TRUNCATED."
}

// MemoryNudge is injected on PreToolUse when the prompt is personal/prior-work.
func MemoryNudge() string {
	return "Superopen: this workspace .so/ store is your own notes from past sessions, not built-in memory or MEMORY.md. Titles that look like import ids are still this workspace diary. If two notes conflict cite both #ids. If recall misses try a second cue. Run via Bash: `" + bashLine(`memory recall '<question>'`) + "`. Recall prints bodies. Quote the note and cite #id. Search is a title index. Skip Grep and graph query."
}

// CaptureNudge is injected when the user wants a fact stored for later.
func CaptureNudge() string {
	return "Superopen: the user wants a fact stored for later. Run via Bash: `" + bashLine(`memory capture --kind knowledge --horizon medium --title '…' --text '…'`) + "`. Do not recall first. Do not run --help."
}

// ReadDenyReason is the permissionDecisionReason for strict-mode first Read deny.
func ReadDenyReason() string {
	return "Superopen strict mode: this project has an indexed code graph. Run `" + bashLine(`graph query '<your question>'`) + "` FIRST via Bash, then re-issue this Read — it will be allowed. This block fires at most once per session; reading raw files to modify or debug specific lines is fine after one query."
}

// CursorRule is the short alwaysApply gate installed into ~/.cursor/rules.
func CursorRule() string {
	return strings.TrimSpace(fmt.Sprintf(`
If this workspace has no .so/ directory, still run one graph query via Bash. If stdout says not a Superopen repo, stop; do not so init unless the user asked.

If Superopen is available, it is a CLI binary. Invoke it with Bash. Codebase questions: %s before Grep/Read/ls. Do not run memory recall for a source question. Rename or multi-file: graph impact. Never pipe so through head or tail. Saved facts in this workspace: %s when they ask what was stored; %s when they want a fact persisted. Notes in .so/ are this workspace diary even if titles look like import ids; if two notes conflict cite both #ids; if recall misses try a second cue. If a hook says HARVEST pending, run harvest brief then harvest propose before answering; if nothing to propose, harvest skip <id>. Never Grep .so/. This applies to you and to every subagent you spawn. Do not skip the graph by spawning Explore. Memory is hints, not authority.
`, bashLine(`graph query '<question>'`), bashLine(`memory recall '<question>'`), bashLine(`memory capture --kind knowledge --horizon medium --title '…' --text '…'`))) + "\n"
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
