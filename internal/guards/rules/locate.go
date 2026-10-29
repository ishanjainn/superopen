package rules

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// RuleError is a rule rejected at a place in its YAML. Line is 1-based, or 0
// when the problem is not tied to one line (an empty file).
type RuleError struct {
	Line int
	Msg  string
}

func (e *RuleError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
	}
	return e.Msg
}

// LineOf returns the line of err when it is a RuleError, else 0.
func LineOf(err error) int {
	var re *RuleError
	if errors.As(err, &re) {
		return re.Line
	}
	return 0
}

var (
	yamlLine     = regexp.MustCompile(`line (\d+)`)
	unknownField = regexp.MustCompile(`field (\S+) not found in type`)
	ruleIDPrefix = regexp.MustCompile(`^rule "[^"]*": `)
	celPosition  = regexp.MustCompile(`<input>:(\d+):(\d+): `)
)

// decodeError turns a YAML decode failure into a RuleError with its line.
func decodeError(err error) *RuleError {
	text := err.Error()
	line := 0
	if m := yamlLine.FindStringSubmatch(text); m != nil {
		line, _ = strconv.Atoi(m[1])
	}
	msg := text
	if m := unknownField.FindStringSubmatch(text); m != nil {
		msg = fmt.Sprintf("unknown field %q", m[1])
	} else {
		msg = strings.TrimPrefix(msg, "yaml: ")
		msg = yamlLine.ReplaceAllString(msg, "")
		msg = strings.TrimLeft(strings.TrimSpace(msg), ": ")
		if i := strings.Index(msg, "\n"); i >= 0 {
			msg = msg[:i]
		}
		if msg == "" {
			msg = "the YAML could not be read"
		}
	}
	return &RuleError{Line: line, Msg: msg}
}

// keyLine returns the 1-based line where key is set. A top-level key must start
// the line; a nested one may be indented. Zero when absent.
func keyLine(data []byte, key string, nested bool) int {
	pattern := `^` + regexp.QuoteMeta(key) + `:`
	if nested {
		pattern = `^\s+(- )?` + regexp.QuoteMeta(key) + `:`
	}
	re := regexp.MustCompile(pattern)
	for i, line := range strings.Split(string(data), "\n") {
		if re.MatchString(line) {
			return i + 1
		}
	}
	return 0
}

// validationError places a rule.Validate / CheckRule failure on the field the
// message is about, and trims the wrapper text around it.
func validationError(data []byte, err error) *RuleError {
	msg := ruleIDPrefix.ReplaceAllString(err.Error(), "")
	line := 0
	switch {
	case strings.HasPrefix(msg, "id "):
		line = keyLine(data, "id", false)
	case strings.HasPrefix(msg, "version"):
		line = keyLine(data, "version", false)
	case strings.HasPrefix(msg, "title"):
		line = keyLine(data, "title", false)
	case strings.HasPrefix(msg, "invalid severity"):
		line = keyLine(data, "severity", false)
		msg += " (use info, low, medium, high or critical)"
	case strings.HasPrefix(msg, "invalid status"):
		line = keyLine(data, "status", false)
		msg += " (use experimental, stable or deprecated)"
	case strings.HasPrefix(msg, "invalid posture"):
		line = keyLine(data, "posture", false)
		msg += " (use detect or enforce-capable)"
	case strings.HasPrefix(msg, "emit.reason"):
		line = keyLine(data, "reason", true)
		if line == 0 {
			line = keyLine(data, "emit", false)
		}
	case strings.HasPrefix(msg, "at least one test"):
		line = keyLine(data, "tests", false)
	case strings.HasPrefix(msg, "match:"):
		line = keyLine(data, "match", false)
		msg = celMessage(strings.TrimPrefix(msg, "match: "))
	case strings.HasPrefix(msg, "correlation.window"):
		line = keyLine(data, "window", true)
	case strings.HasPrefix(msg, "correlation.scope"):
		line = keyLine(data, "scope", true)
	case strings.HasPrefix(msg, "correlation"):
		line = keyLine(data, "correlation", false)
		msg = celMessage(msg)
	case strings.HasPrefix(msg, "exactly one of match or correlation"):
		line = keyLine(data, "match", false)
		if line == 0 {
			line = keyLine(data, "correlation", false)
		}
		if line == 0 {
			line = keyLine(data, "id", false)
		}
	default:
		// Maturity-gate and other rule-level failures.
		if strings.Contains(msg, "status") || strings.Contains(msg, "stable") {
			line = keyLine(data, "status", false)
		}
	}
	if line == 0 {
		// A missing field has no line of its own: point at the top of the rule.
		line = 1
		if id := keyLine(data, "id", false); id > 0 {
			line = id
		}
		if strings.HasPrefix(msg, "title") || strings.HasPrefix(msg, "invalid severity") ||
			strings.HasPrefix(msg, "invalid status") || strings.HasPrefix(msg, "invalid posture") ||
			strings.HasPrefix(msg, "emit.reason") || strings.HasPrefix(msg, "at least one test") {
			msg = "missing field: " + msg
		}
	}
	return &RuleError{Line: line, Msg: msg}
}

// celMessage keeps the first line of a CEL compile error and says where in
// the expression it points.
func celMessage(msg string) string {
	first := msg
	if i := strings.Index(first, "\n"); i >= 0 {
		first = first[:i]
	}
	first = strings.TrimPrefix(first, "compile match: ")
	if m := celPosition.FindStringSubmatch(first); m != nil {
		first = celPosition.ReplaceAllString(first, "")
		first = strings.TrimPrefix(first, "ERROR: ")
		return fmt.Sprintf("the match expression is invalid (expression line %s, column %s): %s", m[1], m[2], first)
	}
	return "the match expression is invalid: " + strings.TrimPrefix(first, "ERROR: ")
}

// fixtureError places a failing test fixture on its `name:` line.
func fixtureError(data []byte, name string, detail string) *RuleError {
	line := 0
	re := regexp.MustCompile(`^\s+- name:\s*"?` + regexp.QuoteMeta(name) + `"?\s*$`)
	for i, l := range strings.Split(string(data), "\n") {
		if re.MatchString(l) {
			line = i + 1
			break
		}
	}
	return &RuleError{Line: line, Msg: fmt.Sprintf("test %q %s", name, detail)}
}
