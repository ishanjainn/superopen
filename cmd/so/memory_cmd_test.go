package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/cli"
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
	if !strings.Contains(combined, "memories[") || !strings.Contains(combined, "{id,kind,title,tokens}:") {
		t.Fatalf("TOON search missing: %q", combined)
	}
	if strings.Contains(combined, "MEM #") {
		t.Fatalf("default search should not print MEM lines: %q", combined)
	}
	if strings.Contains(combined, body) {
		t.Fatalf("search leaked body: %q", combined)
	}
	if !strings.Contains(combined, "help[") {
		t.Fatalf("help[] missing: %q", combined)
	}
}

func TestMemoryHomeDashboard(t *testing.T) {
	root := t.TempDir()
	if err := paths.Resolve(root).EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	store, err := memory.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Capture(memory.CaptureInput{
		Kind: memory.KindSession, Horizon: memory.HorizonMedium, Title: "login timeout is 30s", Text: "keep it in sqlite",
	}); err != nil {
		t.Fatal(err)
	}
	store.Close()
	cliFlags.Root = root
	cliFlags.JSON = false
	t.Cleanup(func() { cliFlags.Root = ""; cliFlags.JSON = false })

	stdout := captureStdout(t, func() {
		cmd := newRootCommand()
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs([]string{"--root", root, "memory"})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	for _, want := range []string{"bin: ", "description: ", "long: ", "diary: ", "pending distill: ", "help[", `so memory search "<cue>"`} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("dashboard missing %q in %q", want, stdout)
		}
	}
}

func TestMemoryUnknownCommand(t *testing.T) {
	root := t.TempDir()
	if err := paths.Resolve(root).EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	cliFlags.Root = root
	t.Cleanup(func() { cliFlags.Root = "" })

	cmd := newRootCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--root", root, "memory", "bogus"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected unknown command")
	}
	if cli.ExitCode(err) != cli.ExitUsage {
		t.Fatalf("exit=%d want usage, err=%v", cli.ExitCode(err), err)
	}
	if !strings.Contains(err.Error(), `unknown command "bogus"`) {
		t.Fatalf("got %v", err)
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
	stdout := captureStdout(t, func() {
		cmd := newRootCommand()
		cmd.SetOut(&cobraOut)
		cmd.SetErr(io.Discard)
		cmd.SetArgs([]string{"--root", root, "memory", "get", strconv.FormatInt(a.ID, 10), strconv.FormatInt(b.ID, 10)})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	got := stdout + cobraOut.String()
	if !strings.Contains(got, "body-one") || !strings.Contains(got, "body-two") {
		t.Fatalf("batch get missing bodies: %q", got)
	}
}

func TestMemoryGetTruncates(t *testing.T) {
	root := t.TempDir()
	if err := paths.Resolve(root).EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	store, err := memory.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Repeat("keep-login-timeout ", 80)
	ep, err := store.Capture(memory.CaptureInput{Kind: memory.KindSession, Title: "long body", Text: body})
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	cliFlags.Root = root
	cliFlags.JSON = false
	t.Cleanup(func() { cliFlags.Root = ""; cliFlags.JSON = false })

	stdout := captureStdout(t, func() {
		cmd := newRootCommand()
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs([]string{"--root", root, "memory", "get", strconv.FormatInt(ep.ID, 10)})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(stdout, "truncated") || !strings.Contains(stdout, "use --full") {
		t.Fatalf("truncated get missing hint: %q", stdout)
	}
}

func TestMemoryCaptureRequiresFields(t *testing.T) {
	root := t.TempDir()
	if err := paths.Resolve(root).EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	cliFlags.Root = root
	t.Cleanup(func() { cliFlags.Root = "" })

	cmd := newRootCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--root", root, "memory", "capture", "--title", "only title"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("capture without kind/horizon/text must fail")
	}

	ok := newRootCommand()
	ok.SetOut(io.Discard)
	ok.SetErr(io.Discard)
	ok.SetArgs([]string{
		"--root", root, "memory", "capture",
		"--kind", "knowledge", "--horizon", "medium",
		"--title", "login timeout is 30s", "--text", "keep login timeout in sqlite",
	})
	if err := ok.Execute(); err != nil {
		t.Fatal(err)
	}
	store, err := memory.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hits, err := store.Search(memory.SearchFilter{Query: "login timeout", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || hits[0].Horizon != memory.HorizonMedium {
		t.Fatalf("capture did not store medium knowledge: %+v", hits)
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

func TestMemoryForgetHidesAXI(t *testing.T) {
	root := t.TempDir()
	if err := paths.Resolve(root).EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	store, err := memory.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	ep, err := store.Capture(memory.CaptureInput{
		Kind: memory.KindSession, Title: "Session auth stays in SQLite", Text: "sqlite is the source of truth",
	})
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	cliFlags.Root = root
	cliFlags.JSON = false
	t.Cleanup(func() { cliFlags.Root = ""; cliFlags.JSON = false })

	var cobraOut bytes.Buffer
	stdout := captureStdout(t, func() {
		cmd := newRootCommand()
		cmd.SetOut(&cobraOut)
		cmd.SetErr(io.Discard)
		cmd.SetArgs([]string{"--root", root, "memory", "forget", strconv.FormatInt(ep.ID, 10)})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	combined := stdout + cobraOut.String()
	if !strings.Contains(combined, "forget #") {
		t.Fatalf("forget line missing: %q", combined)
	}
	if !strings.Contains(combined, "help[") {
		t.Fatalf("AXI help[] missing: %q", combined)
	}

	store, err = memory.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	got, err := store.Get(ep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Faded {
		t.Fatal("forget should fade the row")
	}

	cmd := newRootCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--root", root, "memory", "forget", "999999"})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected not found")
	}
	if cli.ExitCode(err) != cli.ExitNotFound {
		t.Fatalf("exit=%d want not-found, err=%v", cli.ExitCode(err), err)
	}
}
