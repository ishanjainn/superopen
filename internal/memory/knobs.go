package memory

import (
	"strconv"
	"strings"
)

func (s *Store) ensureKnobs() error {
	// Ranking knobs (rrf_k, fts_keep, …) are not seeded. Search falls
	// through to the Go constants so a product default change takes
	// effect without a store rewrite. memory_meta holds explicit
	// SetProfile overrides only.
	defaults := map[string]string{
		"stale_weight":      "0.5",
		"supersede_window":  "10",
		"pin_weight":        "0.35",
		"recall_budget":     "1500",
		"recency_half_life": "21",
	}
	for k, v := range defaults {
		if _, err := s.db.Exec(`INSERT OR IGNORE INTO memory_meta(key, value) VALUES(?,?)`, k, v); err != nil {
			return err
		}
	}
	return nil
}

var seededRankingKeys = []string{
	"rrf_k", "fts_rrf", "dense_rrf", "dense_rrf_lexical",
	"fts_candidates", "dense_candidates", "fts_keep", "dense_keep",
	"capture_floor", "capture_cap", "edge_half_life",
}

func (s *Store) dropSeededRankingKnobs() error {
	for _, k := range seededRankingKeys {
		if _, err := s.db.Exec(`DELETE FROM memory_meta WHERE key=?`, k); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) knobFloat(key string, def float64) float64 {
	raw, err := s.meta(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return def
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return def
	}
	return n
}

func (s *Store) knobInt(key string, def int) int {
	raw, err := s.meta(key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return def
	}
	return n
}

func (s *Store) Profile() map[string]string {
	out := map[string]string{}
	rows, err := s.db.Query(`SELECT key, value FROM memory_meta WHERE key NOT LIKE 'schema%' AND key NOT LIKE 'embedder%'`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if rows.Scan(&k, &v) == nil {
			out[k] = v
		}
	}
	return out
}

func (s *Store) SetProfile(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO memory_meta(key, value) VALUES(?,?)
ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}
