package session

import (
	"os"
	"path/filepath"
	"time"
)

// DeleteOlderThan removes session directories whose last activity is before
// cutoff. Active chats with recent events.jsonl / session.json mtimes are kept.
func (s *Store) DeleteOlderThan(cutoff time.Time) ([]string, error) {
	if cutoff.IsZero() {
		return nil, nil
	}
	entries, err := s.List()
	if err != nil {
		return nil, err
	}
	var deleted []string
	for _, meta := range entries {
		if !expired(meta, s.Paths.SessionDir(meta.ID), cutoff) {
			continue
		}
		if err := s.Delete(meta.ID); err != nil {
			return deleted, err
		}
		deleted = append(deleted, meta.ID)
	}
	return deleted, nil
}

func expired(meta Meta, dir string, cutoff time.Time) bool {
	ref := meta.StartedAt
	if meta.EndedAt != nil && meta.EndedAt.After(ref) {
		ref = *meta.EndedAt
	}
	for _, name := range []string{"session.json", "events.jsonl"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		if info.ModTime().After(ref) {
			ref = info.ModTime()
		}
	}
	if ref.IsZero() {
		return false
	}
	return ref.Before(cutoff)
}
