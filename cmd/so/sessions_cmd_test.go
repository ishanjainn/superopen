package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/cli"
	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/session"
)

func TestSessionsListTOON(t *testing.T) {
	root := t.TempDir()
	layout := paths.Resolve(root)
	if err := layout.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	store := session.NewStore(layout)
	if err := store.Start(session.Meta{
		ID: "abc", Vendor: "cursor", Status: session.StatusActive,
		Title: "login timeout", PromptPreview: "login timeout",
		StartedAt: time.Now().UTC(), Tokens: 44,
	}); err != nil {
		t.Fatal(err)
	}
	cliFlags.Root = root
	cliFlags.JSON = false
	t.Cleanup(func() { cliFlags.Root = ""; cliFlags.JSON = false })

	stdout := captureStdout(t, func() {
		cmd := newRootCommand()
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs([]string{"--root", root, "sessions"})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(stdout, "sessions[") || !strings.Contains(stdout, "{id,status,title,vendor}:") {
		t.Fatalf("TOON list missing: %q", stdout)
	}
	if strings.Contains(stdout, "tokens") && strings.Contains(stdout, "{id,status,title,vendor,") {
		t.Fatalf("default list should drop tokens: %q", stdout)
	}
	if !strings.Contains(stdout, "so sessions show <id>") || !strings.Contains(stdout, "help[") {
		t.Fatalf("help[] missing: %q", stdout)
	}
}

func TestSessionsEmpty(t *testing.T) {
	root := t.TempDir()
	if err := paths.Resolve(root).EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	cliFlags.Root = root
	cliFlags.JSON = false
	t.Cleanup(func() { cliFlags.Root = ""; cliFlags.JSON = false })

	stdout := captureStdout(t, func() {
		cmd := newRootCommand()
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs([]string{"--root", root, "sessions"})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(stdout, "0 sessions") {
		t.Fatalf("empty state missing: %q", stdout)
	}
	if !strings.Contains(stdout, "help[") {
		t.Fatalf("help[] missing: %q", stdout)
	}
}

func TestSessionsShowCompactVsFull(t *testing.T) {
	root := t.TempDir()
	layout := paths.Resolve(root)
	if err := layout.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	store := session.NewStore(layout)
	if err := store.Start(session.Meta{
		ID: "ses1", Vendor: "cursor", Status: session.StatusActive,
		Title: "login timeout", PromptPreview: "login timeout",
		StartedAt: time.Now().UTC(), Tokens: 12,
	}); err != nil {
		t.Fatal(err)
	}
	cliFlags.Root = root
	cliFlags.JSON = false
	t.Cleanup(func() { cliFlags.Root = ""; cliFlags.JSON = false })

	compact := captureStdout(t, func() {
		cmd := newRootCommand()
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs([]string{"--root", root, "sessions", "show", "ses1"})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(compact, "id: ses1") || !strings.Contains(compact, "vendor: cursor") {
		t.Fatalf("compact show missing fields: %q", compact)
	}
	if strings.Contains(compact, `"_about"`) {
		t.Fatalf("compact show dumped document: %q", compact)
	}

	full := captureStdout(t, func() {
		cmd := newRootCommand()
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs([]string{"--root", root, "--full", "sessions", "show", "ses1"})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(full, `"id": "ses1"`) && !strings.Contains(full, `"id":"ses1"`) {
		t.Fatalf("full show should dump document: %q", full)
	}

	cliFlags.JSON = true
	raw := captureStdout(t, func() {
		cmd := newRootCommand()
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs([]string{"--root", root, "--json", "sessions", "show", "ses1"})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	var env map[string]any
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("json envelope: %v %q", err, raw)
	}
	if env["ok"] != true || env["kind"] != "session" {
		t.Fatalf("envelope: %+v", env)
	}
}

func TestSessionsShowNestedTurnsFromEvents(t *testing.T) {
	root := t.TempDir()
	layout := paths.Resolve(root)
	if err := layout.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	store := session.NewStore(layout)
	child := "nested-show"
	if err := store.Start(session.Meta{
		ID: child, Vendor: "cursor", Status: session.StatusActive,
		ParentID: "parent-show", IsSubagent: true,
		StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	events := filepath.Join(layout.SessionDir(child), "events.jsonl")
	body := `{"name":"coding_agent.session.loop.stop","attributes":{"coding_agent.turn.id":"t1"}}` + "\n" +
		`{"name":"coding_agent.llm.turn","attributes":{"coding_agent.turn.id":"t1"}}` + "\n"
	if err := os.WriteFile(events, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cliFlags.Root = root
	cliFlags.JSON = false
	t.Cleanup(func() { cliFlags.Root = ""; cliFlags.JSON = false })

	compact := captureStdout(t, func() {
		cmd := newRootCommand()
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs([]string{"--root", root, "sessions", "show", child})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(compact, "turns: 1") {
		t.Fatalf("nested compact show missing turns: %q", compact)
	}
}

func TestSessionsTokensEmpty(t *testing.T) {
	root := t.TempDir()
	if err := paths.Resolve(root).EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	cliFlags.Root = root
	t.Cleanup(func() { cliFlags.Root = "" })

	stdout := captureStdout(t, func() {
		cmd := newRootCommand()
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs([]string{"--root", root, "sessions", "tokens"})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(stdout, "0 sessions") {
		t.Fatalf("tokens empty state missing: %q", stdout)
	}
}

func TestSessionsUnknownCommand(t *testing.T) {
	root := t.TempDir()
	if err := paths.Resolve(root).EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	cliFlags.Root = root
	t.Cleanup(func() { cliFlags.Root = "" })

	cmd := newRootCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--root", root, "sessions", "bogus"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected unknown command")
	}
	if cli.ExitCode(err) != cli.ExitUsage {
		t.Fatalf("exit=%d want usage, err=%v", cli.ExitCode(err), err)
	}
}

func TestSessionsHookHiddenFromAXICatalog(t *testing.T) {
	root := t.TempDir()
	if err := paths.Resolve(root).EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	cliFlags.Root = root
	cliFlags.JSON = false
	t.Cleanup(func() { cliFlags.Root = ""; cliFlags.JSON = false })

	var helpBuf bytes.Buffer
	helpCmd := newRootCommand()
	helpCmd.SetOut(&helpBuf)
	helpCmd.SetErr(io.Discard)
	helpCmd.SetArgs([]string{"--help"})
	if err := helpCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	help := helpBuf.String()
	for _, banned := range []string{"\n  coding ", "\n  open ", "\n  hook "} {
		if strings.Contains(help, banned) {
			t.Fatalf("root help still lists %q in %q", strings.TrimSpace(banned), help)
		}
	}

	var sessionsHelp bytes.Buffer
	sessionsCmd := newRootCommand()
	sessionsCmd.SetOut(&sessionsHelp)
	sessionsCmd.SetErr(io.Discard)
	sessionsCmd.SetArgs([]string{"sessions", "--help"})
	if err := sessionsCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := sessionsHelp.String()
	if strings.Contains(got, "\n  hook ") {
		t.Fatalf("so sessions --help must not list hidden hook (AXI catalog): %q", got)
	}

	cmd := newRootCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"coding"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected unknown command for so coding")
	}
}

func TestClaimSessionFinalizeSingleFlight(t *testing.T) {
	root := t.TempDir()
	if err := paths.Resolve(root).EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	unlock, ok := claimSessionFinalize(root, "sess-1")
	if !ok || unlock == nil {
		t.Fatal("first claim must succeed")
	}
	defer unlock()
	if _, ok := claimSessionFinalize(root, "sess-1"); ok {
		t.Fatal("second claim for the same session must skip")
	}
	unlock2, ok := claimSessionFinalize(root, "sess-2")
	if !ok {
		t.Fatal("a different session must not block")
	}
	unlock2()
}
