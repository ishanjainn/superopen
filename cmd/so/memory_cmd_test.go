package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/paths"
)

func TestMemorySearchEmptyAXI(t *testing.T) {
	root := t.TempDir()
	if err := paths.Resolve(root).EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	cliFlags.Root = root
	t.Cleanup(func() { cliFlags.Root = ""; cliFlags.JSON = false })

	stdout := captureStdout(t, func() {
		cmd := newRootCommand()
		cmd.SetArgs([]string{"--root", root, "memory", "search", "nothing-here"})
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(stdout, "0 memories") {
		t.Fatalf("empty state missing: %q", stdout)
	}
	if !strings.Contains(stdout, "help[") {
		t.Fatalf("AXI help[] missing: %q", stdout)
	}

	cliFlags.JSON = true
	stdout = captureStdout(t, func() {
		cmd := newRootCommand()
		cmd.SetArgs([]string{"--root", root, "--json", "memory", "search", "nothing-here"})
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	var env map[string]any
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("json envelope: %v %q", err, stdout)
	}
	if env["ok"] != true || env["kind"] != "memories" {
		t.Fatalf("envelope: %+v", env)
	}
}

func TestMemorySearchCompactNoBody(t *testing.T) {
	root := t.TempDir()
	if err := paths.Resolve(root).EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	store, err := memory.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	body := "SECRET_CLI_BODY_xyzzy"
	if _, err := store.Capture(memory.CaptureInput{Kind: memory.KindSession, Title: "JWT expiry is 15m", Text: body, Topic: memory.ObservationDecision}); err != nil {
		t.Fatal(err)
	}
	store.Close()
	cliFlags.Root = root
	cliFlags.JSON = false
	t.Cleanup(func() { cliFlags.Root = "" })

	var cobraOut bytes.Buffer
	stdout := captureStdout(t, func() {
		cmd := newRootCommand()
		cmd.SetOut(&cobraOut)
		cmd.SetErr(io.Discard)
		cmd.SetArgs([]string{"--root", root, "memory", "search", "JWT expiry"})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	combined := stdout + cobraOut.String()
	if !strings.Contains(combined, "MEM #") {
		t.Fatalf("compact MEM line missing: %q", combined)
	}
	if strings.Contains(combined, body) {
		t.Fatalf("search leaked body: %q", combined)
	}
	if !strings.Contains(combined, "help[") {
		t.Fatalf("help[] missing: %q", combined)
	}
}

func TestMemoryGetBatch(t *testing.T) {
	root := t.TempDir()
	if err := paths.Resolve(root).EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	store, err := memory.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	a, err := store.Capture(memory.CaptureInput{Kind: memory.KindSession, Title: "one", Text: "body-one"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.Capture(memory.CaptureInput{Kind: memory.KindSession, Title: "two", Text: "body-two"})
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	cliFlags.Root = root
	t.Cleanup(func() { cliFlags.Root = "" })
	var cobraOut bytes.Buffer
	cmd := newRootCommand()
	cmd.SetOut(&cobraOut)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--root", root, "memory", "get", strconv.FormatInt(a.ID, 10), strconv.FormatInt(b.ID, 10)})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := cobraOut.String()
	if !strings.Contains(got, "body-one") || !strings.Contains(got, "body-two") {
		t.Fatalf("batch get missing bodies: %q", got)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	raw, _ := io.ReadAll(r)
	_ = r.Close()
	return string(raw)
}
