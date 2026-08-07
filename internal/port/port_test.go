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
