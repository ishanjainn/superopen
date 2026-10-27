package rules

import (
	"os"
	"strings"
	"testing"
)

func lineWith(t *testing.T, yaml, needle string) int {
	t.Helper()
	for i, l := range strings.Split(yaml, "\n") {
		if strings.Contains(l, needle) {
			return i + 1
		}
	}
	t.Fatalf("no line with %q", needle)
	return 0
}

func TestSetErrorsPointToTheLineToFix(t *testing.T) {
	store := t.TempDir()
	base, err := Get(store, "recursive-root-delete")
	if err != nil {
		t.Fatal(err)
	}
	good := base.YAML
	cases := []struct {
		name   string
		mutate func(string) string
		line   func(string) int
		msg    string
	}{
		{"yaml syntax", func(s string) string { return strings.Replace(s, "severity: critical", "severity: [critical", 1) }, nil, ""},
		{"unknown field", func(s string) string { return s + "colour: red\n" }, func(s string) int { return lineWith(t, s, "colour:") }, `unknown field "colour"`},
		{"bad severity", func(s string) string { return strings.Replace(s, "severity: critical", "severity: urgent", 1) }, func(s string) int { return lineWith(t, s, "severity:") }, "invalid severity"},
		{"bad status", func(s string) string { return strings.Replace(s, "status: stable", "status: ready", 1) }, func(s string) int { return lineWith(t, s, "status:") }, "invalid status"},
		{"missing title", func(s string) string { return strings.Replace(s, "title: ", "ttl: ", 1) }, nil, ""},
		{"bad id", func(s string) string { return strings.Replace(s, "id: recursive-root-delete", "id: Bad ID", 1) }, func(s string) int { return lineWith(t, s, "id:") }, "id"},
		{"bad expression", func(s string) string {
			return strings.Replace(s, `e.event.action == "command.executed" &&`, `e.event.action == "command.executed" && (`, 1)
		}, func(s string) int { return lineWith(t, s, "match:") }, "match expression is invalid"},
		{"failing fixture", func(s string) string { return strings.Replace(s, `command: "rm -rf /"`, `command: "echo hi"`, 1) }, func(s string) int { return lineWith(t, s, "name: rm_rf_root") }, `test "rm_rf_root" expected match but got no_match`},
		{"title line removed", func(s string) string { return strings.Replace(s, "title: "+base.Rule.Title+"\n", "", 1) }, func(s string) int { return lineWith(t, s, "id:") }, "missing field: title is required"},
		{"severity line removed", func(s string) string { return strings.Replace(s, "severity: critical\n", "", 1) }, func(s string) int { return lineWith(t, s, "id:") }, "missing field"},
		{"missing reason", func(s string) string { return strings.Replace(s, "  reason:", "  why:", 1) }, nil, ""},
	}
	for _, c := range cases {
		bad := c.mutate(good)
		_, err := Set(store, []byte(bad))
		if err == nil {
			t.Fatalf("%s: an invalid rule must be rejected", c.name)
		}
		var re *RuleError
		if e, ok := err.(*RuleError); ok {
			re = e
		} else {
			t.Fatalf("%s: want a RuleError with a line, got %T %v", c.name, err, err)
		}
		if re.Line <= 0 {
			t.Fatalf("%s: the error must say which line: %v", c.name, err)
		}
		if c.line != nil && re.Line != c.line(bad) {
			t.Fatalf("%s: line %d, want %d (%v)", c.name, re.Line, c.line(bad), err)
		}
		if c.msg != "" && !strings.Contains(re.Msg, c.msg) {
			t.Fatalf("%s: message %q should contain %q", c.name, re.Msg, c.msg)
		}
		if strings.HasPrefix(err.Error(), "line ") == false {
			t.Fatalf("%s: text form must lead with the line: %q", c.name, err.Error())
		}
	}
	if entries, _ := os.ReadDir(store); len(entries) != 0 {
		t.Fatalf("no rejected rule may be written: %v", entries)
	}
}

func TestSetRejectsAnEmptyRule(t *testing.T) {
	if _, err := Set(t.TempDir(), []byte("  \n")); err == nil || LineOf(err) != 0 {
		t.Fatalf("an empty rule is rejected without a line: %v", err)
	}
}
