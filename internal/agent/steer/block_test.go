package steer

import (
	"testing"

	"github.com/ishanjainn/superopen/internal/paths"
)

func TestMergeBlockIdempotent(t *testing.T) {
	first := MergeBlock("hello\n")
	second := MergeBlock(first)
	if first != second {
		t.Fatalf("merge not idempotent:\n%s\n---\n%s", first, second)
	}
	if !containsAll(first, beginMarker, endMarker) {
		t.Fatalf("missing markers: %s", first)
	}
	if !paths.MentionsCommand(first, "graph query") {
		t.Fatalf("missing graph query invocation: %s", first)
	}
	if contains(first, "so-verify") || contains(first, "so-scout") || contains(first, "so-auditor") {
		t.Fatalf("always-on block must not name subagents: %s", first)
	}
	if contains(first, "--json") {
		t.Fatalf("always-on block must not mention --json: %s", first)
	}
	if contains(first, "run `so init` once") {
		t.Fatalf("block must not auto-init unmanaged repos: %s", first)
	}
	if contains(first, "ignore Superopen entirely") {
		t.Fatalf("block must still try one graph query when .so/ is missing: %s", first)
	}
	if !contains(first, "still run one graph query") {
		t.Fatalf("block must try one graph query when .so/ is missing: %s", first)
	}
	if !contains(first, "Do not spawn Explore") {
		t.Fatalf("block must close the Explore hole: %s", first)
	}
	if !contains(first, "Do not run memory recall for a source question") {
		t.Fatalf("block must not send codebase questions to memory: %s", first)
	}
	if !contains(first, "list the tree") {
		t.Fatalf("block must say not to list the tree to confirm Superopen: %s", first)
	}
	if !contains(first, "TRUNCATED") {
		t.Fatalf("block must allow a follow-up query only after TRUNCATED: %s", first)
	}
	if paths.MentionsCommand(first, "graph trace") || paths.MentionsCommand(first, "graph search") {
		t.Fatalf("always-on block must not list trace/search as the default: %s", first)
	}
	if !paths.MentionsCommand(first, "graph snippet") {
		t.Fatalf("block must name snippet as overflow for a listed NODE: %s", first)
	}
	if !contains(first, "BODIES") {
		t.Fatalf("block must tell agents to stop on query BODIES: %s", first)
	}
	if !paths.MentionsCommand(first, "graph impact") {
		t.Fatalf("block must name graph impact for multi-file work: %s", first)
	}
	if !contains(first, "head or tail") {
		t.Fatalf("block must forbid piping so through head or tail: %s", first)
	}
	if !paths.MentionsCommand(first, "memory search") {
		t.Fatalf("block must say search is a title index: %s", first)
	}
	if !contains(first, "memory recall") {
		t.Fatalf("block must point prior-work at memory recall: %s", first)
	}
	if !contains(first, "memory capture") {
		t.Fatalf("block must name memory capture when the user wants a fact stored: %s", first)
	}
	if !contains(first, "import ids") {
		t.Fatalf("block must say import-looking titles are this workspace diary: %s", first)
	}
	if !contains(first, "cite both") {
		t.Fatalf("block must say to cite both conflicting notes: %s", first)
	}
	if !contains(first, "second cue") {
		t.Fatalf("block must say to recall with a second cue: %s", first)
	}
	if !contains(first, "CLI binary") {
		t.Fatalf("block must say so is a CLI binary: %s", first)
	}
	if !contains(first, "shell") {
		t.Fatalf("block must say invoke with the shell tool: %s", first)
	}
	if !contains(first, "PowerShell") {
		t.Fatalf("block must name PowerShell for Windows hosts: %s", first)
	}
	if contains(first, "personal questions") {
		t.Fatalf("block must not frame diary as personal/privacy: %s", first)
	}
	if !contains(first, "learned:") {
		t.Fatalf("block must say learned: is not authority: %s", first)
	}
}

