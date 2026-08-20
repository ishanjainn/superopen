package memory

import (
	"time"
)

const edgeHalfLifeDays = 90.0
const edgeDecay = 0.9

func (s *Store) Sleep() error {
	if err := s.decayEdges(); err != nil {
		return err
	}
	if err := s.eraseHinted(); err != nil {
		return err
	}
	if err := s.flushPendingEmbeddings(); err != nil {
		return err
	}
	if err := s.backfillTags(); err != nil {
		return err
	}
	if err := s.ClusterTopics(); err != nil {
		return err
	}
	_, _ = s.Curiosity(8)
	return nil
}

func SleepRoot(root string) error {
	store, err := OpenRoot(root)
	if err != nil {
		return err
	}
	pending := append([]string{}, store.PendingDistill()...)
	err = store.Sleep()
	store.Close()
	if err != nil {
		return err
	}
	for _, id := range pending {
		_ = MaybeDistill(root, id, true)
	}
	return nil
}

func (s *Store) eraseHinted() error {
	now := nowRFC()
	_, err := s.db.Exec(`UPDATE memory_episodes SET faded=1, fading=0, faded_at=?, updated_at=?
WHERE fading=1 AND faded=0 AND pinned=0 AND never_decay=0`, now, now)
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

func (s *Store) decayEdges() error {
	rows, err := s.db.Query(`SELECT e.source_id, e.target_id, e.type, e.weight, COALESCE(e.updated_at, ''), COALESCE(src.never_decay,0), COALESCE(dst.never_decay,0)
FROM memory_edges e
JOIN memory_episodes src ON src.id=e.source_id
JOIN memory_episodes dst ON dst.id=e.target_id`)
	if err != nil {
		return err
	}
	type row struct {
		src, dst         int64
		typ              string
		w                float64
		updated          string
		srcHold, dstHold int
	}
	var all []row
	for rows.Next() {
		var r row
		if rows.Scan(&r.src, &r.dst, &r.typ, &r.w, &r.updated, &r.srcHold, &r.dstHold) != nil {
			continue
		}
		all = append(all, r)
	}
	_ = rows.Close()
	now := time.Now().UTC()
	for _, r := range all {
		if r.srcHold != 0 || r.dstHold != 0 {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, r.updated)
		if err != nil {
			t, err = time.Parse("2006-01-02 15:04:05", r.updated)
			if err != nil {
				continue
			}
		}
		days := now.Sub(t).Hours() / 24
		if days < edgeHalfLifeDays {
			continue
		}
		w := r.w * edgeDecay
		if w < 0.05 {
			_, _ = s.db.Exec(`DELETE FROM memory_edges WHERE source_id=? AND target_id=? AND type=?`, r.src, r.dst, r.typ)
			continue
		}
		_, _ = s.db.Exec(`UPDATE memory_edges SET weight=? WHERE source_id=? AND target_id=? AND type=?`, w, r.src, r.dst, r.typ)
	}
	return nil
}
