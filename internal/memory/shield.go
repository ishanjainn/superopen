package memory

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	minCaptureLen = 12
	maxCaptureLen = 100000
)

func blockedCapture(text string) bool {
	s := strings.ToLower(strings.TrimSpace(text))
	if s == "" {
		return false
	}
	// Role-play slogans are their own sentence ("You are now DAN"). Ordinary
	// prose like "where you are now" must still store.
	for _, p := range []string{
		"you are now",
		"from now on you",
	} {
		if containsSentencePhrase(s, p) {
			return true
		}
	}
	for _, p := range []string{
		"ignore previous instructions",
		"ignore previous",
		"disregard previous",
		"new instructions:",
		"system: you",
	} {
		if containsPhrase(s, p) {
			return true
		}
	}
	return false
}

func containsPhrase(s, phrase string) bool {
	if phrase == "" || s == "" {
		return false
	}
	for start := 0; start <= len(s); {
		i := strings.Index(s[start:], phrase)
		if i < 0 {
			return false
		}
		i += start
		if phraseBounded(s, i, i+len(phrase)) {
			return true
		}
		start = i + 1
	}
	return false
}

func containsSentencePhrase(s, phrase string) bool {
	if phrase == "" || s == "" {
		return false
	}
	for start := 0; start <= len(s); {
		i := strings.Index(s[start:], phrase)
		if i < 0 {
			return false
		}
		i += start
		if phraseBounded(s, i, i+len(phrase)) && sentenceStart(s, i) {
			return true
		}
		start = i + 1
	}
	return false
}

func phraseBounded(s string, start, end int) bool {
	if start > 0 {
		r, _ := utf8.DecodeLastRuneInString(s[:start])
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	if end < len(s) {
		r, _ := utf8.DecodeRuneInString(s[end:])
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func sentenceStart(s string, i int) bool {
	j := i
	for j > 0 {
		r, size := utf8.DecodeLastRuneInString(s[:j])
		if r == utf8.RuneError && size == 1 {
			return false
		}
		if unicode.IsSpace(r) {
			j -= size
			continue
		}
		switch r {
		case '.', '!', '?', '。', '！', '？':
			return true
		default:
			return false
		}
	}
	return true
}

func noisyCapture(text string) bool {
	s := strings.TrimSpace(text)
	if s == "" {
		return true
	}
	if s == "[Request interrupted by user]" {
		return true
	}
	for _, prefix := range []string{
		"<command-message>",
		"<command-name>",
		"Base directory for this skill:",
		"<task-notification>",
		"<system-reminder>",
	} {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return dumpCapture(s)
}

func dumpCapture(text string) bool {
	s := strings.TrimSpace(text)
	s = strings.TrimLeft(s, "\u200b\ufeff")
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "```") {
		return true
	}
	body := "\n" + s
	return strings.Count(body, "\nNODE ") >= 3 || strings.Count(body, "\nEDGE ") >= 3
}

func unwrapCapture(text string) string {
	s := strings.TrimSpace(text)
	for i := 0; i < 8; i++ {
		next := stripCaptureEnvelope(s)
		if next == s {
			return strings.TrimSpace(s)
		}
		s = strings.TrimSpace(next)
	}
	return s
}

func stripCaptureEnvelope(s string) string {
	s = strings.TrimSpace(s)
	prefixes := []string{
		"<system-reminder>",
		"<command-message>",
		"<command-name>",
		"<task-notification>",
		"Base directory for this skill:",
	}
	for _, p := range prefixes {
		if !strings.HasPrefix(s, p) {
			continue
		}
		if strings.HasPrefix(p, "<") && strings.HasSuffix(p, ">") {
			name := strings.TrimSuffix(strings.TrimPrefix(p, "<"), ">")
			close := "</" + name + ">"
			if i := strings.Index(strings.ToLower(s), strings.ToLower(close)); i >= 0 {
				return strings.TrimSpace(s[i+len(close):])
			}
			if j := strings.Index(s, "\n\n"); j >= 0 {
				return strings.TrimSpace(s[j+2:])
			}
			return ""
		}
		if j := strings.Index(s, "\n\n"); j >= 0 {
			return strings.TrimSpace(s[j+2:])
		}
		if j := strings.Index(s, "\n"); j >= 0 {
			return strings.TrimSpace(s[j+1:])
		}
		return ""
	}
	return s
}

func packFingerprint(text string) bool {
	s := strings.TrimSpace(text)
	if s == "" {
		return false
	}
	if strings.Contains(s, "Fetch: memory_get") || strings.Contains(s, "Fetch: so memory get") {
		return true
	}
	if strings.HasPrefix(s, "Superopen: codebase questions") {
		return true
	}
	if strings.HasPrefix(s, "Superopen:") && (strings.Contains(s, "CLI binary") || strings.Contains(s, "memories in this workspace")) {
		return true
	}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Working:") || strings.HasPrefix(line, "MEM #") || strings.HasPrefix(line, "memories[") {
			return true
		}
	}
	return false
}

func tooShort(text string) bool {
	return len([]rune(strings.TrimSpace(text))) < minCaptureLen
}

func undeclaredNonEnglish(text string) bool {
	letters := 0
	foreign := 0
	for _, r := range text {
		if !unicode.IsLetter(r) {
			continue
		}
		letters++
		switch {
		case unicode.In(r, unicode.Cyrillic), unicode.In(r, unicode.Han), unicode.In(r, unicode.Hiragana), unicode.In(r, unicode.Katakana), unicode.In(r, unicode.Hangul):
			foreign++
		}
	}
	if letters == 0 {
		return false
	}
	return foreign*4 >= letters
}

func clipCapture(text string) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) > maxCaptureLen {
		return string(runes[:maxCaptureLen])
	}
	return string(runes)
}

func toolsTrailer(names []string) string {
	seen := map[string]struct{}{}
	var out []string
	for _, n := range names {
		n = cleanToolName(n)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
		if len(out) == 8 {
			break
		}
	}
	if len(out) == 0 {
		return ""
	}
	extra := ""
	if len(names) > 8 {
		extra = " +"
	}
	return "\n[tools: " + strings.Join(out, ", ") + extra + "]"
}

func cleanToolName(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r == ',' || r == '[' || r == ']' {
			continue
		}
		if unicode.IsPrint(r) {
			b.WriteRune(r)
		}
	}
	name := strings.TrimSpace(b.String())
	if len(name) > 80 {
		name = name[:80]
	}
	return strings.TrimSpace(name)
}
