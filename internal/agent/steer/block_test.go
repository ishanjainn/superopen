package steer

import "testing"

func TestMergeBlockIdempotent(t *testing.T) {
	first := MergeBlock("hello\n")
	second := MergeBlock(first)
	if first != second {
		t.Fatalf("merge not idempotent:\n%s\n---\n%s", first, second)
	}
	if !containsAll(first, beginMarker, endMarker, "Superopen code graph", "Superopen memory") {
		t.Fatalf("missing markers/content: %s", first)
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
