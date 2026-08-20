package memory

import (
	"strings"
	"unicode"
)

const (
	minCaptureLen = 12
	maxCaptureLen = 8000
)

func blockedCapture(text string) bool {
	s := strings.ToLower(strings.TrimSpace(text))
	if s == "" {
		return false
	}
	for _, p := range []string{
		"ignore previous instructions",
		"ignore previous",
		"you are now",
		"from now on you",
		"disregard previous",
		"new instructions:",
		"system: you",
	} {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
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
