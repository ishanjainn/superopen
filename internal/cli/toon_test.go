package cli

import (
	"strings"
	"testing"
)

func TestToonScalarsMapsAndTables(t *testing.T) {
	got := Toon(map[string]any{
		"state": "ready",
		"nodes": 49124,
		"ratio": 0.5,
		"ok":    true,
		"note":  "a, b",
		"engine": map[string]any{
			"name": "so-graph",
		},
		"tags": []string{"x", "y"},
		"rows": []map[string]any{
			{"path": "a.py", "kind": "parse_partial"},
			{"path": "b,c.py", "kind": "ok"},
		},
		"none": []string{},
	}, false)
	want := `engine:
  name: so-graph
nodes: 49124
none: none
note: a, b
ok: true
ratio: 0.5
rows[2]{kind,path}:
  parse_partial,a.py
  ok,"b,c.py"
state: ready
tags[2]: x,y
`
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestToonClipsLongListsAndStringsUnlessFull(t *testing.T) {
	var rows []map[string]any
	for i := 0; i < 50; i++ {
		rows = append(rows, map[string]any{"n": i})
	}
	clipped := Toon(map[string]any{"rows": rows, "text": strings.Repeat("x", 500)}, false)
	if !strings.Contains(clipped, "rows[50]{n}:") || !strings.Contains(clipped, "rows_shown: 20 of 50 (use --full for all)") {
		t.Fatalf("long list must report shown of total:\n%s", clipped)
	}
	if !strings.Contains(clipped, "(truncated, 500 chars — use --full)") {
		t.Fatalf("long string must carry a size hint:\n%s", clipped)
	}
	full := Toon(map[string]any{"rows": rows, "text": strings.Repeat("x", 500)}, true)
	if strings.Contains(full, "rows_shown") || strings.Contains(full, "truncated") {
		t.Fatalf("--full must not clip:\n%s", full)
	}
}

func TestToonNestedRecordsAndQuoting(t *testing.T) {
	got := Toon(map[string]any{
		"items": []map[string]any{{"id": 1, "tags": []string{"a"}}, {"id": 2, "tags": []string{}}},
		"empty": "",
		"flag":  "true",
	}, false)
	for _, want := range []string{"items[2]:\n  - id: 1\n    tags[1]: a\n  - id: 2\n    tags: none\n", `empty: ""`, `flag: "true"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestOKAndObjectAreDeterministicTOON(t *testing.T) {
	var buf strings.Builder
	o := &Out{W: &buf, ErrW: &buf}
	o.OK("pruned", map[string]any{"b": 2, "a": 1})
	if buf.String() != "ok: pruned\na: 1\nb: 2\n" {
		t.Fatalf("OK output must be sorted TOON: %q", buf.String())
	}
}

func TestRowsPrintCountFirstThenTable(t *testing.T) {
	var buf strings.Builder
	o := &Out{W: &buf, ErrW: &buf}
	o.RowsOf("projects", []string{"id", "name"}, []map[string]any{{"id": "a1", "name": "x"}}, 9)
	want := "count: 1 of 9\nprojects[1]{id,name}:\n  a1,x\n"
	if buf.String() != want {
		t.Fatalf("got %q want %q", buf.String(), want)
	}
}

func TestRowsLeaveBooleansAndNumbersUnquoted(t *testing.T) {
	var buf strings.Builder
	o := &Out{W: &buf, ErrW: &buf}
	o.Rows("rules", []string{"id", "enabled", "n", "note"}, []map[string]any{{"id": "a", "enabled": true, "n": 3, "note": "true"}})
	if !strings.Contains(buf.String(), "  a,true,3,\"true\"\n") {
		t.Fatalf("got %q", buf.String())
	}
}
