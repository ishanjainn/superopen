package memory

import (
	"fmt"
	"math"
	"strings"
	"time"
)

type SearchFilter struct {
	Query         string
	Kind          string
	Type          string
	SessionID     string
	File          string
	Limit         int
	IncludeFaded  bool
	RecordEconomy bool
	AsOf          string
	ChangedSince  string
}

func (s *Store) Search(filter SearchFilter) ([]Hit, error) {
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.RecordEconomy {
		_ = s.RecordSearch()
	}
	query := strings.TrimSpace(filter.Query)
	lexW := s.knobFloat("lex_fusion", lexFusionW)
	cosW := s.knobFloat("cosine_weight", cosineWeight)
	centW := s.knobFloat("centrality_weight", centralityWeight)
	shapeW := s.knobFloat("shape_fusion", shapeFusionW)
	pinW := s.knobFloat("pin_weight", pinWeight)
	staleW := s.knobFloat("stale_weight", staleDownweight)
	recencyHL := s.knobFloat("recency_half_life", 21)
	window := s.knobInt("supersede_window", supersedeCapWindow)

	ftsIDs := s.ftsCandidates(query)

	where := []string{"1=1"}
	args := []any{}
	if !filter.IncludeFaded {
		where = append(where, "faded=0")
	}
	if filter.Kind != "" {
		where = append(where, "kind=?")
		args = append(args, filter.Kind)
	}
	if typ := strings.TrimSpace(filter.Type); typ != "" {
		where = append(where, "(topic=? OR kind=?)")
		args = append(args, typ, typ)
	}
	if filter.SessionID != "" {
		where = append(where, "session_id=?")
		args = append(args, filter.SessionID)
	}
	if filter.File != "" {
		norm := NormalizePath(filter.File)
		where = append(where, "(files LIKE ? OR files LIKE ?)")
		args = append(args, "%"+filter.File+"%", "%"+norm+"%")
	}
	if t := parseTimeArg(filter.AsOf); t != "" {
		where = append(where, "(valid_from='' OR valid_from<=?) AND (valid_to='' OR valid_to>=?)")
		args = append(args, t, t)
	}
	if t := parseTimeArg(filter.ChangedSince); t != "" {
		where = append(where, "updated_at>=?")
		args = append(args, t)
	}
	q := `SELECT ` + episodeCols + ` FROM memory_episodes WHERE ` + strings.Join(where, " AND ") + ` ORDER BY pinned DESC, created_at DESC LIMIT 800`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	episodes, err := s.scanEpisodes(rows)
	if err != nil {
		return nil, err
	}

	var qvec Vector
	useVec := query != ""
	if useVec {
		qvec = EmbedText(query)
		if isZero(qvec) {
			useVec = false
		}
	}
	stale, outgoing, _ := s.contradictMaps()
	now := time.Now().UTC()
	hits := make([]Hit, 0, len(episodes))
	maxC := 1.0
	for _, ep := range episodes {
		if ep.Centrality > maxC {
			maxC = ep.Centrality
		}
	}
	for _, ep := range episodes {
		if ep.Kind == KindTool {
			continue
		}
		score := 0.0
		if fts, ok := ftsIDs[ep.ID]; ok {
			score += lexW * fts
		} else if query != "" && !strings.Contains(strings.ToLower(ep.Title+" "+ep.Text+" "+ep.Topic), strings.ToLower(query)) {
			if !useVec {
				continue
			}
		}
		cos := 0.0
		if useVec {
			var blob []byte
			var embedder string
			if err := s.db.QueryRow(`SELECT embedder_id, vector FROM memory_vectors WHERE episode_id=?`, ep.ID).Scan(&embedder, &blob); err == nil && embedder == CurrentEmbedder() {
				cos = Cosine(qvec, blob)
			}
		}
		cent := 0.0
		if maxC > 0 {
			cent = ep.Centrality / maxC
		}
		score += cosW*cos + centW*cent
		if useVec {
			var blob []byte
			if err := s.db.QueryRow(`SELECT blob FROM memory_shapes WHERE episode_id=?`, ep.ID).Scan(&blob); err == nil {
				score += shapeW * shapeScore(bindShape(qvec), blob)
			}
		}
		if ep.Pinned {
			score += pinW
		}
		if created, err := time.Parse(time.RFC3339Nano, ep.CreatedAt); err == nil {
			days := now.Sub(created).Hours() / 24
			if recencyHL <= 0 {
				recencyHL = 21
			}
			score += math.Exp(-days / recencyHL)
		}
		if ep.Faded {
			score -= 0.8
		}
		if query == "" {
			score += 0.01 * float64(ep.ID)
		}
		if inventedLearnedFiction(ep) {
			score *= 0.2
		}
		ep.Score = score
		hits = append(hits, Hit{Episode: ep, Snippet: firstLine(ep.Text, 140)})
	}
	hist := historicalCue(query)
	hits = applyStaleDownweight(hits, stale, hist, staleW)
	hits = applySupersedeCap(hits, outgoing, stale, hist, window)
	sortHits(hits)
	if len(hits) > filter.Limit {
		hits = hits[:filter.Limit]
	}
	if len(hits) == 0 && filter.Kind == "" && filter.Type == "" && filter.SessionID == "" && filter.File == "" {
		return s.sessionTitleHits(8), nil
	}
	return hits, nil
}

