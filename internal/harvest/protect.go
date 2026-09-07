package harvest

import (
	"path/filepath"
	"strings"
)

const (
	sentinelBegin = "<!-- BEGIN SUPEROPEN -->"
	sentinelEnd   = "<!-- END SUPEROPEN -->"
	skillTripwire = "If `.so/` is missing, still run **one** `graph query`."
)

func ProtectedPath(rel string) bool {
	slash := filepath.ToSlash(strings.ToLower(rel))
	if strings.HasPrefix(slash, ".so/") {
		return true
	}
	base := filepath.Base(slash)
	if strings.Contains(slash, "/hooks/") && (base == "hooks.json" || strings.HasSuffix(base, ".json")) {
		return true
	}
	if strings.Contains(slash, "/skills/so/") || strings.Contains(slash, "/skills/superopen/") {
		return true
	}
	return false
}

func ProtectedContent(body string) bool {
	if strings.Contains(body, skillTripwire) {
		return true
	}
	if strings.Contains(body, sentinelBegin) && strings.Contains(body, sentinelEnd) {
		return true
	}
	low := strings.ToLower(body)
	if strings.Contains(low, "so sessions hook") || strings.Contains(low, "so coding hook") || strings.Contains(low, "so graph query") && strings.Contains(low, "hook") {
		if strings.Contains(body, `"hooks"`) || strings.Contains(body, "PreToolUse") {
			return true
		}
	}
	return false
}

func SentinelBlocks(body string) []string {
	var blocks []string
	rest := body
	for {
		start := strings.Index(rest, sentinelBegin)
		if start < 0 {
			break
		}
		end := strings.Index(rest[start:], sentinelEnd)
		if end < 0 {
			break
		}
		end = start + end + len(sentinelEnd)
		blocks = append(blocks, rest[start:end])
		rest = rest[end:]
	}
	return blocks
}

func SentinelsChanged(before, after string) bool {
	a := SentinelBlocks(before)
	b := SentinelBlocks(after)
	if len(a) != len(b) {
		return true
	}
	for i := range a {
		if a[i] != b[i] {
			return true
		}
	}
	if strings.Contains(before, skillTripwire) && !strings.Contains(after, skillTripwire) {
		return true
	}
	return false
}
