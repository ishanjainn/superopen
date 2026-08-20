package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
)

// HookLaunch is the workspace + chat id Cursor/Claude put on hook stdin.
// Cursor runs user hooks with cwd ~/.cursor, so process cwd is not the repo.
type HookLaunch struct {
	Workspace string
	SessionID string
}

// ParseHookPayload reads workspace_roots / cwd / conversation_id from a hook JSON blob.
func ParseHookPayload(raw []byte) HookLaunch {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return HookLaunch{}
	}
	var m struct {
		CWD            string   `json:"cwd"`
		WorkspaceRoots []string `json:"workspace_roots"`
		ConversationID string   `json:"conversation_id"`
		SessionID      string   `json:"session_id"`
	}
	if json.Unmarshal(raw, &m) != nil {
		return HookLaunch{}
	}
	ws := ""
	if len(m.WorkspaceRoots) > 0 {
		ws = strings.TrimSpace(m.WorkspaceRoots[0])
	}
	if ws == "" {
		ws = strings.TrimSpace(m.CWD)
	}
	sid := strings.TrimSpace(m.ConversationID)
	if sid == "" {
		sid = strings.TrimSpace(m.SessionID)
	}
	return HookLaunch{Workspace: ws, SessionID: sid}
}

// ReadHookStdin parses hook JSON from a pipe. TTY stdin is ignored so
// `so sessions finalize --detach` from a shell keeps using the process cwd.
func ReadHookStdin(r io.Reader) HookLaunch {
	if r == nil {
		return HookLaunch{}
	}
	if f, ok := r.(*os.File); ok {
		st, err := f.Stat()
		if err != nil || st.Mode()&os.ModeCharDevice != 0 {
			return HookLaunch{}
		}
	}
	raw, err := io.ReadAll(io.LimitReader(r, 8<<20))
	if err != nil {
		return HookLaunch{}
	}
	return ParseHookPayload(raw)
}
