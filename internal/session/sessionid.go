// Package otlp contains OpenTelemetry attribute normalization shared by
// session ingestion and vendor hooks. Superopen does not run a local receiver;
// coding hooks persist directly to repository-local session files.
package session

import "strings"

// ResolveSessionID picks the chat-thread id coding agents stamp on spans.
// Prefer gen_ai.conversation.id (stable for the life of a chat) over
// coding_agent.session.id (can be per-process / per-invocation on Cursor).
// Older paths used coding_agent.session_id.
func ResolveSessionID(attrs map[string]string, fallback string) string {
	for _, k := range []string{
		"gen_ai.conversation.id",
		"coding_agent.session.id",
		"coding_agent.session_id",
		"session.id",
		"session_id",
	} {
		if v := strings.TrimSpace(attrs[k]); v != "" && !IsPlaceholderSessionID(v) {
			return v
		}
	}
	if strings.TrimSpace(fallback) != "" && !IsPlaceholderSessionID(fallback) {
		return fallback
	}
	return ""
}

// IsPlaceholderSessionID reports ids that must never become Sessions UI rows.
func IsPlaceholderSessionID(id string) bool {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case "", "unknown", "null", "undefined", "nil", "none":
		return true
	default:
		return false
	}
}

// ResolveParentID returns the parent chat-thread id when this span belongs
// to a subagent / nested agent. Empty when missing or a self-parent echo.
func ResolveParentID(attrs map[string]string) string {
	if attrs == nil {
		return ""
	}
	parent := ""
	for _, k := range []string{
		"coding_agent.agent.parent_id",
		"coding_agent.parent_conversation.id",
		"gen_ai.conversation.parent_id",
	} {
		if v := attrs[k]; v != "" {
			parent = v
			break
		}
	}
	if parent == "" {
		return ""
	}
	self := ResolveSessionID(attrs, "")
	if parent == self {
		return ""
	}
	return parent
}

// IsSubagentAttrs reports whether span attributes mark a nested/subagent session.
func IsSubagentAttrs(attrs map[string]string) bool {
	if attrs == nil {
		return false
	}
	if v := attrs["coding_agent.session.is_subagent"]; v == "true" || v == "1" {
		return true
	}
	if v := attrs["coding_agent.agent.type"]; v == "subagent" {
		return true
	}
	return ResolveParentID(attrs) != ""
}
