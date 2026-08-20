package cli

import (
	"strings"
	"testing"
)

func TestParseHookPayloadUsesWorkspaceRoots(t *testing.T) {
	got := ParseHookPayload([]byte(`{
		"cwd": "/tmp/tool-dir",
		"workspace_roots": ["/Users/me/grafana-gpuo11y-app"],
		"conversation_id": "chat-1",
		"session_id": "proc-9"
	}`))
	if got.Workspace != "/Users/me/grafana-gpuo11y-app" {
		t.Fatalf("Workspace=%q, want workspace_roots (repo), not tool cwd", got.Workspace)
	}
	if got.SessionID != "chat-1" {
		t.Fatalf("SessionID=%q, want conversation_id", got.SessionID)
	}

	got = ParseHookPayload([]byte(`{
		"workspace_roots": ["/Users/me/grafana-gpuo11y-app"],
		"conversation_id": "19e680b2-13df-48b4-9e8a-cdf40a6689cf"
	}`))
	if got.Workspace != "/Users/me/grafana-gpuo11y-app" {
		t.Fatalf("Workspace=%q, want workspace_roots[0]", got.Workspace)
	}
}

func TestReadHookStdinIgnoresEmpty(t *testing.T) {
	if got := ReadHookStdin(strings.NewReader("")); got.Workspace != "" {
		t.Fatalf("empty stdin: %+v", got)
	}
}
