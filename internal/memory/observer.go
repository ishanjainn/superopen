package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/agent/headless"
)

const observeTimeout = 30 * time.Second

type ObserveResult struct {
	SessionID string `json:"session_id"`
	Inserted  int    `json:"inserted"`
	Provider  string `json:"provider,omitempty"`
	Skipped   string `json:"skipped,omitempty"`
}

// ObserveSession writes additional typed observation rows for a session.
// KindPrompt text is never rewritten. Heuristic types always run for
// non-questions. Headless observe is not auto-run (same as distill: opt-in CLI only).
func ObserveSession(root, sessionID string) (ObserveResult, error) {
	res := ObserveResult{SessionID: sessionID}
	store, err := OpenRoot(root)
	if err != nil {
		res.Skipped = err.Error()
		return res, nil
	}
	defer store.Close()
	n, err := store.observeHeuristic(sessionID)
	if err != nil {
		return res, err
	}
	res.Inserted += n
	res.Provider = "heuristic"
	return res, nil
}

func (s *Store) observeHeuristic(sessionID string) (int, error) {
	rows, err := s.db.Query(`SELECT `+episodeCols+` FROM memory_episodes WHERE session_id=? AND kind=? AND faded=0 ORDER BY id`, sessionID, KindPrompt)
	if err != nil {
		return 0, err
	}
	prompts, err := s.scanEpisodes(rows)
	if err != nil {
		return 0, err
	}
	inserted := 0
	for _, p := range prompts {
		if looksLikeQuestion(p.Title + " " + p.Text) {
			continue
		}
		if !isCompressible(p) && p.Kind == KindPrompt {
			// keep verbatim; write an extra observation beside it
		}
		title := firstLine(p.Title, 72)
		topic := p.Topic
		if topic == "" {
			topic = heuristicTopic(p.Title + " " + p.Text)
		}
		facts := []string{firstLine(p.Text, 140)}
		ep := Episode{
			UID:         contentHashUID(sessionID, KindObservation, topic, title),
			ContentHash: contentHashUID(sessionID, KindObservation, topic, title),
			SessionID:   sessionID,
			Kind:        KindObservation,
			Source:      SourceObserver,
			Title:       title,
			Text:        firstLine(p.Text, 240),
			Files:       p.Files,
			Topic:       topic,
			Facts:       facts,
			Narrative:   "",
			Concepts:    splitConcepts(p.Tags),
			Tokens:      EstimateTokens(title),
			Tier:        tierSemantic,
			CreatedAt:   p.CreatedAt,
		}
		id, added, err := s.storeEpisode(ep)
		if err != nil {
			return inserted, err
		}
		if added && id != 0 {
			inserted++
			_ = s.addEdge(id, p.ID, EdgeRolledUpFrom)
		}
	}
	return inserted, nil
}

