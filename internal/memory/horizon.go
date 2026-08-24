package memory

import (
	"strconv"
	"strings"
)

const (
	HorizonWorking = "working"
	HorizonShort   = "short"
	HorizonMedium  = "medium"
	HorizonLong    = "long"

	metaSessionSeq = "session_seq"
	metaDistilled  = "distilled_sessions"

	shortKeepSessions  = 3
	mediumKeepSessions = 30
)

func NormalizeHorizon(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case HorizonWorking, HorizonShort, HorizonMedium, HorizonLong:
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func DefaultHorizon(kind string, pin bool) string {
	if pin {
		return HorizonLong
	}
	switch kind {
	case KindTeaching, KindPin:
		return HorizonLong
	case KindSession:
		return HorizonMedium
	default:
		return HorizonWorking
	}
}

func horizonStrength(h string) int {
	switch NormalizeHorizon(h) {
	case HorizonLong:
		return 3
	case HorizonMedium:
		return 2
	case HorizonShort:
		return 1
	default:
		return 0
	}
}

func keepUntilDefault(horizon string, seq int) int {
	switch horizon {
	case HorizonShort:
		return seq + shortKeepSessions
	case HorizonMedium:
		return seq + mediumKeepSessions
	default:
		return 0
	}
}

func applyHorizonDefaults(ep *Episode, seq int) {
	if ep == nil {
		return
	}
	h := NormalizeHorizon(ep.Horizon)
	if h == "" {
		h = DefaultHorizon(ep.Kind, ep.Pinned)
	}
	ep.Horizon = h
	if h == HorizonLong {
		ep.NeverDecay = true
		ep.KeepUntilSession = 0
		return
	}
	if ep.KeepUntilSession <= 0 && (h == HorizonShort || h == HorizonMedium) {
		ep.KeepUntilSession = keepUntilDefault(h, seq)
	}
}

func (s *Store) SessionSeq() int {
	raw, _ := s.meta(metaSessionSeq)
	n, _ := strconv.Atoi(strings.TrimSpace(raw))
	if n < 0 {
		return 0
	}
	return n
}

func (s *Store) BumpSessionSeq() int {
	n := s.SessionSeq() + 1
	_ = s.setMeta(metaSessionSeq, strconv.Itoa(n))
	return n
}

func (s *Store) DistilledSessions() []string {
	raw, _ := s.meta(metaDistilled)
	return splitCSV(raw)
}

func (s *Store) HasDistilled(sessionID string) bool {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	for _, id := range s.DistilledSessions() {
		if id == sessionID {
			return true
		}
	}
	return false
}

func (s *Store) MarkDistilled(sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	set := map[string]struct{}{}
	for _, id := range s.DistilledSessions() {
		set[id] = struct{}{}
	}
	set[sessionID] = struct{}{}
	return s.setMeta(metaDistilled, joinCSV(setKeys(set)))
}

func (s *Store) ExpireHorizons() (int, error) {
	seq := s.SessionSeq()
	now := nowRFC()
	res, err := s.db.Exec(`UPDATE memory_episodes SET faded=1, fading=0, faded_at=?, updated_at=?
WHERE faded=0 AND pinned=0 AND never_decay=0 AND horizon IN (?, ?) AND keep_until_session > 0 AND keep_until_session <= ?`,
		now, now, HorizonShort, HorizonMedium, seq)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (s *Store) PromoteHorizon(id int64, horizon string) error {
	horizon = NormalizeHorizon(horizon)
	if id <= 0 || horizon == "" {
		return nil
	}
	ep, err := s.Get(id)
	if err != nil {
		return err
	}
	if ep.Pinned && horizon != HorizonLong {
		return nil
	}
	keep := 0
	never := 0
	if horizon == HorizonLong {
		never = 1
	} else {
		keep = keepUntilDefault(horizon, s.SessionSeq())
	}
	_, err = s.db.Exec(`UPDATE memory_episodes SET horizon=?, keep_until_session=?, never_decay=?, faded=0, fading=0, faded_at='', updated_at=? WHERE id=?`,
		horizon, keep, never, nowRFC(), id)
	return err
}

func (s *Store) ForgetEpisode(id int64) error {
	if id <= 0 {
		return nil
	}
	ep, err := s.Get(id)
	if err != nil {
		return err
	}
	if ep.Pinned {
		return nil
	}
	now := nowRFC()
	_, err = s.db.Exec(`UPDATE memory_episodes SET faded=1, fading=0, faded_at=?, updated_at=? WHERE id=? AND pinned=0`, now, now, id)
	return err
}

func (s *Store) LiveKnowledge(limit int) ([]Episode, error) {
	if limit <= 0 {
		limit = 24
	}
	rows, err := s.db.Query(`SELECT `+episodeCols+` FROM memory_episodes
WHERE faded=0 AND horizon IN (?,?,?)
ORDER BY CASE horizon WHEN ? THEN 0 WHEN ? THEN 1 ELSE 2 END, created_at DESC, id DESC
LIMIT ?`, HorizonLong, HorizonMedium, HorizonShort, HorizonLong, HorizonMedium, limit)
	if err != nil {
		return nil, err
	}
	return s.scanEpisodes(rows)
}
