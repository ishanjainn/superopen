package memory

func (s *Store) Sleep() error {
	if err := s.eraseHinted(); err != nil {
		return err
	}
	if _, err := s.ExpireHorizons(); err != nil {
		return err
	}
	if err := s.flushPendingEmbeddings(); err != nil {
		return err
	}
	return nil
}

func SleepRoot(root string) error {
	store, err := OpenRoot(root)
	if err != nil {
		return err
	}
	err = store.Sleep()
	store.Close()
	return err
}

func (s *Store) eraseHinted() error {
	now := nowRFC()
	_, err := s.db.Exec(`UPDATE memory_episodes SET faded=1, fading=0, faded_at=?, updated_at=?
WHERE fading=1 AND faded=0 AND pinned=0`, now, now)
	return err
}

func (s *Store) flushPendingEmbeddings() error {
	rows, err := s.db.Query(`SELECT ` + episodeCols + ` FROM memory_episodes WHERE embedding_pending=1 AND faded=0 LIMIT 200`)
	if err != nil {
		return err
	}
	eps, err := s.scanEpisodes(rows)
	if err != nil {
		return err
	}
	for _, ep := range eps {
		vec := EmbedText(ep.Title + "\n" + ep.Text)
		if isZero(vec) {
			continue
		}
		if err := s.writeVector(ep.ID, vec); err != nil {
			return err
		}
		_ = s.writeShape(ep.ID, vec)
		_, _ = s.db.Exec(`UPDATE memory_episodes SET embedding_pending=0, updated_at=? WHERE id=?`, nowRFC(), ep.ID)
	}
	return nil
}
