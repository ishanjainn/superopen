package codingotlp

import (
	"os"
	"strings"
)

// firstEnv returns the first non-empty trimmed environment value.
// Prefer SUPEROPEN_* names; SO_* aliases may be passed as fallbacks.
func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}
