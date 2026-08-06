package axi

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
	if !strings.Contains(got, "so init") {
		t.Fatalf("missing next: %q", got)
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
