package memory

import (
	"strings"
)

func (s *Store) ensurePassagesSchema() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS memory_passages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  episode_id INTEGER NOT NULL REFERENCES memory_episodes(id) ON DELETE CASCADE,
  ord INTEGER NOT NULL,
  text TEXT NOT NULL DEFAULT '',
  embedder_id TEXT NOT NULL,
  dimensions INTEGER NOT NULL,
  quantization TEXT NOT NULL,
  vector BLOB NOT NULL,
  UNIQUE(episode_id, ord)
);
CREATE INDEX IF NOT EXISTS memory_passages_episode ON memory_passages(episode_id);
`)
	return err
}

func (s *Store) writePassages(id int64, text string) error {
	if id <= 0 {
		return nil
	}
	_, _ = s.db.Exec(`DELETE FROM memory_passages WHERE episode_id=?`, id)
	chunks := chunkTeach(text)
	if len(chunks) <= 1 {
		return nil
	}
	for i, chunk := range chunks {
		vec := EmbedText(chunk)
		if isZero(vec) {
			continue
		}
		if _, err := s.db.Exec(`
INSERT INTO memory_passages(episode_id, ord, text, embedder_id, dimensions, quantization, vector)
VALUES(?,?,?,?,?,?,?)`,
			id, i, chunk, CurrentEmbedder(), embedDimensions, quantizationInt8, vec.Bytes()); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) backfillPassages(whereSQL string, args []any) {
	q := `SELECT id FROM memory_episodes
WHERE ` + whereSQL + `
AND tokens > ?
AND id NOT IN (SELECT episode_id FROM memory_passages)
LIMIT 200`
	in := append(append([]any{}, args...), teachChunkTokens)
	rows, err := s.db.Query(q, in...)
	if err != nil {
		return
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	_ = rows.Close()
	for _, id := range ids {
		ep, err := s.Get(id)
		if err != nil || strings.TrimSpace(ep.Text) == "" {
			continue
		}
		_ = s.writePassages(id, ep.Text)
	}
}