func (s *Store) FileContext(path string, limit int) ([]Hit, error) {
	return s.Search(SearchFilter{File: path, Limit: limit, RecordEconomy: false})
}

type TimelineBucket struct {
	When  string    `json:"when"`
	Items []Episode `json:"items"`
}

func (s *Store) Timeline(limit int) ([]TimelineBucket, error) {
	if limit <= 0 {
		limit = 80
	}
	rows, err := s.db.Query(`SELECT `+episodeCols+` FROM memory_episodes WHERE faded=0 AND kind != 'tool' ORDER BY created_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	episodes, err := s.scanEpisodes(rows)
	if err != nil {
		return nil, err
	}
	groups := map[string][]Episode{}
	order := []string{}
	for _, ep := range episodes {
		when := whenLabel(ep.CreatedAt)
		if _, ok := groups[when]; !ok {
			order = append(order, when)
		}
		groups[when] = append(groups[when], ep)
	}
	out := make([]TimelineBucket, 0, len(order))
	for _, when := range order {
		out = append(out, TimelineBucket{When: when, Items: groups[when]})
	}
	return out, nil
}

func (s *Store) TimelineAround(id int64, before, after int) ([]Episode, error) {
	if before <= 0 {
		before = 5
	}
	if after <= 0 {
		after = 5
	}
	anchor, err := s.scanOne(`SELECT `+episodeCols+` FROM memory_episodes WHERE id=?`, id)
	if err != nil {
		return nil, err
	}
	prev, err := s.db.Query(`SELECT `+episodeCols+` FROM memory_episodes WHERE faded=0 AND kind != 'tool' AND (created_at < ? OR (created_at=? AND id<?)) ORDER BY created_at DESC, id DESC LIMIT ?`,
		anchor.CreatedAt, anchor.CreatedAt, anchor.ID, before)
	if err != nil {
		return nil, err
	}
	older, err := s.scanEpisodes(prev)
	if err != nil {
		return nil, err
	}
	next, err := s.db.Query(`SELECT `+episodeCols+` FROM memory_episodes WHERE faded=0 AND kind != 'tool' AND (created_at > ? OR (created_at=? AND id>?)) ORDER BY created_at ASC, id ASC LIMIT ?`,
		anchor.CreatedAt, anchor.CreatedAt, anchor.ID, after)
	if err != nil {
		return nil, err
	}
	newer, err := s.scanEpisodes(next)
	if err != nil {
		return nil, err
	}
	out := make([]Episode, 0, len(older)+1+len(newer))
	for i := len(older) - 1; i >= 0; i-- {
		out = append(out, older[i])
	}
	out = append(out, anchor)
	out = append(out, newer...)
	return out, nil
}

// ftsCandidates runs multi-seed FTS as a candidate bonus, not as the
// only retrieval path. Cosine/corrector hits still rank from the SQL window.
func (s *Store) ftsCandidates(query string) map[int64]float64 {
	out := map[int64]float64{}
	query = strings.TrimSpace(query)
	if query == "" {
		return out
	}
	seeds := []string{query}
	for _, tok := range strings.Fields(query) {
		tok = strings.Trim(tok, `"*.,:;!?`)
		if len(tok) < 3 {
			continue
		}
		if strings.EqualFold(tok, query) {
			continue
		}
		seeds = append(seeds, tok)
		if len(seeds) >= 8 {
			break
		}
	}
	for _, seed := range seeds {
		rows, err := s.db.Query(`
SELECT rowid, bm25(memory_episodes_fts) FROM memory_episodes_fts
WHERE memory_episodes_fts MATCH ?
ORDER BY bm25(memory_episodes_fts)
LIMIT 80`, ftsQuery(seed))
		if err != nil {
			continue
		}
		for rows.Next() {
			var id int64
			var rank float64
			if rows.Scan(&id, &rank) != nil {
				continue
			}
			score := 1 / (1 + math.Abs(rank))
			if prev, ok := out[id]; !ok || score > prev {
				out[id] = score
			}
		}
		_ = rows.Close()
	}
	return out
}

