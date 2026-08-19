package session_test

import (
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/session"
	"github.com/ishanjainn/superopen/internal/session/trace"
)

func TestApplyVCSFromSpans(t *testing.T) {
	meta := &session.Meta{ID: "s1"}
	session.ApplyVCSFromSpans(meta, []trace.Span{
		{
			Name:           "coding_agent.git.commit",
			StartTimeUnixN: time.Now().UnixNano(),
			Attributes: map[string]string{
				"coding_agent.git.commit.sha":     "abc123def",
				"coding_agent.git.commit.message": "fix stuff",
				"vcs.ref.head.name":               "feature/x",
			},
		},
		{
			Name: "coding_agent.git.pull_request",
			Attributes: map[string]string{
				"coding_agent.git.pull_request.url":    "https://github.com/o/r/pull/9",
				"coding_agent.git.pull_request.number": "9",
				"coding_agent.git.pull_request.title":  "Fix",
			},
		},
	})
	if meta.Branch != "feature/x" {
		t.Fatalf("branch=%s", meta.Branch)
	}
	if len(meta.Commits) != 1 || meta.Commits[0].SHA != "abc123def" {
		t.Fatalf("commits=%+v", meta.Commits)
	}
	if len(meta.PullRequests) != 1 || meta.PullRequests[0].Number != 9 {
		t.Fatalf("prs=%+v", meta.PullRequests)
	}
}
