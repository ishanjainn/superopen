package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Toon renders any JSON-shaped value (a struct, map or slice) as TOON, the
// one dialect every command speaks: `key: value` scalars, nested maps
// indented under `key:`, scalar lists as `key[N]: a,b`, and lists of flat
// records as `key[N]{cols}:` tables. Keys are sorted so output is stable.
// Lists past ToonMaxRows and strings past ToonMaxChars are clipped with a size
// hint unless full is set (AXI §3).
func Toon(v any, full bool) string {
	var norm any
	raw, err := json.Marshal(v)
	if err != nil || json.Unmarshal(raw, &norm) != nil {
		return fmt.Sprint(v) + "\n"
	}
	e := toonEncoder{full: full}
	e.value("", norm, 0)
	return e.b.String()
}

const (
	// ToonMaxRows is the longest table or list printed by default.
	ToonMaxRows = 20
	// ToonMaxChars is the longest string value printed by default.
	ToonMaxChars = 200
)

type toonEncoder struct {
	b    strings.Builder
	full bool
}

func (e *toonEncoder) line(depth int, format string, args ...any) {
	e.b.WriteString(strings.Repeat("  ", depth))
	fmt.Fprintf(&e.b, format, args...)
	e.b.WriteByte('\n')
}

// value writes key and its value. An empty key means the top-level value.
func (e *toonEncoder) value(key string, v any, depth int) {
	switch x := v.(type) {
	case map[string]any:
		if key != "" {
			if len(x) == 0 {
				e.line(depth, "%s: none", key)
				return
			}
			e.line(depth, "%s:", key)
			depth++
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			e.value(k, x[k], depth)
		}
	case []any:
		e.list(key, x, depth)
	default:
		e.line(depth, "%s: %s", key, e.scalar(x, false))
	}
}

func (e *toonEncoder) list(key string, items []any, depth int) {
	if key == "" {
		key = "items"
	}
	total := len(items)
	if total == 0 {
		e.line(depth, "%s: none", key)
		return
	}
	shown := items
	if !e.full && len(shown) > ToonMaxRows {
		shown = shown[:ToonMaxRows]
	}
	switch {
	case allScalars(items):
		cells := make([]string, len(shown))
		for i, item := range shown {
			cells[i] = e.scalar(item, true)
		}
		e.line(depth, "%s[%d]: %s", key, total, strings.Join(cells, ","))
	case cols(items) != nil:
		columns := cols(items)
		e.line(depth, "%s[%d]{%s}:", key, total, strings.Join(columns, ","))
		for _, item := range shown {
			rec := item.(map[string]any)
			cells := make([]string, len(columns))
			for i, c := range columns {
				cells[i] = e.scalar(rec[c], true)
			}
			e.line(depth+1, "%s", strings.Join(cells, ","))
		}
	default:
		e.line(depth, "%s[%d]:", key, total)
		for _, item := range shown {
			var sub toonEncoder
			sub.full = e.full
			sub.value("", item, 0)
			for i, l := range strings.Split(strings.TrimRight(sub.b.String(), "\n"), "\n") {
				prefix := "  - "
				if i > 0 {
					prefix = "    "
				}
				e.line(depth, "%s%s", prefix, l)
			}
		}
	}
	if len(shown) < total {
		e.line(depth, "%s_shown: %d of %d (use --full for all)", key, len(shown), total)
	}
}

func allScalars(items []any) bool {
	for _, item := range items {
		switch item.(type) {
		case map[string]any, []any:
			return false
		}
	}
	return true
}

// cols returns the sorted column set when every item is a flat record, else nil.
func cols(items []any) []string {
	set := map[string]bool{}
	for _, item := range items {
		rec, ok := item.(map[string]any)
		if !ok || len(rec) == 0 {
			return nil
		}
		for k, v := range rec {
			switch v.(type) {
			case map[string]any, []any:
				return nil
			}
			set[k] = true
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	if len(out) > 8 {
		return nil
	}
	return out
}

// scalar formats a leaf. cell quotes anything a table or list would misread.
func (e *toonEncoder) scalar(v any, cell bool) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case string:
		s := x
		if n := utf8.RuneCountInString(s); !e.full && n > ToonMaxChars {
			s = string([]rune(s)[:ToonMaxChars]) + fmt.Sprintf("… (truncated, %d chars — use --full)", n)
		}
		return ToonQuote(s, cell)
	default:
		return ToonQuote(fmt.Sprint(x), cell)
	}
}

// ToonQuote quotes s when plain text would be ambiguous. A cell (inside a
// table or list) also needs quoting for commas.
func ToonQuote(s string, cell bool) string {
	needs := s == "" || s != strings.TrimSpace(s) || strings.ContainsAny(s, "\n\r\"") ||
		(cell && strings.Contains(s, ",")) || s == "null" || s == "true" || s == "false"
	if !needs {
		return s
	}
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	return `"` + s + `"`
}
