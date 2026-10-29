package memory

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
