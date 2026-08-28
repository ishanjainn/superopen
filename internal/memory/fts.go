package memory

import (
	"strings"
)

const plaintextFTSDDL = `CREATE VIRTUAL TABLE memory_episodes_fts USING fts5(
  title, text, files, tool_name,
  tokenize='unicode61 remove_diacritics 2'
)`

const ftsDeleteTrigger = `CREATE TRIGGER IF NOT EXISTS memory_episodes_ad AFTER DELETE ON memory_episodes BEGIN
  DELETE FROM memory_episodes_fts WHERE rowid = old.id;
END;`

// ensurePlaintextFTS migrates content= / ciphertext FTS to a standalone
// index over plaintext so lexical search can see diary bodies.
func (s *Store) ensurePlaintextFTS() error {
	_, _ = s.db.Exec(`DROP TRIGGER IF EXISTS memory_episodes_ai`)
	_, _ = s.db.Exec(`DROP TRIGGER IF EXISTS memory_episodes_au`)
	var sql string
	err := s.db.QueryRow(`SELECT sql FROM sqlite_master WHERE name='memory_episodes_fts' AND sql IS NOT NULL`).Scan(&sql)
	needRebuild := err != nil || strings.Contains(strings.ToLower(sql), "content=")
	if !needRebuild {
		needRebuild = s.ftsLooksSealed()
	}
	if !needRebuild {
		_, _ = s.db.Exec(ftsDeleteTrigger)
		return nil
	}
	_, _ = s.db.Exec(`DROP TRIGGER IF EXISTS memory_episodes_ad`)
	if _, err := s.db.Exec(`DROP TABLE IF EXISTS memory_episodes_fts`); err != nil {
		return err
	}
	if _, err := s.db.Exec(plaintextFTSDDL); err != nil {
		return err
	}
	if _, err := s.db.Exec(ftsDeleteTrigger); err != nil {
		return err
	}
	return s.rebuildFTS()
}

func (s *Store) ftsLooksSealed() bool {
	var sample string
	err := s.db.QueryRow(`SELECT text FROM memory_episodes_fts WHERE text != '' LIMIT 1`).Scan(&sample)
	if err != nil {
		return false
	}
	return strings.HasPrefix(sample, encPrefix)
}

func (s *Store) rebuildFTS() error {
	if _, err := s.db.Exec(`DELETE FROM memory_episodes_fts`); err != nil {
		return err
	}
	type row struct {
		id     int64
		uid    string
		title  string
		stored string
		files  string
		tool   string
	}
	var all []row
	rows, err := s.db.Query(`SELECT id, uid, title, text, files, tool_name FROM memory_episodes`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var r row
		if rows.Scan(&r.id, &r.uid, &r.title, &r.stored, &r.files, &r.tool) != nil {
			continue
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, r := range all {
		if err := s.writeFTS(r.id, r.title, s.openText(r.uid, r.stored), r.files, r.tool); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) writeFTS(id int64, title, plain, files, tool string) error {
	if id <= 0 {
		return nil
	}
	_, _ = s.db.Exec(`DELETE FROM memory_episodes_fts WHERE rowid=?`, id)
	_, err := s.db.Exec(
		`INSERT INTO memory_episodes_fts(rowid, title, text, files, tool_name) VALUES(?,?,?,?,?)`,
		id, title, plain, files, tool,
	)
	return err
}
