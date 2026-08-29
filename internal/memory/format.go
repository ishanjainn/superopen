package memory

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// IndexHit is the cheap search/timeline row: IDs, type, title, tokens.
// Bodies stay behind so memory get (index-then-fetch pattern).
type IndexHit struct {
	ID        int64    `json:"id"`
	Kind      string   `json:"kind"`
	Topic     string   `json:"topic,omitempty"`
	Horizon   string   `json:"horizon,omitempty"`
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

func FormatIndexLine(ep Episode) string {
	title := displayTitle(ep, 48)
	if title == "" {
		title = "(untitled)"
	}
	label := DisplayType(ep)
	if h := NormalizeHorizon(ep.Horizon); h != "" {
		label = h
	}
	if d := episodeDateLabel(ep); d != "" {
		return fmt.Sprintf("#%d  %s  %s  %s", ep.ID, label, d, title)
	}
	return fmt.Sprintf("#%d  %s  %s", ep.ID, label, title)
}

func episodeDateLabel(ep Episode) string {
	raw := strings.TrimSpace(ep.CreatedAt)
	if raw == "" {
		raw = strings.TrimSpace(ep.ValidFrom)
	}
	if raw == "" {
		return ""
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC().Format("2006-01-02")
		}
	}
	if len(raw) >= 10 && raw[4] == '-' && raw[7] == '-' {
		return raw[:10]
	}
	return ""
}

func IndexFromEpisode(ep Episode) IndexHit {
	return IndexHit{
		ID:        ep.ID,
		Kind:      ep.Kind,
		Horizon:   ep.Horizon,
		Topic:     ep.Topic,
		Title:     displayTitle(ep, 80),
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

// IndexRow is a 4-column AXI list row for search/last/timeline.
func IndexRow(idx IndexHit) map[string]any {
	title := strings.TrimSpace(idx.Title)
	if title == "" {
		title = strings.TrimSpace(idx.Topic)
	}
	row := map[string]any{
		"id":     idx.ID,
		"kind":   idx.Kind,
		"title":  title,
		"tokens": idx.Tokens,
	}
	if h := strings.TrimSpace(idx.Horizon); h != "" {
		row["horizon"] = h
	}
	return row
}

func IndexRowsFromHits(hits []Hit) []map[string]any {
	rows := make([]map[string]any, 0, len(hits))
	for _, h := range hits {
		rows = append(rows, IndexRow(IndexFromHit(h)))
	}
	return rows
}

func IndexRowsFromEpisodes(eps []Episode) []map[string]any {
	rows := make([]map[string]any, 0, len(eps))
	for _, ep := range eps {
		rows = append(rows, IndexRow(IndexFromEpisode(ep)))
	}
	return rows
}

func HelpForSearch(hits []Hit) []string {
	if len(hits) == 0 {
		return []string{
			`so memory recall "<cue>"`,
			`so memory search "<cue>"`,
		}
	}
	return []string{
		fmt.Sprintf("so memory get %d --full", hits[0].ID),
		fmt.Sprintf("so memory timeline --around %d", hits[0].ID),
	}
}

// ClippedBodyHint tells the agent recall bodies are windows, not the full episode.
func ClippedBodyHint(hits []Hit) string {
	if len(hits) == 0 {
		return ""
	}
	for _, h := range hits {
		if strings.HasPrefix(h.Text, "…") || strings.HasSuffix(h.Text, "…") || strings.TrimSpace(h.Text) == "" {
			return fmt.Sprintf("bodies clipped — so memory get %d --full for the complete text", hits[0].ID)
		}
	}
	return ""
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

// displayTitle is the CLI/pack headline. Opaque stored titles (import ids,
// uuids) stay on the row; agents see the first informative sentence of the
// body and still cite #id. Capture does not rewrite memory_episodes.title.
func displayTitle(ep Episode, limit int) string {
	title := strings.TrimSpace(ep.Title)
	if !opaqueTitle(title) {
		if t := firstLine(title, limit); t != "" {
			return t
		}
	}
	if h := firstInformative(ep.Text, limit); h != "" {
		return h
	}
	if t := firstLine(title, limit); t != "" {
		return t
	}
	return "(untitled)"
}

func opaqueTitle(title string) bool {
	t := strings.TrimSpace(title)
	if t == "" {
		return true
	}
	for _, r := range t {
		if unicode.IsSpace(r) {
			return false
		}
	}
	if uuidLike(t) {
		return true
	}
	if strings.Contains(t, "_") || strings.ContainsRune(t, ':') {
		return true
	}
	return false
}

func uuidLike(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !isHexByte(c) {
				return false
			}
		}
	}
	return true
}

func isHexByte(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func firstInformative(text string, limit int) string {
	s := strings.TrimSpace(text)
	for s != "" {
		line := s
		rest := ""
		if i := strings.IndexAny(s, "\n\r"); i >= 0 {
			line = strings.TrimSpace(s[:i])
			rest = strings.TrimSpace(s[i+1:])
		}
		s = rest
		if line == "" {
			continue
		}
		if j := sentenceEnd(line); j > 0 {
			line = strings.TrimSpace(line[:j])
		}
		if opaqueTitle(line) {
			continue
		}
		return firstLine(line, limit)
	}
	return ""
}

func sentenceEnd(s string) int {
	for i, r := range s {
		if r == '.' || r == '!' || r == '?' || r == '。' || r == '！' || r == '？' {
			end := i + len(string(r))
			if end >= len(s) {
				return end
			}
			next, _ := utf8.DecodeRuneInString(s[end:])
			if unicode.IsSpace(next) {
				return end
			}
		}
	}
	return 0
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
