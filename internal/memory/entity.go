package memory

import (
	"regexp"
	"strings"
	"unicode"
)

var identRe = regexp.MustCompile(`[A-Za-z][A-Za-z0-9_\-./]{2,}`)

func entityTags(text string) string {
	seen := map[string]bool{}
	var out []string
	for _, m := range identRe.FindAllString(text, 24) {
		if len(m) > 48 || seen[m] {
			continue
		}
		if isStopWord(m) {
			continue
		}
		seen[m] = true
		out = append(out, m)
		if len(out) >= 12 {
			break
		}
	}
	return strings.Join(out, ",")
}

func isStopWord(s string) bool {
	switch strings.ToLower(s) {
	case "the", "and", "for", "that", "this", "with", "from", "have", "been", "were", "will", "when", "what", "which":
		return true
	}
	for _, r := range s {
		if unicode.IsUpper(r) {
			return false
		}
	}
	return len(s) < 5
}
