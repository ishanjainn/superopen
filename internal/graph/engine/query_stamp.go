package engine

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	queryStampName   = "last_query_stamp"
	defaultStrictTTL = 1800 * time.Second
	strictTTLEnv     = "SUPEROPEN_HOOK_STRICT_TTL"
)

// RecordQueryStamp marks that a graph query just oriented the agent for this repo.
func RecordQueryStamp(repoRoot string) {
	path := queryStampPath(repoRoot)
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, []byte(strconv.FormatInt(time.Now().Unix(), 10)+"\n"), 0o644)
}

// QueryStampFresh reports whether a graph query ran within the strict TTL.
func QueryStampFresh(repoRoot string) bool {
	if strings.TrimSpace(repoRoot) == "" {
		return false
	}
	path := queryStampPath(repoRoot)
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < queryStampTTL()
}

func queryStampPath(repoRoot string) string {
	paths, err := CachePaths(repoRoot)
	if err != nil {
		return ""
	}
	return filepath.Join(paths.Root, queryStampName)
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
