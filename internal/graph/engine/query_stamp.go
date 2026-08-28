package engine

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	queryStampName   = "last_query_stamp"
	defaultStrictTTL = 1800 * time.Second
	strictTTLEnv     = "SUPEROPEN_HOOK_STRICT_TTL"
)

// RecordQueryStamp marks that a graph query just oriented this repo (no session).
func RecordQueryStamp(repoRoot string) {
	RecordQueryStampFor(repoRoot, "")
}

// RecordQueryStampFor marks that a graph query oriented one agent session.
// Empty sessionID writes the repo-wide stamp used by sessionless hosts.
func RecordQueryStampFor(repoRoot, sessionID string) {
	path := queryStampPathFor(repoRoot, sessionID)
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, []byte(strconv.FormatInt(time.Now().Unix(), 10)+"\n"), 0o644)
}

// QueryStampFresh reports whether a graph query ran within the strict TTL (repo-wide).
func QueryStampFresh(repoRoot string) bool {
	return QueryStampFreshFor(repoRoot, "")
}

// QueryStampFreshFor is true only for the given session. A session id does not
// fall back to the repo-wide stamp — that leak silenced later chats for 30 minutes.
func QueryStampFreshFor(repoRoot, sessionID string) bool {
	if strings.TrimSpace(repoRoot) == "" {
		return false
	}
	return QueryStampFreshAt(queryStampPathFor(repoRoot, sessionID))
}

// QueryStampFreshAt is the same TTL check for any stamp file under .so/db/.
func QueryStampFreshAt(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < queryStampTTL()
}

func queryStampPath(repoRoot string) string {
	return queryStampPathFor(repoRoot, "")
}

func queryStampPathFor(repoRoot, sessionID string) string {
	paths, err := CachePaths(repoRoot)
	if err != nil {
		return ""
	}
	base := filepath.Join(paths.Root, queryStampName)
	sid := sanitizeStampSession(sessionID)
	if sid == "" {
		return base
	}
	return base + "." + sid
}

func sanitizeStampSession(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range id {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}

func queryStampTTL() time.Duration {
	raw := strings.TrimSpace(os.Getenv(strictTTLEnv))
	if raw == "" {
		return defaultStrictTTL
	}
	seconds, err := strconv.ParseFloat(raw, 64)
	if err != nil || seconds < 0 {
		return defaultStrictTTL
	}
	return time.Duration(seconds * float64(time.Second))
}
