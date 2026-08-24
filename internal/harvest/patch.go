package harvest

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

type Hunk struct {
	OldStart int      `json:"old_start"`
	OldCount int      `json:"old_count"`
	NewStart int      `json:"new_start"`
	NewCount int      `json:"new_count"`
	Lines    []string `json:"lines"` // includes leading ' ', '+', '-'
}

type Patch struct {
	OldFile string `json:"old_file,omitempty"`
	NewFile string `json:"new_file,omitempty"`
	Hunks   []Hunk `json:"hunks"`
	Plus    int    `json:"plus"`
	Minus   int    `json:"minus"`
}

func ParseUnified(diff string) (Patch, error) {
	var p Patch
	lines := strings.Split(strings.ReplaceAll(diff, "\r\n", "\n"), "\n")
	var cur *Hunk
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "--- "):
			p.OldFile = strings.TrimSpace(strings.TrimPrefix(line, "--- "))
			p.OldFile = strings.TrimPrefix(p.OldFile, "a/")
		case strings.HasPrefix(line, "+++ "):
			p.NewFile = strings.TrimSpace(strings.TrimPrefix(line, "+++ "))
			p.NewFile = strings.TrimPrefix(p.NewFile, "b/")
		case strings.HasPrefix(line, "@@"):
			m := hunkHeader.FindStringSubmatch(line)
			if m == nil {
				return p, fmt.Errorf("bad hunk header: %s", line)
			}
			if cur != nil {
				p.Hunks = append(p.Hunks, *cur)
			}
			h := Hunk{
				OldStart: atoiDefault(m[1], 0),
				OldCount: atoiDefault(m[2], 1),
				NewStart: atoiDefault(m[3], 0),
				NewCount: atoiDefault(m[4], 1),
			}
			cur = &h
		default:
			if cur == nil {
				continue
			}
			if line == "" {
				continue
			}
			prefix := line[0]
			if prefix != ' ' && prefix != '+' && prefix != '-' && prefix != '\\' {
				continue
			}
			if prefix == '+' {
				p.Plus++
			}
			if prefix == '-' {
				p.Minus++
			}
			cur.Lines = append(cur.Lines, line)
		}
	}
	if cur != nil {
		p.Hunks = append(p.Hunks, *cur)
	}
	if len(p.Hunks) == 0 && strings.TrimSpace(diff) != "" && !strings.Contains(diff, "@@") {
		return p, fmt.Errorf("not a unified diff")
	}
	return p, nil
}

func (p Patch) Additive() bool {
	return p.Minus == 0 && p.Plus > 0
}

func ApplyUnified(src, diff string) (string, error) {
	patch, err := ParseUnified(diff)
	if err != nil {
		return "", err
	}
	orig := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	// Keep a trailing empty slice element only if file ended with newline handling:
	if len(orig) == 1 && orig[0] == "" && src == "" {
		orig = []string{}
	}
	var out []string
	cursor := 0 // 0-based index into orig
	for _, h := range patch.Hunks {
		start := h.OldStart - 1
		if start < 0 {
			start = 0
		}
		if start > len(orig) {
			return "", fmt.Errorf("hunk starts past end of file")
		}
		out = append(out, orig[cursor:start]...)
		cursor = start
		for _, line := range h.Lines {
			if line == "" {
				continue
			}
			op, body := line[0], line[1:]
			switch op {
			case ' ':
				if cursor >= len(orig) || orig[cursor] != body {
					return "", fmt.Errorf("context mismatch at line %d", cursor+1)
				}
				out = append(out, orig[cursor])
				cursor++
			case '-':
				if cursor >= len(orig) || orig[cursor] != body {
					return "", fmt.Errorf("remove mismatch at line %d", cursor+1)
				}
				cursor++
			case '+':
				out = append(out, body)
			case '\\':
				// "\ No newline at end of file"
			}
		}
	}
	if cursor < len(orig) {
		out = append(out, orig[cursor:]...)
	}
	result := strings.Join(out, "\n")
	if strings.HasSuffix(src, "\n") && !strings.HasSuffix(result, "\n") {
		result += "\n"
	}
	return result, nil
}

func AlreadyContains(src, diff string) bool {
	patch, err := ParseUnified(diff)
	if err != nil {
		return false
	}
	for _, h := range patch.Hunks {
		for _, line := range h.Lines {
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				if !strings.Contains(src, line[1:]) {
					return false
				}
			}
		}
	}
	return patch.Plus > 0
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func DiffStats(diff string) (plus, minus int) {
	p, err := ParseUnified(diff)
	if err != nil {
		return 0, 0
	}
	return p.Plus, p.Minus
}
