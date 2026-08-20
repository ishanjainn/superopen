package memory

import (
	"fmt"
	"strings"
)

func (s *Store) migrate() error {
	cols, err := s.tableColumns("memory_episodes")
	if err != nil {
		return err
	}
	adds := []struct {
		name string
		ddl  string
	}{
		{"community_id", `ALTER TABLE memory_episodes ADD COLUMN community_id TEXT NOT NULL DEFAULT ''`},
		{"centrality", `ALTER TABLE memory_episodes ADD COLUMN centrality REAL NOT NULL DEFAULT 0`},
		{"tier", `ALTER TABLE memory_episodes ADD COLUMN tier TEXT NOT NULL DEFAULT ''`},
		{"never_decay", `ALTER TABLE memory_episodes ADD COLUMN never_decay INTEGER NOT NULL DEFAULT 0`},
		{"tags", `ALTER TABLE memory_episodes ADD COLUMN tags TEXT NOT NULL DEFAULT ''`},
		{"fading", `ALTER TABLE memory_episodes ADD COLUMN fading INTEGER NOT NULL DEFAULT 0`},
	}
	for _, a := range adds {
		if !cols[a.name] {
			if _, err := s.db.Exec(a.ddl); err != nil {
				return fmt.Errorf("migrate episodes %s: %w", a.name, err)
			}
		}
	}
	edgeCols, err := s.tableColumns("memory_edges")
	if err != nil {
		return err
	}
	if !edgeCols["weight"] {
		if _, err := s.db.Exec(`ALTER TABLE memory_edges ADD COLUMN weight REAL NOT NULL DEFAULT 1`); err != nil {
			return fmt.Errorf("migrate edges weight: %w", err)
		}
	}
	if !edgeCols["updated_at"] {
		if _, err := s.db.Exec(`ALTER TABLE memory_edges ADD COLUMN updated_at TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("migrate edges updated_at: %w", err)
		}
	}
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS memory_shapes (
  episode_id INTEGER PRIMARY KEY REFERENCES memory_episodes(id) ON DELETE CASCADE,
  blob BLOB NOT NULL
)`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`UPDATE memory_episodes SET faded=1, faded_at=?, updated_at=? WHERE kind=? AND faded=0`, nowRFC(), nowRFC(), KindTool); err != nil {
		return err
	}
	_ = s.setMeta("schema_version", memorySchemaVersion)
	_ = s.ensureKnobs()
	_ = s.backfillTags()
	return nil
}

func (s *Store) backfillTags() error {
	rows, err := s.db.Query(`SELECT id, uid, text, tags FROM memory_episodes WHERE tags='' LIMIT 400`)
	if err != nil {
		return err
	}
	type row struct {
		id   int64
		uid  string
		text string
		tags string
	}
	var all []row
	for rows.Next() {
		var r row
		if rows.Scan(&r.id, &r.uid, &r.text, &r.tags) == nil {
			all = append(all, r)
		}
	}
	_ = rows.Close()
	for _, r := range all {
		plain := s.openText(r.uid, r.text)
		tags := entityTags(plain)
		if tags == "" {
			continue
		}
		_, _ = s.db.Exec(`UPDATE memory_episodes SET tags=? WHERE id=?`, tags, r.id)
	}
	return nil
}

func (s *Store) tableColumns(table string) (map[string]bool, error) {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		out[name] = true
	}
	return out, rows.Err()
}

func copyColumnList(available map[string]bool, wanted ...string) string {
	var cols []string
	for _, c := range wanted {
		if available[c] {
			cols = append(cols, c)
		}
	}
	return strings.Join(cols, ",")
}
