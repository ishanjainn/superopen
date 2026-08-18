package session

import (
	"os/exec"
	"strings"
	"time"
)

const (
	trailerSession     = "SO-Session"
	trailerAttribution = "SO-Attribution"
)

// BackfillFromGitLog scans recent commits for SO-Session trailers matching this session.
func BackfillFromGitLog(meta *Meta, repoRoot string, limit int) {
	if meta == nil || meta.ID == "" {
		return
	}
	if limit <= 0 {
		limit = 50
	}
	cmd := exec.Command("git", "-C", repoRoot, "log", "-n", itoa(limit), "--format=%H%x00%B%x00")
	out, err := cmd.Output()
	if err != nil {
		return
	}
	parts := strings.Split(string(out), "\x00")
	for i := 0; i+1 < len(parts); i += 2 {
		sha := strings.TrimSpace(parts[i])
		msg := parts[i+1]
		if sha == "" {
			continue
		}
		sid, attr := parseCommitTrailers(msg)
		if sid != meta.ID {
			continue
		}
		subject := strings.SplitN(strings.TrimSpace(msg), "\n", 2)[0]
		MergeTrailerSession(meta, sha, subject, time.Now().UTC())
		if attr != "" && meta.Attribution == nil {
			meta.Attribution = &AttributionSummary{Display: attr}
		}
	}
}

func parseCommitTrailers(msg string) (sessionID, attribution string) {
	for _, line := range strings.Split(msg, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, trailerSession+":") {
			sessionID = strings.TrimSpace(strings.TrimPrefix(line, trailerSession+":"))
		}
		if strings.HasPrefix(line, trailerAttribution+":") {
			attribution = strings.TrimSpace(strings.TrimPrefix(line, trailerAttribution+":"))
		}
	}
	return sessionID, attribution
}

func itoa(n int) string {
	if n <= 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
