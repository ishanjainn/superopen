package memory

import (
	"strconv"
	"strings"
)

func (s *Store) ensureKnobs() error {
	defaults := map[string]string{
		"capture_floor":     "12",
		"capture_cap":       "8000",
		"stale_weight":      "0.5",
		"supersede_window":  "10",
		"lex_fusion":        "0.35",
		"cosine_weight":     "0.6",
		"centrality_weight": "0.4",
		"shape_fusion":      "0.25",
		"pin_weight":        "0.35",
		"recall_budget":     "1500",
		"edge_half_life":    "90",
		"recency_half_life": "21",
		"english_only":      "1",
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
