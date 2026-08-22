package memory

import (
	"fmt"
	"strconv"
	"strings"
)

// IndexHit is the cheap search/timeline row: IDs, type, title, tokens.
// Bodies stay behind so memory get (index-then-fetch pattern).
type IndexHit struct {
	ID        int64    `json:"id"`
	Kind      string   `json:"kind"`
	Topic     string   `json:"topic,omitempty"`
	Title     string   `json:"title"`
	Tokens    int      `json:"tokens"`
	Score     float64  `json:"score,omitempty"`
	SessionID string   `json:"session_id,omitempty"`
	Files     []string `json:"files,omitempty"`
}

func DisplayType(ep Episode) string {
	if t := strings.TrimSpace(ep.Topic); t != "" {
		return t
	}
	return ep.Kind
}

func FormatHit(ep Episode) string {
	title := compactLine(ep)
	if title == "" {
		title = "(untitled)"
	}
	return fmt.Sprintf("MEM #%d %s %q ~%dt", ep.ID, DisplayType(ep), title, ep.Tokens)
}

func FormatIndexLine(ep Episode) string {
	title := firstLine(ep.Title, 48)
	if title == "" {
		title = "(untitled)"
	}
	return fmt.Sprintf("#%d  %s  %s  ~%dt", ep.ID, DisplayType(ep), title, ep.Tokens)
}

func IndexFromEpisode(ep Episode) IndexHit {
	return IndexHit{
		ID:        ep.ID,
		Kind:      ep.Kind,
		Topic:     ep.Topic,
		Title:     firstLine(ep.Title, 80),
		Tokens:    ep.Tokens,
		Score:     ep.Score,
		SessionID: ep.SessionID,
		Files:     ep.Files,
	}
}

func IndexFromHit(h Hit) IndexHit {
	idx := IndexFromEpisode(h.Episode)
	idx.Score = h.Score
	return idx
}

func HelpForSearch(hits []Hit) []string {
	if len(hits) == 0 {
		return []string{
			`so memory recall "<cue>"`,
			`so graph query "<question>"`,
		}
	}
	return []string{
		fmt.Sprintf("so memory get %d", hits[0].ID),
		fmt.Sprintf("so memory timeline --around %d", hits[0].ID),
	}
}

func HelpForGet(eps []Episode) []string {
	if len(eps) == 0 {
		return []string{`so memory search "<cue>"`}
	}
	hints := []string{`so memory recall "<cue>"`}
	if !eps[0].Faded && eps[0].ValidTo == "" {
		hints = append(hints, fmt.Sprintf("so memory contradict %d --text \"…\"", eps[0].ID))
	}
	return hints
}

func ParseIDs(args []string) ([]int64, error) {
	var ids []int64
	for _, raw := range args {
		raw = strings.TrimSpace(strings.TrimPrefix(raw, "#"))
		if raw == "" {
			continue
		}
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("invalid memory id %q", raw)
		}
		ids = append(ids, n)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("memory id required")
	}
	return ids, nil
}
