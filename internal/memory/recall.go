package memory

import (
	"encoding/json"
	"strings"
)

type RecallResult struct {
	Hits         []Hit `json:"hits"`
	AntiHits     []Hit `json:"anti_hits"`
	BudgetTokens int   `json:"budget_tokens"`
}

func (s *Store) Recall(query string, budget int) (RecallResult, error) {
	return s.RecallFilter(SearchFilter{Query: query, Limit: 40}, budget)
}

func (s *Store) RecallFilter(filter SearchFilter, budget int) (RecallResult, error) {
	if budget <= 0 {
		budget = s.knobInt("recall_budget", recallBudgetTok)
	}
	if filter.Limit <= 0 {
		filter.Limit = 40
	}
	hits, err := s.Search(filter)
	if err != nil {
		return RecallResult{BudgetTokens: budget}, err
	}
	stale, _, neighbors := s.contradictMaps()
	hist := historicalCue(filter.Query) || filter.AsOf != ""
	byID := map[int64]Hit{}
	for _, h := range hits {
		byID[h.ID] = h
	}
	var current, anti []Hit
	for _, h := range hits {
		if stale[h.ID] && !hist {
			anti = append(anti, h)
			continue
		}
		current = append(current, h)
	}
	for _, h := range current {
		for _, nbr := range neighbors[h.ID] {
			if _, ok := byID[nbr]; ok {
				continue
			}
			ep, err := s.Get(nbr)
			if err != nil || ep.Kind == KindTool {
				continue
			}
			hit := Hit{Episode: ep, Snippet: firstLine(ep.Text, 140)}
			byID[nbr] = hit
			anti = append(anti, hit)
		}
	}
	if len(anti) > 3 {
		anti = anti[:3]
	}
	current = trimToBudget(current, budget)
	return RecallResult{Hits: current, AntiHits: anti, BudgetTokens: budget}, nil
}

func (s *Store) TemporalRecall(query, asOf, changedSince string, budget int) (RecallResult, error) {
	return s.RecallFilter(SearchFilter{Query: query, AsOf: asOf, ChangedSince: changedSince, Limit: 40}, budget)
}

func (s *Store) Reinforce(id int64) error {
	_, err := s.db.Exec(`UPDATE memory_edges SET weight=weight+1, updated_at=? WHERE source_id=? OR target_id=?`, nowRFC(), id, id)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE memory_episodes SET centrality=centrality+0.05 WHERE id=?`, id)
	return err
}

func (s *Store) Curiosity(limit int) ([]Hit, error) {
	if limit <= 0 {
		limit = 8
	}
	rows, err := s.db.Query(`SELECT `+episodeCols+` FROM memory_episodes WHERE faded=0 AND kind NOT IN ('tool','working')
ORDER BY centrality ASC, created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	eps, err := s.scanEpisodes(rows)
	if err != nil {
		return nil, err
	}
	hits := make([]Hit, 0, len(eps))
	for _, ep := range eps {
		hits = append(hits, Hit{Episode: ep, Snippet: firstLine(ep.Text, 140)})
	}
	return hits, nil
}

func (s *Store) Patterns(limit int) ([]string, error) {
	if limit <= 0 {
		limit = 8
	}
	rows, err := s.db.Query(`SELECT t.label, COUNT(*) FROM memory_topics t
JOIN memory_episodes e ON e.community_id=CAST(t.community AS TEXT)
WHERE e.faded=0 GROUP BY t.id ORDER BY COUNT(*) DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var title string
		var n int
		if rows.Scan(&title, &n) == nil && title != "" {
			out = append(out, title)
		}
	}
	return out, rows.Err()
}

func (s *Store) When(query string, limit int) ([]Hit, error) {
	res, err := s.TemporalRecall(query, "", "", 0)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 12
	}
	hits := res.Hits
	filtered := hits[:0]
	for _, h := range hits {
		if h.Kind == KindTool {
			continue
		}
		if strings.TrimSpace(h.CreatedAt) == "" {
			continue
		}
		filtered = append(filtered, h)
	}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered, nil
}

func (s *Store) Recent(limit int) ([]Episode, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`SELECT `+episodeCols+` FROM memory_episodes WHERE faded=0 AND kind != 'tool' ORDER BY created_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	return s.scanEpisodes(rows)
}

func (s *Store) Events(limit int) ([]TimelineBucket, error) {
	return s.Timeline(limit)
}

func (s *Store) Consolidate() error {
	return s.Sleep()
}

func (s *Store) MapJSON() (string, error) {
	layout, err := s.Layout(400)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(layout)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
