package steer

import "testing"

func TestMergeBlockIdempotent(t *testing.T) {
	first := MergeBlock("hello\n")
	second := MergeBlock(first)
	if first != second {
		t.Fatalf("merge not idempotent:\n%s\n---\n%s", first, second)
	}
	if !containsAll(first, beginMarker, endMarker, "so graph query") {
		t.Fatalf("missing markers/content: %s", first)
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
	if !contains(first, "ignore Superopen entirely") {
		t.Fatalf("block must gate unmanaged repos: %s", first)
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
	if contains(first, "so graph trace") || contains(first, "so graph search") {
		t.Fatalf("always-on block must not list trace/search as the default: %s", first)
	}
	if !contains(first, "so graph snippet") {
		t.Fatalf("block must name snippet as overflow for a listed NODE: %s", first)
	}
	if !contains(first, "BODIES") {
		t.Fatalf("block must tell agents to stop on query BODIES: %s", first)
	}
	if !contains(first, "so memory search") {
		t.Fatalf("block must say search is a title index: %s", first)
	}
	if !contains(first, "memory recall") {
		t.Fatalf("block must point prior-work at memory recall: %s", first)
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
	if !contains(first, "Bash") {
		t.Fatalf("block must say invoke with Bash: %s", first)
	}
	if contains(first, "personal questions") {
		t.Fatalf("block must not frame diary as personal/privacy: %s", first)
	}
	if !contains(first, "learned:") {
		t.Fatalf("block must say learned: is not authority: %s", first)
	}
}

func TestNudgesAreOneLinersWithoutQuotes(t *testing.T) {
	if contains(SearchNudge(), "so graph search") || contains(ReadNudge(), "so graph search") {
		t.Fatal("nudges must not list so graph search (spray menu)")
	}
	for _, n := range []string{SearchNudge(), ReadNudge(), MemoryNudge(), GraphStartLine(), MemoryStartLine(3), HookReminder(), MemoryHookReminder(), SnippetOverflowNudge(), QueryRepeatNudge()} {
		if contains(n, "MANDATORY") {
			t.Fatalf("hooks must not say MANDATORY: %s", n)
		}
		if contains(n, "\n") {
			t.Fatalf("nudge must be one line: %q", n)
		}
		if contains(n, `"`) {
			t.Fatalf("nudge must not contain double quotes (OpenCode/Pi echo): %q", n)
		}
		if !contains(n, "Bash") && !contains(n, "bash") {
			t.Fatalf("nudge should say Bash: %s", n)
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
	if !contains(SearchNudge(), ".so/") || !contains(HookReminder(), ".so/") {
		t.Fatal("search/reminder should tell agents not to Grep .so/")
	}
	if contains(ReadNudge(), "so graph snippet") || contains(ReadNudge(), "so graph trace") {
		t.Fatal("read nudge must not list snippet/trace (spray menu)")
	}
	if ReadNudge() != SearchNudge() {
		t.Fatal("read nudge should match search nudge (query only)")
	}
	overflow := SnippetOverflowNudge()
	if !contains(overflow, "so graph snippet") {
		t.Fatal("post-query overflow must name snippet")
	}
	if contains(overflow, "so graph search") || contains(overflow, "so graph trace") {
		t.Fatal("overflow must not spray search/trace")
	}
	if contains(overflow, "MANDATORY") || contains(overflow, `"`) || contains(overflow, "\n") {
		t.Fatalf("overflow must be one line without quotes: %q", overflow)
	}
	repeat := QueryRepeatNudge()
	if !contains(repeat, "so graph snippet") || !contains(repeat, "TRUNCATED") {
		t.Fatalf("repeat-query overflow must name snippet and TRUNCATED: %q", repeat)
	}
	if contains(SearchNudge(), "Do not ls") == false {
		t.Fatal("search nudge must discourage listing to confirm Superopen")
	}
}

func TestCursorRuleIsShortGate(t *testing.T) {
	rule := CursorRule()
	if !contains(rule, "ignore Superopen entirely") {
		t.Fatalf("missing gate: %s", rule)
	}
	if contains(rule, "memory_search") {
		t.Fatalf("alwaysApply rule must not dump the memory playbook: %s", rule)
	}
	if !contains(rule, "memory recall") {
		t.Fatalf("alwaysApply rule should point prior-work at memory recall: %s", rule)
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
