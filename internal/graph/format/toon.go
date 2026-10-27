package format

import (
	"fmt"
	"strconv"
	"strings"
)

// TOON (toonformat.dev) is the one agent-facing record shape: `key: value`
// scalars and `name[N]{cols}:` tables. Every graph command renders through
// these helpers so an agent never parses two dialects in one session.

// Cell quotes a TOON value when it could be misread: delimiters, quotes,
// structure characters, edge whitespace, or a leading - or #.
func Cell(s string) string {
	if s == "" {
		return `""`
	}
	needs := strings.ContainsAny(s, ",:\"\\[]{}\n\r\t") || strings.TrimSpace(s) != s ||
		strings.HasPrefix(s, "-") || strings.HasPrefix(s, "#")
	// Numeric strings (line numbers, counts) stay bare: agents read them as
	// numbers either way, and quoting every line column is noise.
	if !needs {
		return s
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	return `"` + r.Replace(s) + `"`
}

// Table writes `name[N]{cols}:` and one comma-separated row per line.
func Table(b *strings.Builder, indent, name string, cols []string, rows [][]string) {
	fmt.Fprintf(b, "%s%s[%d]{%s}:\n", indent, name, len(rows), strings.Join(cols, ","))
	for _, row := range rows {
		cells := make([]string, len(row))
		for i, v := range row {
			cells[i] = Cell(v)
		}
		fmt.Fprintf(b, "%s  %s\n", indent, strings.Join(cells, ","))
	}
}

// Field writes `key: value` with TOON quoting.
func Field(b *strings.Builder, indent, key string, value any) {
	switch v := value.(type) {
	case string:
		fmt.Fprintf(b, "%s%s: %s\n", indent, key, Cell(v))
	default:
		fmt.Fprintf(b, "%s%s: %v\n", indent, key, v)
	}
}

// List writes `name[N]: a,b,c` for short scalar lists.
func List(b *strings.Builder, indent, name string, values []string) {
	cells := make([]string, len(values))
	for i, v := range values {
		cells[i] = Cell(v)
	}
	fmt.Fprintf(b, "%s%s[%d]: %s\n", indent, name, len(values), strings.Join(cells, ","))
}

// Code writes a source block with absolute line numbers so an agent can
// answer file:line without reading the file again. The header carries the
// shown and total line counts when the body was clipped.
func Code(b *strings.Builder, indent string, start, total int, code string) {
	code = strings.TrimRight(code, "\n")
	lines := strings.Split(code, "\n")
	if code == "" {
		lines = nil
	}
	if total > len(lines) {
		fmt.Fprintf(b, "%scode[%d of %d lines]:\n", indent, len(lines), total)
	} else {
		fmt.Fprintf(b, "%scode[%d lines]:\n", indent, len(lines))
	}
	if start <= 0 {
		start = 1
	}
	width := len(strconv.Itoa(start + len(lines) - 1))
	for i, line := range lines {
		fmt.Fprintf(b, "%s  %*d| %s\n", indent, width, start+i, line)
	}
}

// Count is AXI's definitive total: `count: shown of total`.
func Count(b *strings.Builder, shown, total int) {
	if total < shown {
		total = shown
	}
	fmt.Fprintf(b, "count: %d of %d\n", shown, total)
}

// Quote renders a value for a suggested shell command.
func Quote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}