func (s *Store) contradictMaps() (map[int64]bool, map[int64][]int64, map[int64][]int64) {
	stale := map[int64]bool{}
	outgoing := map[int64][]int64{}
	neighbors := map[int64][]int64{}
	rows, err := s.db.Query(`SELECT source_id, target_id FROM memory_edges WHERE type=?`, EdgeContradicts)
	if err != nil {
		return stale, outgoing, neighbors
	}
	defer rows.Close()
	for rows.Next() {
		var src, dst int64
		if rows.Scan(&src, &dst) != nil {
			continue
		}
		// source is the successor, target is the superseded fact.
		stale[dst] = true
		outgoing[dst] = append(outgoing[dst], src)
		neighbors[src] = append(neighbors[src], dst)
		neighbors[dst] = append(neighbors[dst], src)
	}
	return stale, outgoing, neighbors
}

func (s *Store) staleIDs() map[int64]bool {
	stale, _, _ := s.contradictMaps()
	return stale
}

func inventedLearnedFiction(ep Episode) bool {
	if ep.Source == SourceAgent || ep.Source == SourceTeach {
		return false
	}
	blob := ep.Title + "\n" + ep.Text
	for _, line := range strings.Split(blob, "\n") {
		if strings.HasPrefix(strings.TrimSpace(strings.ToLower(line)), "learned:") {
			return true
		}
	}
	return false
}

func ftsQuery(q string) string {
	parts := strings.Fields(q)
	quoted := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.Trim(p, `"*`)
		if p == "" {
			continue
		}
		quoted = append(quoted, `"`+strings.ReplaceAll(p, `"`, `""`)+`"`)
	}
	if len(quoted) == 0 {
		return `""`
	}
	return strings.Join(quoted, " ")
}

func whenLabel(created string) string {
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		t, err = time.Parse(time.RFC3339, created)
		if err != nil {
			return "unknown"
		}
	}
	now := time.Now().UTC()
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	if y1 == y2 && m1 == m2 && d1 == d2 {
		return "today"
	}
	if now.Sub(t) < 48*time.Hour && y1 == y2 && m1 == m2 && d1 == d2-1 {
		return "yesterday"
	}
	if now.Sub(t) < 7*24*time.Hour {
		return "this week"
	}
	return t.Format("2006-01")
}

func parseTimeArg(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02", "2006-01"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC().Format(time.RFC3339Nano)
		}
	}
	return raw
}

func sortHits(hits []Hit) {
	for i := 0; i < len(hits); i++ {
		for j := i + 1; j < len(hits); j++ {
			if hits[j].Score > hits[i].Score || (hits[j].Score == hits[i].Score && hits[j].ID < hits[i].ID) {
				hits[i], hits[j] = hits[j], hits[i]
			}
		}
	}
}

func (s *Store) Preview(id int64) string {
	ep, err := s.Get(id)
	if err != nil {
		return fmt.Sprintf("memory %d not found", id)
	}
	return firstLine(ep.Title, 80)
}