func TestNudgesAreOneLinersWithoutQuotes(t *testing.T) {
	if paths.MentionsCommand(SearchNudge(), "graph search") || paths.MentionsCommand(ReadNudge(), "graph search") {
		t.Fatal("nudges must not list so graph search (spray menu)")
	}
	for _, n := range []string{SearchNudge(), ReadNudge(), MemoryNudge(), CaptureNudge(), GraphStartLine(), MemoryStartLine(3), HookReminder(), MemoryHookReminder(), SnippetOverflowNudge(), QueryRepeatNudge()} {
		if contains(n, "MANDATORY") {
			t.Fatalf("hooks must not say MANDATORY: %s", n)
		}
		if contains(n, "\n") {
			t.Fatalf("nudge must be one line: %q", n)
		}
		if contains(n, `"`) {
			t.Fatalf("nudge must not contain double quotes (OpenCode/Pi echo): %q", n)
		}
		if !contains(n, "shell") {
			t.Fatalf("nudge should say shell: %s", n)
		}
	}
	if contains(SearchNudge(), "not an MCP") || contains(ReadNudge(), "not an MCP") || contains(MemoryNudge(), "not an MCP") {
		t.Fatal("per-hook lines must not repeat the MCP denial")
	}
	if !contains(MemoryNudge(), "memory recall") || contains(MemoryNudge(), "graph query") && !contains(MemoryNudge(), "Skip Grep and graph query") {
		t.Fatal("memory nudge must point at recall and skip graph query")
	}
	if !contains(MemoryNudge(), "import ids") || !contains(MemoryStartLine(2), "import ids") {
		t.Fatal("memory steer must say import-looking titles are this workspace diary")
	}
	if !contains(MemoryNudge(), "cite both") || !contains(MemoryStartLine(2), "cite both") {
		t.Fatal("memory steer must say to cite both conflicting notes")
	}
	if !contains(MemoryNudge(), "second cue") || !contains(MemoryStartLine(2), "second cue") {
		t.Fatal("memory steer must say to try a second cue")
	}
	if !contains(MemoryNudge(), "MEMORY.md") || !contains(MemoryStartLine(2), "MEMORY.md") {
		t.Fatal("memory steer must disambiguate host MEMORY.md")
	}
	if !contains(CaptureNudge(), "memory capture") || contains(CaptureNudge(), "memory recall") {
		t.Fatal("capture nudge must point at capture, not recall")
	}
	if !contains(SearchNudge(), ".so/") || !contains(HookReminder(), ".so/") {
		t.Fatal("search/reminder should tell agents not to Grep .so/")
	}
	if paths.MentionsCommand(ReadNudge(), "graph snippet") || paths.MentionsCommand(ReadNudge(), "graph trace") {
		t.Fatal("read nudge must not list snippet/trace (spray menu)")
	}
	if ReadNudge() != SearchNudge() {
		t.Fatal("read nudge should match search nudge (query only)")
	}
	overflow := SnippetOverflowNudge()
	if !paths.MentionsCommand(overflow, "graph snippet") {
		t.Fatal("post-query overflow must name snippet")
	}
	if paths.MentionsCommand(overflow, "graph search") || paths.MentionsCommand(overflow, "graph trace") {
		t.Fatal("overflow must not spray search/trace")
	}
	if contains(overflow, "MANDATORY") || contains(overflow, `"`) || contains(overflow, "\n") {
		t.Fatalf("overflow must be one line without quotes: %q", overflow)
	}
	repeat := QueryRepeatNudge()
	if !paths.MentionsCommand(repeat, "graph snippet") || !contains(repeat, "TRUNCATED") {
		t.Fatalf("repeat-query overflow must name snippet and TRUNCATED: %q", repeat)
	}
	if contains(SearchNudge(), "Do not ls") == false {
		t.Fatal("search nudge must discourage listing to confirm Superopen")
	}
}

func TestCursorRuleIsShortGate(t *testing.T) {
	rule := CursorRule()
	if contains(rule, "ignore Superopen entirely") {
		t.Fatalf("rule must still try one graph query when .so/ is missing: %s", rule)
	}
	if !contains(rule, "still run one graph query") {
		t.Fatalf("rule must try one graph query when .so/ is missing: %s", rule)
	}
	if !contains(rule, "graph impact") {
		t.Fatalf("rule must name graph impact for multi-file work: %s", rule)
	}
	if !contains(rule, "head or tail") {
		t.Fatalf("rule must forbid piping so through head/tail: %s", rule)
	}
	if contains(rule, "memory_search") {
		t.Fatalf("alwaysApply rule must not dump the memory playbook: %s", rule)
	}
	if !contains(rule, "memory recall") {
		t.Fatalf("alwaysApply rule should point prior-work at memory recall: %s", rule)
	}
	if !contains(rule, "memory capture") {
		t.Fatalf("alwaysApply rule should name memory capture when they want a fact stored: %s", rule)
	}
	if !contains(rule, "query") {
		t.Fatalf("alwaysApply rule should mention query-first: %s", rule)
	}
	if !contains(rule, "/ls") && !contains(rule, "Read/ls") {
		t.Fatalf("alwaysApply rule should query before ls: %s", rule)
	}
	if !contains(rule, "subagent") {
		t.Fatalf("alwaysApply rule must apply to spawned subagents: %s", rule)
	}
}

func TestStripBlock(t *testing.T) {
	merged := MergeBlock("keep me\n")
	stripped := StripBlock(merged)
	if containsAll(stripped, beginMarker) {
		t.Fatalf("marker remained: %s", stripped)
	}
	if stripped != "keep me\n" {
		t.Fatalf("got %q", stripped)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if len(p) == 0 || !contains(s, p) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