func (s *Store) observeHeadless(root, sessionID string, provider headless.Provider) (int, error) {
	rows, err := s.db.Query(`SELECT `+episodeCols+` FROM memory_episodes WHERE session_id=? AND kind=? AND faded=0 ORDER BY id LIMIT 24`, sessionID, KindPrompt)
	if err != nil {
		return 0, err
	}
	prompts, err := s.scanEpisodes(rows)
	if err != nil {
		return 0, err
	}
	titles := make([]string, 0, len(prompts))
	for _, p := range prompts {
		line := fmt.Sprintf("- %s [%s] %s", firstLine(p.Title, 72), p.Topic, strings.Join(p.Files, ","))
		if preview := firstLine(p.Text, 160); preview != "" && preview != firstLine(p.Title, 72) {
			line += "\n  " + preview
		}
		titles = append(titles, line)
	}
	if len(titles) == 0 {
		return 0, nil
	}
	prompt := "Extract typed observations from these Superopen session titles. " +
		"Output a JSON array of objects with keys type, title, facts, narrative, concepts. " +
		"type must be one of decision, bugfix, feature, refactor, discovery, change. " +
		"facts is a string array. Do not rewrite verbatim user prompts; write additional observations only.\n\n" +
		strings.Join(titles, "\n")
	ctx, cancel := context.WithTimeout(context.Background(), observeTimeout)
	defer cancel()
	out, err := headless.Run(ctx, provider, prompt)
	if err != nil {
		return 0, err
	}
	var parsed []struct {
		Type      string   `json:"type"`
		Title     string   `json:"title"`
		Facts     []string `json:"facts"`
		Narrative string   `json:"narrative"`
		Concepts  []string `json:"concepts"`
	}
	body := extractJSONArray(out)
	if json.Unmarshal([]byte(body), &parsed) != nil {
		return 0, nil
	}
	inserted := 0
	for _, item := range parsed {
		topic := canonicalObservationType(item.Type)
		title := firstLine(item.Title, 72)
		if title == "" {
			continue
		}
		ep := Episode{
			UID:         contentHashUID(sessionID, KindObservation, "llm", topic, title),
			ContentHash: contentHashUID(sessionID, KindObservation, "llm", topic, title),
			SessionID:   sessionID,
			Kind:        KindObservation,
			Source:      SourceHeadless,
			Title:       title,
			Text:        strings.Join(item.Facts, "\n"),
			Topic:       topic,
			Facts:       item.Facts,
			Narrative:   item.Narrative,
			Concepts:    item.Concepts,
			Tokens:      EstimateTokens(title + " " + strings.Join(item.Facts, " ")),
			Tier:        tierSemantic,
		}
		id, added, err := s.storeEpisode(ep)
		if err != nil {
			return inserted, err
		}
		if added && id != 0 {
			inserted++
		}
	}
	return inserted, nil
}

func looksLikeQuestion(text string) bool {
	s := strings.TrimSpace(text)
	if s == "" {
		return false
	}
	l := strings.ToLower(s)
	trimmed := strings.TrimRight(l, ".!")
	if strings.HasSuffix(trimmed, "?") {
		return true
	}
	first := l
	if i := strings.IndexAny(l, " \t\n"); i > 0 {
		first = l[:i]
	}
	first = strings.Trim(first, ":,")
	switch first {
	case "who", "what", "where", "when", "why", "how", "which", "do", "did", "can", "should", "is", "are", "was", "were":
		return true
	}
	for _, cue := range []string{
		"what did we decide",
		"did we decide",
		"don't guess",
		"dont guess",
		"do not guess",
		"last time",
	} {
		if strings.Contains(l, cue) {
			return true
		}
	}
	return false
}

func heuristicTopic(text string) string {
	if looksLikeQuestion(text) {
		return ObservationChange
	}
	l := strings.ToLower(text)
	switch {
	case containsAny(l, "bug", "fix", "timeout", "crash", "error", "panic", "fail"):
		return ObservationBugfix
	case containsAny(l, "decid", "jwt", "expir", "policy", "auth uses", "we will"):
		return ObservationDecision
	case containsAny(l, "refactor", "rename", "extract", "cleanup"):
		return ObservationRefactor
	case containsAny(l, "feature", "implement", "add ", "new "):
		return ObservationFeature
	case containsAny(l, "found", "discover", "learned", "turns out"):
		return ObservationDiscovery
	default:
		return ObservationChange
	}
}

func canonicalObservationType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case ObservationDecision, ObservationBugfix, ObservationFeature, ObservationRefactor, ObservationDiscovery, ObservationChange:
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ObservationChange
	}
}

func containsAny(hay string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(hay, n) {
			return true
		}
	}
	return false
}

func splitConcepts(tags string) []string {
	return splitCSV(tags)
}

func extractJSONArray(s string) string {
	s = strings.TrimSpace(s)
	i := strings.Index(s, "[")
	j := strings.LastIndex(s, "]")
	if i >= 0 && j > i {
		return s[i : j+1]
	}
	return s
}
