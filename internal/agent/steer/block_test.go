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
	if contains(first, "so graph search") {
		t.Fatalf("always-on block must not list so graph search as the default: %s", first)
	}
	if !contains(first, "so memory search") {
		t.Fatalf("block must point prior-work at so memory search: %s", first)
	}
	if !contains(first, "learned:") {
		t.Fatalf("block must say learned: is not authority: %s", first)
	}
}

func TestNudgesAreMandatoryOneLiners(t *testing.T) {
	if contains(SearchNudge(), "so graph search") || contains(ReadNudge(), "so graph search") {
		t.Fatal("nudges must not list so graph search (spray menu)")
	}
	if !contains(SearchNudge(), "MANDATORY") || !contains(ReadNudge(), "MANDATORY") {
		t.Fatal("nudges must stay MANDATORY")
	}
	if !contains(ReadNudge(), "so graph snippet") || !contains(ReadNudge(), "so graph trace") {
		t.Fatal("read nudge should list snippet/trace as focused follow-ups")
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
	if !contains(rule, "so memory search") {
		t.Fatalf("alwaysApply rule should point prior-work at so memory search: %s", rule)
	}
	if !contains(rule, "query") {
		t.Fatalf("alwaysApply rule should mention query-first: %s", rule)
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
