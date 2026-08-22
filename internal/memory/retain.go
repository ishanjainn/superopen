package memory

import (
	"fmt"
	"strings"
	"time"
)

// DeleteExpired hard-deletes unpinned moments older than cutoff.
// Teachings, pins, and never_decay rows are kept.
func (s *Store) DeleteExpired(cutoff time.Time) (int, error) {
	if cutoff.IsZero() {
		return 0, nil
	}
	res, err := s.db.Exec(
		`DELETE FROM memory_episodes
WHERE pinned=0 AND never_decay=0 AND kind NOT IN (?, ?) AND created_at < ?`,
		KindTeaching, KindPin, cutoff.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// DeleteUnprotectedForSessions removes diary rows tied to deleted session ids.
// Teachings, pins, and never_decay rows stay.
func (s *Store) DeleteUnprotectedForSessions(sessionIDs []string) (int, error) {
	ids := make([]string, 0, len(sessionIDs))
	for _, id := range sessionIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return 0, nil
	}
	args := make([]any, 0, len(ids)+2)
	args = append(args, KindTeaching, KindPin)
	placeholders := make([]string, 0, len(ids))
	for _, id := range ids {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	q := fmt.Sprintf(
		`DELETE FROM memory_episodes
WHERE pinned=0 AND never_decay=0 AND kind NOT IN (?, ?) AND session_id IN (%s)`,
		strings.Join(placeholders, ","),
	)
	res, err := s.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
