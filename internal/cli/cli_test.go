package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestEmptyText(t *testing.T) {
	var buf bytes.Buffer
	o := &Out{W: &buf, ErrW: &buf}
	o.Next("so init")
	o.Empty("sessions")
	got := buf.String()
	if !strings.Contains(got, "0 sessions") {
		t.Fatalf("got %q", got)
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

func TestWriteErrorStdout(t *testing.T) {
	var out, errBuf bytes.Buffer
	o := &Out{W: &out, ErrW: &errBuf}
	o.WriteError(Fail(ExitNotFound, "memory 9 not found", `so memory search "<cue>"`))
	if errBuf.Len() != 0 {
		t.Fatalf("errors must not go to stderr: %q", errBuf.String())
	}
	got := out.String()
	if !strings.Contains(got, "error: memory 9 not found") || !strings.Contains(got, "hint:") {
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
