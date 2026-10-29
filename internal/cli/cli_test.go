package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestEmptyText(t *testing.T) {
	var buf bytes.Buffer
	o := &Out{W: &buf, ErrW: &buf}
	o.Next("so init")
	o.Empty("sessions")
	got := buf.String()
	if !strings.HasPrefix(got, "count: 0\nsessions: none\n") {
		t.Fatalf("an empty list states the zero and what is empty: %q", got)
	}
	if !strings.Contains(got, "help[1]:") {
		t.Fatalf("missing AXI help: %q", got)
	}
}

func TestRowsJSON(t *testing.T) {
	var buf bytes.Buffer
	o := &Out{Flags: Flags{JSON: true}, W: &buf, ErrW: &buf}
	o.Rows("sessions", []string{"id", "vendor"}, []map[string]any{
		{"id": "a", "vendor": "cursor"},
	})
	if !strings.Contains(buf.String(), `"count":1`) {
		t.Fatalf("got %s", buf.String())
	}
}

func TestRowsTextIncludesColumnNames(t *testing.T) {
	var buf bytes.Buffer
	o := &Out{W: &buf, ErrW: &buf}
	o.Rows("memories", []string{"id", "tokens"}, []map[string]any{
		{"id": 1, "tokens": 44},
	})
	got := buf.String()
	if !strings.Contains(got, "memories[1]{id,tokens}:") {
		t.Fatalf("TOON header missing: %q", got)
	}
	if !strings.Contains(got, "  1,44\n") {
		t.Fatalf("TOON row missing: %q", got)
	}
	if !strings.Contains(got, "count: 1 of 1") {
		t.Fatalf("count missing: %q", got)
	}
}

func TestRowsQuotesComma(t *testing.T) {
	var buf bytes.Buffer
	o := &Out{W: &buf, ErrW: &buf}
	o.Rows("sessions", []string{"id", "title"}, []map[string]any{
		{"id": "a", "title": "login, timeout"},
	})
	got := buf.String()
	if !strings.Contains(got, `"login, timeout"`) {
		t.Fatalf("comma field should be quoted: %q", got)
	}
}

func TestRowsBoundDefaultAndFullEscape(t *testing.T) {
	rows := make([]map[string]any, 25)
	for i := range rows {
		rows[i] = map[string]any{
			"id": i, "title": strings.Repeat("x", 100), "status": "open",
			"target": "pkg.method", "kind": "code", "source": "session",
		}
	}
	cols := []string{"id", "title", "status", "target", "kind", "source"}
	var buf bytes.Buffer
	(&Out{W: &buf, ErrW: &buf}).Rows("proposals", cols, rows)
	got := buf.String()
	for _, want := range []string{
		"count: 20 of 25", "proposals[20]{id,title,status,target}:",
		"shown: 20 of 25 rows, 4 of 6 columns (use --full for all)",
		"truncated, 100 chars — use --full",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("default rows missing %q: %s", want, got)
		}
	}
	buf.Reset()
	(&Out{Flags: Flags{Full: true}, W: &buf, ErrW: &buf}).Rows("proposals", cols, rows)
	if !strings.Contains(buf.String(), "count: 25 of 25") ||
		!strings.Contains(buf.String(), "proposals[25]{id,title,status,target,kind,source}:") ||
		strings.Contains(buf.String(), "truncated") {
		t.Fatalf("--full did not restore complete rows: %s", buf.String())
	}
}

func TestWriteErrorStdout(t *testing.T) {
	var out, errBuf bytes.Buffer
	o := &Out{W: &out, ErrW: &errBuf}
	o.WriteError(Fail(ExitNotFound, "memory 9 not found", `so memory search "<cue>"`))
	if errBuf.Len() != 0 {
		t.Fatalf("errors must not go to stderr: %q", errBuf.String())
	}
	got := out.String()
	if !strings.Contains(got, "error: memory 9 not found") || !strings.Contains(got, "help[1]:\n  so memory search") {
		t.Fatalf("structured stdout error missing: %q", got)
	}
}

func TestHome(t *testing.T) {
	var buf bytes.Buffer
	o := &Out{W: &buf, ErrW: &buf}
	o.Next("so memory search \"<cue>\"")
	o.Home("Project diary", []string{"long: 1"})
	got := buf.String()
	if !strings.HasPrefix(got, "bin: ") {
		t.Fatalf("bin missing: %q", got)
	}
	if !strings.Contains(got, "description: Project diary") || !strings.Contains(got, "long: 1") {
		t.Fatalf("home body missing: %q", got)
	}
	if !strings.Contains(got, "help[1]:") {
		t.Fatalf("help missing: %q", got)
	}
}

func TestQuietUnmanagedSkipsWithoutErrorPrefix(t *testing.T) {
	err := QuietUnmanaged(errors.New("not a Superopen repo; run so init"))
	if ExitCode(err) != ExitOK {
		t.Fatalf("exit=%d", ExitCode(err))
	}
	var buf bytes.Buffer
	(&Out{W: &buf}).WriteError(err)
	if buf.String() != "status: not a Superopen repo\n" {
		t.Fatalf("got %q", buf.String())
	}
}

func TestNormalizeExitUsage(t *testing.T) {
	err := NormalizeExit(errors.New(`unknown command "nope" for "so"`))
	if ExitCode(err) != ExitUsage {
		t.Fatalf("exit=%d", ExitCode(err))
	}
	err = NormalizeExit(errors.New(`invalid argument "nope" for "--sessions-hours" flag`))
	if ExitCode(err) != ExitUsage {
		t.Fatalf("flag exit=%d", ExitCode(err))
	}
	err = NormalizeExit(errors.New("disk full"))
	if ExitCode(err) != ExitFail {
		t.Fatalf("fail exit=%d", ExitCode(err))
	}
}

func TestUnknownCommand(t *testing.T) {
	err := UnknownCommand("memory", "bogus")
	if ExitCode(err) != ExitUsage {
		t.Fatalf("exit=%d", ExitCode(err))
	}
	if !strings.Contains(err.Error(), `unknown command "bogus"`) {
		t.Fatalf("got %v", err)
	}
}

func TestTruncateHint(t *testing.T) {
	o := &Out{}
	got := o.TruncateHint(strings.Repeat("x", 12), 4)
	if !strings.Contains(got, "truncated, 12 chars") {
		t.Fatalf("got %q", got)
	}
	o.Flags.Full = true
	if o.TruncateHint(strings.Repeat("x", 12), 4) != strings.Repeat("x", 12) {
		t.Fatal("full should not truncate")
	}
}

func TestTruncate(t *testing.T) {
	o := &Out{}
	s := o.Truncate(strings.Repeat("x", 100), 10)
	if !strings.HasSuffix(s, "…") || len([]rune(s)) != 11 {
		t.Fatalf("got %q", s)
	}
	o.Flags.Full = true
	if o.Truncate(strings.Repeat("x", 100), 10) != strings.Repeat("x", 100) {
		t.Fatal("full should not truncate")
	}
}
