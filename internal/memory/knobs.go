package memory

import (
	"strconv"
	"strings"
)

func (s *Store) seedKnobs() error {
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
