package blocks

import (
	"strings"
)

// BlockFor identifies a built-in that can replace a repeated shell pattern.
// Plain test output is routed to first-error because test-summary accepts
// structured reports only.
func BlockFor(cmd string) string {
	fields := strings.Fields(cmd)
	low := strings.ToLower(strings.Join(fields, " "))
	if low == "" || mentionsSO(low) {
		return ""
	}
	switch {
	case testTail(low):
		return "first-error"
	case jsonPeek(low):
		return "json-peek"
	case jsonlStats(low):
		return "jsonl-stats"
	case firstErr(low):
		return "first-error"
	case httpJSON(low):
		return "http-json"
	case replaceFile(low):
		return "replace-in-file"
	case waitFor(low):
		return "wait-for"
	default:
		return ""
	}
}

// Hint is a one-line, actionable suggestion. It never denies or rewrites a command.
func Hint(cmd string) string {
	switch BlockFor(cmd) {
	case "first-error":
		return "save full command output, then run so blocks run first-error <file>"
	case "json-peek":
		return "use so blocks run json-peek <file> [path]"
	case "jsonl-stats":
		return "use so blocks run jsonl-stats <file>"
	case "http-json":
		return "use so blocks run http-json <url>"
	case "replace-in-file":
		return "use so blocks run replace-in-file <file> <old> <new>"
	case "wait-for":
		return "use so blocks run wait-for --file <file> --pattern <text>"
	default:
		return ""
	}
}

func mentionsSO(low string) bool {
	return strings.Contains(low, " so ") || strings.HasPrefix(low, "so ") ||
		strings.Contains(low, "/so ") || strings.Contains(low, "so.exe") ||
		strings.Contains(low, " so.exe")
}

func testTail(low string) bool {
	if !strings.Contains(low, "tail") && !strings.Contains(low, "head") {
		return false
	}
	return strings.Contains(low, "go test") || strings.Contains(low, "pytest") ||
		strings.Contains(low, "npm test") || strings.Contains(low, "pnpm test") ||
		strings.Contains(low, "yarn test") || strings.Contains(low, "cargo test")
}

func jsonPeek(low string) bool {
	if strings.Contains(low, "json.load") || strings.Contains(low, "json.loads") || strings.Contains(low, "json.parse") {
		return strings.Contains(low, "python") || strings.Contains(low, "node")
	}
	return strings.Contains(low, "jq 'keys") || strings.Contains(low, `jq ".|keys"`) || strings.Contains(low, "jq 'keys_unsorted")
}

func jsonlStats(low string) bool {
	return strings.Contains(low, "jsonl") && (strings.Contains(low, "wc ") || strings.Contains(low, "python") || strings.Contains(low, "jq"))
}

func firstErr(low string) bool {
	if !strings.Contains(low, "error") && !strings.Contains(low, "panic") {
		return false
	}
	return strings.Contains(low, "grep") && (strings.Contains(low, "head") || strings.Contains(low, "tail"))
}

func httpJSON(low string) bool {
	if !strings.Contains(low, "curl ") && !strings.HasPrefix(low, "curl ") {
		return false
	}
	return strings.Contains(low, "jq") || strings.Contains(low, "json")
}

func replaceFile(low string) bool {
	return strings.Contains(low, "sed -i") || strings.Contains(low, "perl -pi")
}

func waitFor(low string) bool {
	return strings.Contains(low, "until ") || strings.Contains(low, "while ! ") ||
		(strings.Contains(low, "sleep") && strings.Contains(low, "grep"))
}
