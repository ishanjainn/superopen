package codingotlp

import (
	"os"
	"strings"
)

// firstEnv returns the first non-empty trimmed environment value.
// Prefer SUPEROPEN_* / SO_* names; pass legacy OPENLIT_* keys last for older installs.
func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}
