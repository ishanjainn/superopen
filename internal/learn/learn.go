// Package learn mines corrections from session transcripts into lessons/skills.
package learn

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/llm"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/recommend"
)

var correctionRes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(always|never|don't|do not|prefer|remember to|make sure to)\b.{10,200}`),
	regexp.MustCompile(`(?i)\bso learn\b[:\s]+(.+)`),
	regexp.MustCompile(`(?i)/so remember\b[:\s]+(.+)`),
}

// MineTranscript extracts correction-like lines and writes lessons + recommendations.
func MineTranscript(paths harness.Paths, sessionID string, transcriptLines []string, completer llm.Completer) ([]memory.Lesson, []recommend.Recommendation, error) {
	store := memory.NewStore(paths)
	_ = store.Ensure()
	var lessons []memory.Lesson
	seen := map[string]bool{}
	for _, line := range transcriptLines {
		line = strings.TrimSpace(line)
		if len(line) < 12 || len(line) > 500 {
			continue
		}
		for _, re := range correctionRes {
			m := re.FindString(line)
			if m == "" {
				continue
			}
			key := strings.ToLower(m)
			if seen[key] {
				continue
			}
			seen[key] = true
			l := memory.Lesson{
				Text:          m,
				Scope:         "workspace",
				Confidence:    0.85,
				SourceSession: sessionID,
				CreatedAt:     time.Now().UTC(),
			}
			if err := store.AddLesson(l, memory.ModePersistent); err != nil {
				continue
			}
			lessons = append(lessons, l)
		}
	}
	var recs []recommend.Recommendation
	now := time.Now().UTC()
	for _, l := range lessons {
		body := heuristicSkillBody(l.Text)
		if completer != nil && completer.Available() {
			if out, err := completer.Complete(
				"Write a short Superopen SKILL.md markdown for this lesson. No fences.",
				l.Text,
			); err == nil && strings.TrimSpace(out) != "" {
				body = strings.TrimSpace(out)
			}
		}
		slug := slugify(l.Text)
		path := filepath.Join(paths.SkillsDir, slug+".md")
		recs = append(recs, recommend.Recommendation{
			ID:           fmt.Sprintf("rec_learn_%d_%s", now.UnixNano(), slug),
			Fingerprint:  recommend.FingerprintKey("skill", path, slug),
			SessionID:    sessionID,
			Type:         "skill",
			Title:        "Skill from correction: " + truncate(l.Text, 60),
			Rationale:    "Mined from session transcript correction signal.",
			Why:          "Persisting this as a named skill keeps the next session from repeating the same mistake.",
			Evidence:     []string{l.Text},
			ProposedPath: path,
			ProposedBody: body,
			Status:       "pending",
			CreatedAt:    now,
		})
	}
	if len(recs) > 0 {
		_, _ = recommend.MergePending(paths, recs)
	}
	return lessons, recs, nil
}

// MineSessionFile reads .so/sessions/<id>/transcript.jsonl loosely as text lines.
func MineSessionFile(paths harness.Paths, sessionID string, completer llm.Completer) ([]memory.Lesson, []recommend.Recommendation, error) {
	p := filepath.Join(paths.SessionDir(sessionID), "transcript.jsonl")
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	lines := strings.Split(string(data), "\n")
	return MineTranscript(paths, sessionID, lines, completer)
}

func heuristicSkillBody(text string) string {
	return fmt.Sprintf("# Skill from correction\n\n## Rule\n\n%s\n\n## Checklist\n\n1. Apply this rule before finishing related work.\n2. Prefer `.so/memory/active-context.md` and graph query over rediscovery.\n", text)
}

func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r == ' ' || r == '-' || r == '_' {
			b.WriteByte('-')
		}
		if b.Len() > 40 {
			break
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "correction"
	}
	return out
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
