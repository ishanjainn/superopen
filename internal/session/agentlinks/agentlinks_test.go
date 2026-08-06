package agentlinks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractAgentID(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`{"agentId":"abc-123"}`, "abc-123"},
		{"Result\n\nagentId: subX (for resuming)", "subX"},
		{`{"ok":true,"agent_id":"task-9"}`, "task-9"},
		{"no id here", ""},
	}
	for _, tc := range cases {
		if got := ExtractAgentID(tc.in); got != tc.want {
			t.Errorf("ExtractAgentID(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestRegisterLookup(t *testing.T) {
	dir := t.TempDir()
	child := "9e24ef36-3521-462c-b43e-e552bbf0f807"
	parent := "f0c187a2-93b7-4d06-ac3b-7ca9f6539b1a"
	if err := Register(dir, child, parent, "cursor", "test"); err != nil {
		t.Fatal(err)
	}
	// call- ids should be ignored
	if err := Register(dir, "call-abc-tool", parent, "cursor", "test"); err != nil {
		t.Fatal(err)
	}
	p, ok := Lookup(dir, child)
	if !ok || p != parent {
		t.Fatalf("Lookup: got %q %v", p, ok)
	}
	if _, ok := Lookup(dir, "call-abc-tool"); ok {
		t.Fatal("call- ids must not be stored")
	}
}

func TestIsChildSessionID(t *testing.T) {
	if !IsChildSessionID("9e24ef36-3521-462c-b43e-e552bbf0f807") {
		t.Fatal("uuid")
	}
	if IsChildSessionID("call-6d1f0541-71e8-4768-b2f0-95cf9d2770bd-50") {
		t.Fatal("tool call id")
	}
	if !IsChildSessionID("agent-xyz") {
		t.Fatal("agent- prefix")
	}
}

func TestParentFromCursorTranscriptPath(t *testing.T) {
	path := "/Users/me/.cursor/projects/repo/agent-transcripts/f0c187a2-93b7-4d06-ac3b-7ca9f6539b1a/subagents/9e24ef36-3521-462c-b43e-e552bbf0f807.jsonl"
	parent, child := ParentFromCursorTranscriptPath(path)
	if parent != "f0c187a2-93b7-4d06-ac3b-7ca9f6539b1a" {
		t.Errorf("parent=%q", parent)
	}
	if child != "9e24ef36-3521-462c-b43e-e552bbf0f807" {
		t.Errorf("child=%q", child)
	}
}

func TestPendingClaim(t *testing.T) {
	dir := t.TempDir()
	parent := "aaaaaaaa-1111-2222-3333-444444444444"
	child := "bbbbbbbb-1111-2222-3333-444444444444"
	if err := NotePending(dir, parent, "cursor", "tc-1"); err != nil {
		t.Fatal(err)
	}
	got := ClaimPending(dir, child, "cursor")
	if got != parent {
		t.Fatalf("ClaimPending=%q want %q", got, parent)
	}
	p, ok := Lookup(dir, child)
	if !ok || p != parent {
		t.Fatalf("Lookup after claim: %q %v", p, ok)
	}
	// second claim should not steal
	other := "cccccccc-1111-2222-3333-444444444444"
	if ClaimPending(dir, other, "cursor") != "" {
		t.Fatal("expected no pending left")
	}
}

func TestSessionsDirRequiresCWD(t *testing.T) {
	if SessionsDir("") != "" {
		t.Fatal("empty cwd must not resolve sessions dir")
	}
}

func TestUnregister(t *testing.T) {
	dir := t.TempDir()
	child := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	parent := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	if err := Register(dir, child, parent, "cursor", "test"); err != nil {
		t.Fatal(err)
	}
	if err := Unregister(dir, child); err != nil {
		t.Fatal(err)
	}
	if _, ok := Lookup(dir, child); ok {
		t.Fatal("expected link removed")
	}
}

func TestClaimPendingSkipsTopLevelCursorChat(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	parent := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	child := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	// child looks like a parent chat (own transcripts folder)
	top := filepath.Join(home, ".cursor", "projects", "proj", "agent-transcripts", child)
	if err := os.MkdirAll(top, 0o755); err != nil {
		t.Fatal(err)
	}
	sessions := t.TempDir()
	if err := NotePending(sessions, parent, "cursor", "tc-1"); err != nil {
		t.Fatal(err)
	}
	if got := ClaimPending(sessions, child, "cursor"); got != "" {
		t.Fatalf("ClaimPending=%q want empty for top-level Cursor chat", got)
	}
}

func TestDiscoverCursorParent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// On darwin UserHomeDir uses home; ensure layout exists
	parent := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	child := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	dir := filepath.Join(home, ".cursor", "projects", "proj", "agent-transcripts", parent, "subagents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, child+".jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := DiscoverCursorParent(child); got != parent {
		t.Fatalf("DiscoverCursorParent=%q want %q", got, parent)
	}
}
