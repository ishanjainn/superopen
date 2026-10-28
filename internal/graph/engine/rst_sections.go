package engine

import (
	"strings"
	"unicode/utf8"
)

type rstLine struct {
	text   string
	start  uint32
	indent int
	blank  bool
}

type rstTitle struct {
	first int
	line  int
	next  int
	name  string
}

// ReST titles use a punctuation adornment above and below, or below only.
// Sections end immediately before the next title, including their prose.
func extractRSTSections(source []byte) []SyntaxFact {
	parts := strings.Split(string(source), "\n")
	lines := make([]rstLine, len(parts))
	var offset uint32
	for i, part := range parts {
		text := expandRSTTabs(strings.TrimSuffix(part, "\r"))
		lines[i] = rstLine{text: text, start: offset, indent: len(text) - len(strings.TrimLeft(text, " ")),
			blank: strings.TrimSpace(text) == ""}
		offset += uint32(len(part)) + 1
	}
	var titles []rstTitle
	for i := 0; i < len(lines); {
		if lines[i].blank || lines[i].indent != 0 || i > 0 && !lines[i-1].blank {
			i++
			continue
		}
		if first, ok := rstAdorn(lines[i].text); ok {
			if i+2 < len(lines) && !lines[i+1].blank && !rstIsAdorn(lines[i+1].text) {
				if last, under := rstAdorn(lines[i+2].text); under && first == last {
					titles = append(titles, rstTitle{first: i, line: i + 1, next: i + 3,
						name: strings.Join(strings.Fields(lines[i+1].text), " ")})
					i += 3
					continue
				}
			}
			i++
			continue
		}
		if i+1 < len(lines) && !strings.HasPrefix(lines[i].text, "..") {
			if _, ok := rstAdorn(lines[i+1].text); ok {
				underline := strings.TrimRight(lines[i+1].text, " \t\r")
				if len(underline) >= utf8.RuneCountInString(strings.TrimSpace(lines[i].text)) || len(underline) >= 4 {
					titles = append(titles, rstTitle{first: i, line: i, next: i + 2,
						name: strings.Join(strings.Fields(lines[i].text), " ")})
					i += 2
					continue
				}
			}
		}
		i++
	}
	result := make([]SyntaxFact, 0, len(titles))
	for i, title := range titles {
		end := len(lines)
		if i+1 < len(titles) {
			end = titles[i+1].first
		}
		last := end - 1
		for last > title.line && lines[last].blank {
			last--
		}
		if title.name == "" {
			continue
		}
		fact := SyntaxFact{Kind: "section", Name: title.name, NodeType: "rst_title",
			StartByte: lines[title.line].start, EndByte: lines[last].start + uint32(len(lines[last].text)),
			StartLine: title.line + 1, StartColumn: 1, EndLine: last + 1, EndColumn: len(lines[last].text) + 1,
			Confidence: 1, Lines: last - title.line + 1}
		if title.next < end {
			fact.Docstring = rstSectionBody(parts[title.next:end])
		}
		result = append(result, fact)
	}
	return result
}

func rstSectionBody(lines []string) string {
	var body strings.Builder
	for _, line := range lines {
		for _, word := range strings.Fields(line) {
			if body.Len() > 0 {
				if body.Len() == 500 {
					return body.String()
				}
				body.WriteByte(' ')
			}
			for _, char := range word {
				if body.Len()+utf8.RuneLen(char) > 500 {
					return body.String()
				}
				body.WriteRune(char)
			}
		}
	}
	return body.String()
}

func rstAdorn(line string) (byte, bool) {
	line = strings.TrimRight(line, " \t\r")
	if line == "" || line == "::" {
		return 0, false
	}
	char := line[0]
	if !((char >= '!' && char <= '/') || (char >= ':' && char <= '@') ||
		(char >= '[' && char <= '`') || (char >= '{' && char <= '~')) {
		return 0, false
	}
	for i := 1; i < len(line); i++ {
		if line[i] != char {
			return 0, false
		}
	}
	return char, true
}

func rstIsAdorn(line string) bool {
	_, ok := rstAdorn(line)
	return ok
}

func expandRSTTabs(line string) string {
	if !strings.ContainsRune(line, '\t') {
		return line
	}
	var out strings.Builder
	column := 0
	for _, char := range line {
		if char == '\t' {
			spaces := 8 - column%8
			for i := 0; i < spaces; i++ {
				out.WriteByte(' ')
			}
			column += spaces
		} else {
			out.WriteRune(char)
			column++
		}
	}
	return out.String()
}
