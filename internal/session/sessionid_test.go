package session

import "testing"

func TestResolveSessionIDPrefersConversation(t *testing.T) {
	attrs := map[string]string{
		"coding_agent.session.id": "ephemeral-session",
		"coding_agent.session_id": "sess-a",
		"gen_ai.conversation.id":  "stable-chat",
	}
	if got := ResolveSessionID(attrs, "trace"); got != "stable-chat" {
		t.Fatalf("got %q want stable-chat", got)
	}
	if got := ResolveSessionID(map[string]string{
		"coding_agent.session.id": "ses-only",
	}, "trace"); got != "ses-only" {
		t.Fatalf("session-only got %q", got)
	}
	if got := ResolveSessionID(map[string]string{}, "trace"); got != "trace" {
		t.Fatalf("fallback got %q", got)
	}
	if got := ResolveSessionID(map[string]string{
		"coding_agent.session.id": "unknown",
	}, "trace"); got != "trace" {
		t.Fatalf("placeholder session id must fall through, got %q", got)
	}
	if !IsPlaceholderSessionID("unknown") {
		t.Fatal("expected unknown placeholder")
	}
}

func TestResolveParentID(t *testing.T) {
	attrs := map[string]string{
		"coding_agent.agent.parent_id": "parent-1",
		"gen_ai.conversation.id":       "child-1",
	}
	if got := ResolveParentID(attrs); got != "parent-1" {
		t.Fatalf("got %q", got)
	}
	// Self-parent echo ignored
	attrs = map[string]string{
		"coding_agent.agent.parent_id": "same",
		"gen_ai.conversation.id":       "same",
	}
	if got := ResolveParentID(attrs); got != "" {
		t.Fatalf("self-parent got %q", got)
	}
	if !IsSubagentAttrs(map[string]string{"coding_agent.session.is_subagent": "true"}) {
		t.Fatal("expected is_subagent")
	}
	if !IsSubagentAttrs(map[string]string{"coding_agent.agent.parent_id": "p", "gen_ai.conversation.id": "c"}) {
		t.Fatal("expected parent implies subagent")
	}
}
