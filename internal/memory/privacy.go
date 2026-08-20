package memory

import (
	"strings"

	"github.com/ishanjainn/superopen/internal/redact"
)

const maxStoredRunes = 8000
const privateSkip = "[EXCLUDED_PRIVATE]"

// Sanitize strips <private> blocks and secrets. Empty after privacy tags
// means the moment must not be stored or sent to a distill prompt.
func Sanitize(s string) string {
	s = redact.Private(s)
	if strings.TrimSpace(s) == "" || strings.TrimSpace(s) == privateSkip {
		return ""
	}
	s = redact.StringFull(s)
	s = strings.TrimSpace(s)
	if s == "" || s == privateSkip {
		return ""
	}
	runes := []rune(s)
	if len(runes) > maxStoredRunes {
		s = string(runes[:maxStoredRunes]) + "…"
	}
	return s
}

func skipPrivate(s string) bool {
	out := Sanitize(s)
	if out == "" || out == privateSkip {
		return true
	}
	return strings.TrimSpace(strings.ReplaceAll(out, privateSkip, "")) == ""
}
