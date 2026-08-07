package session

import (
	"strconv"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/tracestore"
)

// ApplyVCSFromSpans folds commit/PR/branch attrs from spans into meta.
func ApplyVCSFromSpans(meta *Meta, spans []tracestore.Span) {
	if meta == nil {
		return
	}
	commitsBySHA := map[string]CommitRef{}
	for _, c := range meta.Commits {
		if c.SHA != "" {
			commitsBySHA[c.SHA] = c
		}
	}
	prsByKey := map[string]PRRef{}
	for _, pr := range meta.PullRequests {
		key := pr.URL
		if key == "" {
			key = strconv.Itoa(pr.Number)
		}
		if key != "" {
			prsByKey[key] = pr
		}
	}

	for _, sp := range spans {
		at := time.Unix(0, sp.StartTimeUnixN).UTC()
		if meta.Branch == "" {
			if b := sp.Attributes["vcs.ref.head.name"]; b != "" {
				meta.Branch = b
			}
		}
		if meta.HeadSHA == "" {
			if h := sp.Attributes["vcs.ref.head.revision"]; h != "" {
				meta.HeadSHA = h
			}
		}
		if meta.BaseSHA == "" && meta.HeadSHA != "" {
			meta.BaseSHA = meta.HeadSHA
		}

		name := sp.Name
		if name == "coding_agent.git.commit" || sp.Attributes["coding_agent.git.commit.sha"] != "" {
			sha := sp.Attributes["coding_agent.git.commit.sha"]
			if sha == "" {
				continue
			}
			msg := sp.Attributes["coding_agent.git.commit.message"]
			commitsBySHA[sha] = CommitRef{SHA: sha, Message: msg, At: at}
			meta.HeadSHA = sha
		}
		if name == "coding_agent.git.pull_request" || sp.Attributes["coding_agent.git.pull_request.url"] != "" ||
			sp.Attributes["coding_agent.git.pull_request.number"] != "" {
			url := sp.Attributes["coding_agent.git.pull_request.url"]
			title := sp.Attributes["coding_agent.git.pull_request.title"]
			num := 0
			if n := sp.Attributes["coding_agent.git.pull_request.number"]; n != "" {
				num, _ = strconv.Atoi(n)
			}
			key := url
			if key == "" {
				key = strconv.Itoa(num)
			}
			if key == "" {
				continue
			}
			prsByKey[key] = PRRef{URL: url, Number: num, Title: title, At: at}
		}
	}

	meta.Commits = meta.Commits[:0]
	for _, c := range commitsBySHA {
		meta.Commits = append(meta.Commits, c)
	}
	meta.PullRequests = meta.PullRequests[:0]
	for _, pr := range prsByKey {
		meta.PullRequests = append(meta.PullRequests, pr)
	}
}

// MergeTrailerSession records that a commit trailer pointed at this session.
func MergeTrailerSession(meta *Meta, sha, message string, at time.Time) {
	if meta == nil || sha == "" {
		return
	}
	for _, c := range meta.Commits {
		if strings.EqualFold(c.SHA, sha) || strings.HasPrefix(strings.ToLower(c.SHA), strings.ToLower(sha)) {
			return
		}
	}
	meta.Commits = append(meta.Commits, CommitRef{SHA: sha, Message: message, At: at})
	meta.HeadSHA = sha
}
