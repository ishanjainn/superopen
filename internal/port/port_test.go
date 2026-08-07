package port_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/port"
	"github.com/ishanjainn/superopen/internal/port/adapters"
)

func TestClaudeRoundTripViaSOHub(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cwd := filepath.Join(home, "proj")
	_ = os.MkdirAll(cwd, 0o755)

	encoded := strings.ReplaceAll(cwd, string(os.PathSeparator), "-")
	projDir := filepath.Join(home, ".claude", "projects", encoded)
	_ = os.MkdirAll(projDir, 0o755)
	sid := "sess-test-1"
	rows := []map[string]any{
		{
			"type": "user", "cwd": cwd, "sessionId": sid, "timestamp": "2026-01-01T00:00:00Z",
			"message": map[string]any{"role": "user", "content": "hello from claude"},
			"uuid":    "u1",
		},
		{
			"type": "assistant", "cwd": cwd, "sessionId": sid, "timestamp": "2026-01-01T00:00:01Z",
			"message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": "hi there"}}},
			"uuid":    "u2", "parentUuid": "u1",
		},
	}
	f, err := os.Create(filepath.Join(projDir, sid+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	enc := json.NewEncoder(f)
	for _, r := range rows {
		_ = enc.Encode(r)
	}
	_ = f.Close()

	hubRoot := t.TempDir()
	_ = os.MkdirAll(filepath.Join(hubRoot, ".so"), 0o755)
	reg := port.NewRegistry()
	adapters.RegisterAll(reg, hubRoot)
	o := &port.Orchestrator{
		Reg:      reg,
		Ledger:   port.NewLedger(port.DefaultLedgerPath(filepath.Join(hubRoot, ".so"))),
		RepoRoot: hubRoot,
	}
	res, err := o.Port(port.PortOptions{From: port.HarnessClaude, To: port.HarnessSOHub, All: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Ported != 1 {
		t.Fatalf("ported=%d events=%v", res.Ported, res.Events)
	}

	// Export back to Claude
	res2, err := o.Port(port.PortOptions{From: port.HarnessSOHub, To: port.HarnessClaude, All: true, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Ported < 1 {
		t.Fatalf("re-export ported=%d", res2.Ported)
	}
}

func TestLedgerIdempotency(t *testing.T) {
	dir := t.TempDir()
	l := port.NewLedger(filepath.Join(dir, "ledger.json"))
	e := port.LedgerEntry{
		SourceHarness: port.HarnessClaude, SourceSessionID: "a",
		DestHarness: port.HarnessCodex, DestSessionID: "b",
	}
	if err := l.Upsert(e); err != nil {
		t.Fatal(err)
	}
	got, ok := l.Lookup(port.HarnessClaude, "a", port.HarnessCodex)
	if !ok || got.DestSessionID != "b" {
		t.Fatalf("lookup failed: %+v", got)
	}
}

func TestRemapCWD(t *testing.T) {
	sess := port.NewPortableSession(port.HarnessClaude, "1", "/x", "/old/worktree", "t")
	port.RemapCWD(&sess, "/new/worktree")
	if sess.CWD != "/new/worktree" {
		t.Fatalf("cwd=%s", sess.CWD)
	}
	if sess.SourceMetadata["original_cwd"] != "/old/worktree" {
		t.Fatalf("metadata=%v", sess.SourceMetadata)
	}
}

func TestCodexParseExport(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	sessDir := filepath.Join(home, ".codex", "sessions", "2026", "01", "01")
	_ = os.MkdirAll(sessDir, 0o755)
	path := filepath.Join(sessDir, "rollout-test.jsonl")
	rows := []map[string]any{
		{"type": "session_meta", "payload": map[string]any{"id": "codex-1", "cwd": filepath.Join(home, "app"), "timestamp": "2026-01-01T00:00:00Z"}},
		{"type": "event_msg", "payload": map[string]any{"type": "user_message", "message": "ship it", "timestamp": "2026-01-01T00:00:01Z"}},
		{"type": "event_msg", "payload": map[string]any{"type": "agent_message", "message": "shipping", "timestamp": "2026-01-01T00:00:02Z"}},
	}
	f, _ := os.Create(path)
	enc := json.NewEncoder(f)
	for _, r := range rows {
		_ = enc.Encode(r)
	}
	_ = f.Close()

	imp := adapters.CodexImport{}
	refs, err := imp.Discover()
	if err != nil || len(refs) == 0 {
		t.Fatalf("discover: %v %v", err, refs)
	}
	ps, err := imp.Parse(refs[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(ps.Turns) < 1 {
		t.Fatalf("turns=%d dropped=%d", len(ps.Turns), ps.DroppedTurns)
	}
	exp := adapters.CodexExport{}
	out, err := exp.Write(ps, port.WriteOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.DestSessionID == "" {
		t.Fatal("empty dest id")
	}
}

func TestCodexParseRecoversWorkingState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	sessDir := filepath.Join(home, ".codex", "sessions", "2026", "01", "02")
	_ = os.MkdirAll(sessDir, 0o755)
	path := filepath.Join(sessDir, "rollout-ws.jsonl")
	appCwd := filepath.Join(home, "app")
	rows := []map[string]any{
		{"type": "session_meta", "payload": map[string]any{"id": "codex-ws", "cwd": appCwd, "timestamp": "2026-01-01T00:00:00Z"}},
		{"type": "event_msg", "payload": map[string]any{"type": "user_message", "message": "fix main.go", "timestamp": "2026-01-01T00:00:01Z"}},
		{"type": "response_item", "payload": map[string]any{
			"type": "function_call", "name": "shell", "call_id": "c1",
			"arguments": `{"command":"go test ./..."}`, "timestamp": "2026-01-01T00:00:02Z",
		}},
		{"type": "response_item", "payload": map[string]any{
			"type": "function_call_output", "call_id": "c1",
			"output": `{"exit_code":1}`, "timestamp": "2026-01-01T00:00:03Z",
		}},
		{"type": "response_item", "payload": map[string]any{
			"type": "function_call", "name": "edit_file", "call_id": "c2",
			"arguments": map[string]any{"file_path": filepath.Join(appCwd, "main.go")},
			"timestamp": "2026-01-01T00:00:04Z",
		}},
		{"type": "event_msg", "payload": map[string]any{"type": "agent_message", "message": "fixed it", "timestamp": "2026-01-01T00:00:05Z"}},
	}
	f, _ := os.Create(path)
	enc := json.NewEncoder(f)
	for _, r := range rows {
		_ = enc.Encode(r)
	}
	_ = f.Close()

	imp := adapters.CodexImport{}
	refs, err := imp.Discover()
	if err != nil || len(refs) == 0 {
		t.Fatalf("discover: %v %v", err, refs)
	}
	ps, err := imp.Parse(refs[0])
	if err != nil {
		t.Fatal(err)
	}
	if ps.DroppedTurns == 0 {
		t.Fatal("expected tool call rows to count as dropped turns")
	}
	if len(ps.WorkingState.Commands) != 1 || ps.WorkingState.Commands[0].Cmd != "go test ./..." {
		t.Fatalf("commands=%+v", ps.WorkingState.Commands)
	}
	if got := ps.WorkingState.Commands[0].ExitCode; got == nil || *got != 1 {
		t.Fatalf("expected exit code 1, got %v", got)
	}
	if len(ps.WorkingState.FilesEdited) != 1 || ps.WorkingState.FilesEdited[0] != "main.go" {
		t.Fatalf("files_edited=%v", ps.WorkingState.FilesEdited)
	}
}

func TestCodexParseRepeatedCommandKeepsLatestExitCode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	sessDir := filepath.Join(home, ".codex", "sessions", "2026", "01", "03")
	_ = os.MkdirAll(sessDir, 0o755)
	path := filepath.Join(sessDir, "rollout-repeat.jsonl")
	appCwd := filepath.Join(home, "app")
	rows := []map[string]any{
		{"type": "session_meta", "payload": map[string]any{"id": "codex-repeat", "cwd": appCwd, "timestamp": "2026-01-01T00:00:00Z"}},
		// Fails first...
		{"type": "response_item", "payload": map[string]any{
			"type": "function_call", "name": "shell", "call_id": "c1",
			"arguments": `{"command":"go test ./..."}`, "timestamp": "2026-01-01T00:00:01Z",
		}},
		{"type": "response_item", "payload": map[string]any{
			"type": "function_call_output", "call_id": "c1",
			"output": `{"exit_code":1}`, "timestamp": "2026-01-01T00:00:02Z",
		}},
		// ...then the same command is re-run and passes after a fix.
		{"type": "response_item", "payload": map[string]any{
			"type": "function_call", "name": "shell", "call_id": "c2",
			"arguments": `{"command":"go test ./..."}`, "timestamp": "2026-01-01T00:00:03Z",
		}},
		{"type": "response_item", "payload": map[string]any{
			"type": "function_call_output", "call_id": "c2",
			"output": `{"exit_code":0}`, "timestamp": "2026-01-01T00:00:04Z",
		}},
		{"type": "event_msg", "payload": map[string]any{"type": "agent_message", "message": "fixed", "timestamp": "2026-01-01T00:00:05Z"}},
	}
	f, _ := os.Create(path)
	enc := json.NewEncoder(f)
	for _, r := range rows {
		_ = enc.Encode(r)
	}
	_ = f.Close()

	imp := adapters.CodexImport{}
	refs, err := imp.Discover()
	if err != nil || len(refs) == 0 {
		t.Fatalf("discover: %v %v", err, refs)
	}
	ps, err := imp.Parse(refs[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(ps.WorkingState.Commands) != 1 {
		t.Fatalf("expected the repeated command to be deduped into one entry, got %+v", ps.WorkingState.Commands)
	}
	got := ps.WorkingState.Commands[0].ExitCode
	if got == nil || *got != 0 {
		t.Fatalf("expected the LATEST run's exit code (0) to win over the first (1), got %v", got)
	}
}

func TestCodexParseInterleavedOutputsAttachExitByCallID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	sessDir := filepath.Join(home, ".codex", "sessions", "2026", "01", "04")
	_ = os.MkdirAll(sessDir, 0o755)
	path := filepath.Join(sessDir, "rollout-interleaved.jsonl")
	appCwd := filepath.Join(home, "app")
	// Parallel-style ordering: both calls observed before either output.
	// lastCommandIdx would stamp both exits onto "npm run build".
	rows := []map[string]any{
		{"type": "session_meta", "payload": map[string]any{"id": "codex-interleave", "cwd": appCwd, "timestamp": "2026-01-01T00:00:00Z"}},
		{"type": "response_item", "payload": map[string]any{
			"type": "function_call", "name": "shell", "call_id": "c1",
			"arguments": `{"command":"go test ./..."}`, "timestamp": "2026-01-01T00:00:01Z",
		}},
		{"type": "response_item", "payload": map[string]any{
			"type": "function_call", "name": "shell", "call_id": "c2",
			"arguments": `{"command":"npm run build"}`, "timestamp": "2026-01-01T00:00:02Z",
		}},
		{"type": "response_item", "payload": map[string]any{
			"type": "function_call_output", "call_id": "c1",
			"output": `{"exit_code":1}`, "timestamp": "2026-01-01T00:00:03Z",
		}},
		{"type": "response_item", "payload": map[string]any{
			"type": "function_call_output", "call_id": "c2",
			"output": `{"exit_code":0}`, "timestamp": "2026-01-01T00:00:04Z",
		}},
		{"type": "event_msg", "payload": map[string]any{"type": "agent_message", "message": "done", "timestamp": "2026-01-01T00:00:05Z"}},
	}
	f, _ := os.Create(path)
	enc := json.NewEncoder(f)
	for _, r := range rows {
		_ = enc.Encode(r)
	}
	_ = f.Close()

	imp := adapters.CodexImport{}
	refs, err := imp.Discover()
	if err != nil || len(refs) == 0 {
		t.Fatalf("discover: %v %v", err, refs)
	}
	ps, err := imp.Parse(refs[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(ps.WorkingState.Commands) != 2 {
		t.Fatalf("commands=%+v", ps.WorkingState.Commands)
	}
	byCmd := map[string]*int{}
	for _, c := range ps.WorkingState.Commands {
		byCmd[c.Cmd] = c.ExitCode
	}
	if got := byCmd["go test ./..."]; got == nil || *got != 1 {
		t.Fatalf("go test exit want 1, got %v", got)
	}
	if got := byCmd["npm run build"]; got == nil || *got != 0 {
		t.Fatalf("npm build exit want 0, got %v", got)
	}
}

func TestClaudeParseRecoversWorkingStateFromToolUse(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cwd := filepath.Join(home, "proj")
	_ = os.MkdirAll(cwd, 0o755)
	encoded := strings.ReplaceAll(cwd, string(os.PathSeparator), "-")
	projDir := filepath.Join(home, ".claude", "projects", encoded)
	_ = os.MkdirAll(projDir, 0o755)
	sid := "sess-ws-1"
	rows := []map[string]any{
		{
			"type": "user", "cwd": cwd, "sessionId": sid, "timestamp": "2026-01-01T00:00:00Z",
			"message": map[string]any{"role": "user", "content": "add a test"},
			"uuid":    "u1",
		},
		{
			"type": "assistant", "cwd": cwd, "sessionId": sid, "gitBranch": "feature/x", "timestamp": "2026-01-01T00:00:01Z",
			"message": map[string]any{"role": "assistant", "content": []any{
				map[string]any{"type": "tool_use", "name": "Read", "input": map[string]any{"file_path": filepath.Join(cwd, "main.go")}},
			}},
			"uuid": "u2", "parentUuid": "u1",
		},
		{
			"type": "assistant", "cwd": cwd, "sessionId": sid, "timestamp": "2026-01-01T00:00:02Z",
			"message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": "done"}}},
			"uuid":    "u3", "parentUuid": "u2",
		},
	}
	f, err := os.Create(filepath.Join(projDir, sid+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	enc := json.NewEncoder(f)
	for _, r := range rows {
		_ = enc.Encode(r)
	}
	_ = f.Close()

	imp := adapters.ClaudeImport{}
	refs, err := imp.Discover()
	if err != nil || len(refs) == 0 {
		t.Fatalf("discover: %v %v", err, refs)
	}
	var ref port.SessionRef
	for _, r := range refs {
		if r.SourceSessionID == sid {
			ref = r
		}
	}
	ps, err := imp.Parse(ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps.WorkingState.FilesRead) != 1 || ps.WorkingState.FilesRead[0] != "main.go" {
		t.Fatalf("files_read=%v", ps.WorkingState.FilesRead)
	}
	if ps.WorkingState.GitBranch != "feature/x" {
		t.Fatalf("git_branch=%q", ps.WorkingState.GitBranch)
	}
}

func TestSOHubRoundTripPreservesWorkingState(t *testing.T) {
	hubRoot := t.TempDir()
	_ = os.MkdirAll(filepath.Join(hubRoot, ".so"), 0o755)
	ps := port.NewPortableSession(port.HarnessCodex, "src-ws", "/x", hubRoot, "t")
	ps.Turns = []port.PortableTurn{{Role: "user", Text: "hi"}}
	ps.DroppedTurns = 2
	ps.WorkingState = port.WorkingState{FilesEdited: []string{"a.go"}, GitBranch: "main"}

	exp := adapters.SOHubExport{RepoRoot: hubRoot}
	out, err := exp.Write(ps, port.WriteOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}

	imp := adapters.SOHubImport{RepoRoot: hubRoot}
	refs, err := imp.Discover()
	if err != nil {
		t.Fatal(err)
	}
	var ref port.SessionRef
	for _, r := range refs {
		if r.SourceSessionID == out.DestSessionID {
			ref = r
		}
	}
	if ref.SourceSessionID == "" {
		t.Fatalf("could not find ported session %s among %v", out.DestSessionID, refs)
	}
	back, err := imp.Parse(ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.WorkingState.FilesEdited) != 1 || back.WorkingState.FilesEdited[0] != "a.go" {
		t.Fatalf("working state not preserved through hub round-trip: %+v", back.WorkingState)
	}
	if back.WorkingState.GitBranch != "main" {
		t.Fatalf("git branch not preserved: %+v", back.WorkingState)
	}
	if back.DroppedTurns != 2 {
		t.Fatalf("dropped turns not preserved: %d", back.DroppedTurns)
	}
}

func TestCursorExportPreservesWorkingStateOnHubMirror(t *testing.T) {
	hubRoot := t.TempDir()
	_ = os.MkdirAll(filepath.Join(hubRoot, ".so"), 0o755)
	ps := port.NewPortableSession(port.HarnessCodex, "src-cursor-ws", "/x", hubRoot, "t")
	ps.Turns = []port.PortableTurn{{Role: "user", Text: "hi"}}
	ps.DroppedTurns = 3
	ps.WorkingState = port.WorkingState{
		FilesEdited: []string{"b.go"},
		GitBranch:   "feature/ws",
		Commands:    []port.RanCommand{{Cmd: "go test ./..."}},
	}

	exp := adapters.CursorExport{RepoRoot: hubRoot}
	out, err := exp.Write(ps, port.WriteOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}

	// Hub mirror under .so/sessions must carry the sidecar so a later
	// CursorImport / SOHubImport can restore recovered files/commands.
	imp := adapters.CursorImport{SORoot: filepath.Join(hubRoot, ".so")}
	refs, err := imp.Discover()
	if err != nil {
		t.Fatal(err)
	}
	var ref port.SessionRef
	for _, r := range refs {
		if r.SourceSessionID == out.DestSessionID {
			ref = r
		}
	}
	if ref.SourceSessionID == "" {
		t.Fatalf("could not find cursor hub mirror %s among %v", out.DestSessionID, refs)
	}
	back, err := imp.Parse(ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.WorkingState.FilesEdited) != 1 || back.WorkingState.FilesEdited[0] != "b.go" {
		t.Fatalf("hub mirror lost working state: %+v", back.WorkingState)
	}
	if back.WorkingState.GitBranch != "feature/ws" {
		t.Fatalf("git branch not preserved: %+v", back.WorkingState)
	}
	if back.DroppedTurns != 3 {
		t.Fatalf("dropped turns not preserved: %d", back.DroppedTurns)
	}

	// Native so-port pack must also keep the sidecar for re-import from there.
	portDir := filepath.Join(hubRoot, ".cursor", "so-port", out.DestSessionID)
	if _, err := os.Stat(filepath.Join(portDir, "working-state.json")); err != nil {
		t.Fatalf("so-port pack missing working-state.json: %v", err)
	}
}
