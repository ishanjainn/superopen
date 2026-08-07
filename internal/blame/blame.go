// Package blame maps source lines back to Superopen sessions via git blame
// and SO-Session trailers / materialized commit links.
package blame

import (
	"bufio"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/githooks"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/session"
)

// LineInfo is one annotated source line.
type LineInfo struct {
	Line       int    `json:"line"`
	Content    string `json:"content,omitempty"`
	CommitSHA  string `json:"commit_sha,omitempty"`
	Author     string `json:"author,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	Prompt     string `json:"prompt,omitempty"`
	Vendor     string `json:"vendor,omitempty"`
}

// WhyResult is the answer for `so why file:line`.
type WhyResult struct {
	File      string `json:"file"`
	Line      int    `json:"line"`
	CommitSHA string `json:"commit_sha"`
	SessionID string `json:"session_id,omitempty"`
	Prompt    string `json:"prompt,omitempty"`
	Vendor    string `json:"vendor,omitempty"`
	Title     string `json:"title,omitempty"`
	Message   string `json:"message,omitempty"`
}

// File annotates lines of a file with session ids when commits carry SO-Session.
func File(repoRoot, relPath string, paths harness.Paths) ([]LineInfo, error) {
	cmd := exec.Command("git", "-C", repoRoot, "blame", "--line-porcelain", "--", relPath)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git blame: %w", err)
	}
	ss := session.NewStore(paths)
	sessionByCommit := map[string]string{}

	var lines []LineInfo
	var cur LineInfo
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		line := sc.Text()
		if len(line) > 40 && line[40] == ' ' && isHex(line[:40]) {
			cur = LineInfo{CommitSHA: line[:40]}
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				cur.Line, _ = strconv.Atoi(fields[2])
			}
			continue
		}
		if strings.HasPrefix(line, "author ") {
			cur.Author = strings.TrimPrefix(line, "author ")
			continue
		}
		if strings.HasPrefix(line, "\t") {
			cur.Content = strings.TrimPrefix(line, "\t")
			sid := sessionByCommit[cur.CommitSHA]
			if sid == "" {
				sid = sessionIDForCommit(repoRoot, cur.CommitSHA, ss)
				sessionByCommit[cur.CommitSHA] = sid
			}
			cur.SessionID = sid
			if sid != "" {
				if m, err := ss.Get(sid); err == nil {
					cur.Vendor = m.Vendor
					cur.Prompt = m.PromptPreview
				}
			}
			lines = append(lines, cur)
			cur = LineInfo{}
		}
	}
	return lines, sc.Err()
}

// Why resolves file:line to session context.
func Why(repoRoot, file string, line int, paths harness.Paths) (WhyResult, error) {
	infos, err := File(repoRoot, file, paths)
	if err != nil {
		return WhyResult{}, err
	}
	for _, li := range infos {
		if li.Line == line {
			res := WhyResult{
				File:      file,
				Line:      line,
				CommitSHA: li.CommitSHA,
				SessionID: li.SessionID,
				Prompt:    li.Prompt,
				Vendor:    li.Vendor,
			}
			if li.SessionID != "" {
				if m, err := session.NewStore(paths).Get(li.SessionID); err == nil {
					res.Title = m.Title
					if res.Prompt == "" {
						res.Prompt = m.PromptPreview
					}
				}
			}
			if res.SessionID == "" {
				res.Message = "no SO-Session trailer or session link for this commit"
			}
			return res, nil
		}
	}
	return WhyResult{}, fmt.Errorf("line %d not found in blame for %s", line, file)
}

func sessionIDForCommit(repoRoot, sha string, ss *session.Store) string {
	cmd := exec.Command("git", "-C", repoRoot, "log", "-1", "--format=%B", sha)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	sid, _ := githooks.ParseTrailers(string(out))
	if sid != "" {
		return sid
	}
	// Fall back: scan local sessions for matching commit SHA.
	list, err := ss.List()
	if err != nil {
		return ""
	}
	short := sha
	if len(short) > 12 {
		short = short[:12]
	}
	for _, m := range list {
		if strings.HasPrefix(m.HeadSHA, short) || strings.EqualFold(m.HeadSHA, sha) {
			return m.ID
		}
		for _, c := range m.Commits {
			if strings.HasPrefix(c.SHA, short) || strings.EqualFold(c.SHA, sha) {
				return m.ID
			}
		}
	}
	_ = time.Now()
	return ""
}

func isHex(s string) bool {
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return len(s) > 0
}
