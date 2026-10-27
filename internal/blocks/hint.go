package blocks

import (
	"strings"
)

// Hint is one line when a shell command is starting to retype a built-in.
// It is empty for graph, memory, and session commands, and for plain greps.
// The caller must not deny or rewrite the command.
func Hint(cmd string) string {
	fields := strings.Fields(cmd)
	low := strings.ToLower(strings.Join(fields, " "))
	if low == "" || mentionsSO(low) {
		return ""
	}
	switch {
	case testTail(low):
		return "use so blocks run test-summary"
	case jsonPeek(low):
		return "use so blocks run json-peek"
	case jsonlStats(low):
		return "use so blocks run jsonl-stats"
	case firstErr(low):
		return "use so blocks run first-error"
	case httpJSON(low):
		return "use so blocks run http-json"
	case replaceFile(low):
		return "use so blocks run replace-in-file"
	case waitFor(low):
		return "use so blocks run wait-for"
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
