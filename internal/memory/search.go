package memory

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"
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
	Horizon       string
	ExcludeKinds  []string
}

func (s *Store) Search(filter SearchFilter) ([]Hit, error) {
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.RecordEconomy {
		_ = s.RecordSearch()
	}
	query := strings.TrimSpace(filter.Query)
	pinW := s.knobFloat("pin_weight", pinWeight)
	staleW := s.knobFloat("stale_weight", staleDownweight)
	recencyHL := s.knobFloat("recency_half_life", 21)
	window := s.knobInt("supersede_window", supersedeCapWindow)
	kRRF := s.knobInt("rrf_k", rrfK)
	if kRRF < 1 {
		kRRF = rrfK
	}
	ftsW := s.knobFloat("fts_rrf", ftsRRFNum)
	denseW := s.knobFloat("dense_rrf", denseRRFNum)
	denseLexW := s.knobFloat("dense_rrf_lexical", denseRRFLexical)
	ftsLim := s.knobInt("fts_candidates", ftsCandidateLimit)
	denseLim := s.knobInt("dense_candidates", denseCandidateLimit)
	lexKeep := s.knobInt("fts_keep", ftsPrimaryKeep)
	semKeep := s.knobInt("dense_keep", denseComplement)

	where := []string{"1=1"}
	args := []any{}
	if !filter.IncludeFaded {
		where = append(where, "faded=0")
	}
	if filter.Kind != "" {
		where = append(where, "kind=?")
		args = append(args, filter.Kind)
	}
	if n := len(filter.ExcludeKinds); n > 0 {
		ph := make([]string, 0, n)
		for _, k := range filter.ExcludeKinds {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			ph = append(ph, "?")
			args = append(args, k)
		}
		if len(ph) > 0 {
			where = append(where, "kind NOT IN ("+strings.Join(ph, ",")+")")
		}
	}
	if h := NormalizeHorizon(filter.Horizon); h != "" {
		where = append(where, "horizon=?")
		args = append(args, h)
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
	whereSQL := strings.Join(where, " AND ")

	ftsRank := map[int64]int{}
	cosRank := map[int64]int{}
	if query != "" {
		ftsRank = s.ftsRanked(query, ftsLim, whereSQL, args)
		qvec := EmbedQuery(query)
		if !isZero(qvec) {
			cosRank = s.denseRanked(qvec, denseLim, whereSQL, args)
		}
	}

	var episodes []Episode
	if query != "" && (len(ftsRank) > 0 || len(cosRank) > 0) {
		ids := make([]int64, 0, len(ftsRank)+len(cosRank))
		seen := map[int64]struct{}{}
		for id := range ftsRank {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
		for id := range cosRank {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
		episodes = s.episodesByIDs(ids, whereSQL, args)
	} else {
		q := `SELECT ` + episodeCols + ` FROM memory_episodes WHERE ` + whereSQL + ` ORDER BY pinned DESC, created_at DESC LIMIT 800`
		rows, err := s.db.Query(q, args...)
		if err != nil {
			return nil, err
		}
		var errScan error
		episodes, errScan = s.scanEpisodes(rows)
		if errScan != nil {
			return nil, errScan
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
		if query != "" && isSelfEcho(query, ep) {
			continue
		}
		score := 0.0
		_, lex := ftsRank[ep.ID]
		if r, ok := ftsRank[ep.ID]; ok {
			score += ftsW / (float64(kRRF) + float64(r))
		}
		if r, ok := cosRank[ep.ID]; ok {
			if lex {
				score += denseLexW / (float64(kRRF) + float64(r))
			} else {
				score += denseW / (float64(kRRF) + float64(r))
			}
		}
		if query != "" && len(ftsRank) == 0 && len(cosRank) == 0 {
			if strings.Contains(strings.ToLower(ep.Title+" "+ep.Text+" "+ep.Topic), strings.ToLower(query)) {
				score += 0.2
			}
		}
		cent := 0.0
		if maxC > 0 {
			cent = ep.Centrality / maxC
		}
		score += 0.001 * cent
		if ep.Pinned {
			score += 0.01 * pinW
		}
		if isKnowledgeKind(ep.Kind) {
			if created, err := time.Parse(time.RFC3339Nano, ep.CreatedAt); err == nil {
				days := now.Sub(created).Hours() / 24
				if recencyHL <= 0 {
					recencyHL = 21
				}
				score += 0.001 * math.Exp(-days/recencyHL)
			}
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
	if query != "" {
		hits = blendLexicalSemantic(hits, ftsRank, cosRank, lexKeep, semKeep)
	}
	if len(hits) > filter.Limit {
		hits = hits[:filter.Limit]
	}
	// Prefer hits that contain a proper-noun/identifier token from the
	// query, but only reorder within the already-retrieved top window so
	// the candidate set is unchanged. This surfaces named entities ahead
	// of a generic keyword match without dropping other strong hits.
	preferQueryProperNames(hits, query, recallHitCap)
	return hits, nil
}

func preferQueryProperNames(hits []Hit, query string, within int) {
	if len(hits) < 2 {
		return
	}
	names := queryProperTokens(query)
	if len(names) == 0 {
		return
	}
	if within > len(hits) {
		within = len(hits)
	}
	head := make([]Hit, 0, within)
	tail := make([]Hit, 0, within)
	for i := 0; i < within; i++ {
		if hitContainsProper(hits[i].Episode, names) {
			head = append(head, hits[i])
		} else {
			tail = append(tail, hits[i])
		}
	}
	copy(hits[:within], append(head, tail...))
}

func (s *Store) episodesByIDs(ids []int64, whereSQL string, args []any) []Episode {
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, 0, len(ids))
	inArgs := append([]any{}, args...)
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		placeholders = append(placeholders, "?")
		inArgs = append(inArgs, id)
	}
	if len(placeholders) == 0 {
		return nil
	}
	q := `SELECT ` + episodeCols + ` FROM memory_episodes WHERE ` + whereSQL + ` AND id IN (` + strings.Join(placeholders, ",") + `)`
	rows, err := s.db.Query(q, inArgs...)
	if err != nil {
		return nil
	}
	eps, err := s.scanEpisodes(rows)
	if err != nil {
		return nil
	}
	return eps
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

func (s *Store) ftsRanked(query string, limit int, whereSQL string, args []any) map[int64]int {
	out := map[int64]int{}
	match := ftsQuery(query)
	if match == "" || match == `""` {
		return out
	}
	if limit <= 0 {
		limit = ftsCandidateLimit
	}
	// Bind WHERE args first, then MATCH, then LIMIT — the same order as the
	// SQL. Prepending MATCH to ExcludeKinds args made every default recall
	// search for the literal kind name "prompt".
	q := `
SELECT f.rowid FROM memory_episodes_fts f
WHERE f.rowid IN (SELECT id FROM memory_episodes WHERE ` + whereSQL + `)
AND memory_episodes_fts MATCH ?
ORDER BY bm25(memory_episodes_fts)
LIMIT ?`
	in := append(append([]any{}, args...), match, limit)
	rows, err := s.db.Query(q, in...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "so memory: FTS query failed (%v); ranking without keyword matches\n", err)
		return out
	}
	defer rows.Close()
	rank := 0
	for rows.Next() {
		var id int64
		if rows.Scan(&id) != nil {
			continue
		}
		out[id] = rank
		rank++
	}
	return out
}

func (s *Store) denseRanked(qvec Vector, limit int, whereSQL string, args []any) map[int64]int {
	out := map[int64]int{}
	if isZero(qvec) {
		return out
	}
	if limit <= 0 {
		limit = denseCandidateLimit
	}
	s.backfillPassages(whereSQL, args)
	best := map[int64]float64{}
	q := `SELECT v.episode_id, v.vector FROM memory_vectors v
WHERE v.embedder_id=? AND v.episode_id IN (SELECT id FROM memory_episodes WHERE ` + whereSQL + `)`
	in := append([]any{CurrentEmbedder()}, args...)
	rows, err := s.db.Query(q, in...)
	if err != nil {
		return out
	}
	for rows.Next() {
		var id int64
		var blob []byte
		if rows.Scan(&id, &blob) != nil {
			continue
		}
		best[id] = Cosine(qvec, blob)
	}
	_ = rows.Close()
	pq := `SELECT episode_id, vector FROM memory_passages
WHERE embedder_id=? AND episode_id IN (SELECT id FROM memory_episodes WHERE ` + whereSQL + `)`
	prows, err := s.db.Query(pq, in...)
	if err == nil {
		for prows.Next() {
			var id int64
			var blob []byte
			if prows.Scan(&id, &blob) != nil {
				continue
			}
			c := Cosine(qvec, blob)
			if prev, ok := best[id]; !ok || c > prev {
				best[id] = c
			}
		}
		_ = prows.Close()
	}
	type scored struct {
		id  int64
		cos float64
	}
	all := make([]scored, 0, len(best))
	for id, cos := range best {
		all = append(all, scored{id: id, cos: cos})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].cos > all[j].cos })
	if len(all) > limit {
		all = all[:limit]
	}
	for i, row := range all {
		out[row.id] = i
	}
	return out
}

func ftsTerms(q string) []string {
	seen := map[string]struct{}{}
	var terms []string
	fields := strings.FieldsFunc(strings.ToLower(q), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for _, p := range fields {
		if len(p) < 3 || isFTSStopword(p) {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		terms = append(terms, p)
		if len(terms) >= 32 {
			break
		}
	}
	return terms
}

func isFTSStopword(tok string) bool {
	_, ok := ftsStopwords[tok]
	return ok
}

var ftsStopwords = map[string]struct{}{
	"the": {}, "a": {}, "an": {}, "and": {}, "or": {}, "but": {}, "if": {}, "in": {}, "on": {}, "at": {},
	"to": {}, "for": {}, "of": {}, "as": {}, "is": {}, "was": {}, "were": {}, "be": {}, "been": {}, "being": {},
	"it": {}, "this": {}, "that": {}, "these": {}, "those": {}, "i": {}, "you": {}, "he": {}, "she": {}, "we": {},
	"they": {}, "what": {}, "which": {}, "who": {}, "whom": {}, "when": {}, "where": {}, "why": {}, "how": {},
	"did": {}, "does": {}, "do": {}, "not": {}, "no": {}, "nor": {}, "so": {}, "then": {}, "than": {}, "from": {},
	"with": {}, "by": {}, "about": {}, "into": {}, "over": {}, "after": {}, "before": {}, "between": {},
	"your": {}, "my": {}, "our": {}, "their": {}, "can": {}, "could": {}, "would": {}, "should": {}, "will": {},
	"just": {}, "have": {}, "has": {}, "had": {}, "are": {}, "its": {}, "also": {}, "any": {}, "all": {},
}

func ftsQuery(q string) string {
	terms := ftsTerms(q)
	if len(terms) == 0 {
		return `""`
	}
	termQ := joinFTSTerms(terms)
	names := ftsProperNameTerms(q)
	if len(names) == 0 {
		return termQ
	}
	// Names are a soft OR group so a missing/wrong capitalized token cannot
	// zero the match set. Ranking still prefers named hits via
	// preferQueryProperNames.
	return "(" + joinFTSTerms(names) + ") OR (" + termQ + ")"
}

func ftsProperNameTerms(q string) []string {
	var names []string
	seen := map[string]struct{}{}
	for _, n := range queryProperTokens(q) {
		k := strings.ToLower(strings.TrimSpace(n))
		if len(k) < 3 || isFTSStopword(k) {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		names = append(names, k)
	}
	return names
}

func joinFTSTerms(terms []string) string {
	quoted := make([]string, 0, len(terms))
	for _, p := range terms {
		quoted = append(quoted, ftsMatchTerm(p))
	}
	return strings.Join(quoted, " OR ")
}

func ftsMatchTerm(p string) string {
	p = strings.ToLower(strings.TrimSpace(p))
	if p == "" {
		return `""`
	}
	if len(p) >= 4 && ftsSafeIdent(p) {
		// Prefix so "move" matches "moved" / "moving" without a stemmer.
		return p + "*"
	}
	return `"` + strings.ReplaceAll(p, `"`, `""`) + `"`
}

func ftsSafeIdent(p string) bool {
	for _, r := range p {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
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
			if hitLess(hits[j], hits[i]) {
				hits[i], hits[j] = hits[j], hits[i]
			}
		}
	}
}

func hitLess(a, b Hit) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	ka, kb := isKnowledgeKind(a.Kind), isKnowledgeKind(b.Kind)
	if ka != kb {
		return ka
	}
	return a.ID < b.ID
}

func isKnowledgeKind(kind string) bool {
	switch kind {
	case KindSession, KindPin, KindTeaching:
		return true
	default:
		return false
	}
}

func (s *Store) hasKnowledge() bool {
	if s == nil || s.db == nil {
		return false
	}
	var n int
	_ = s.db.QueryRow(`SELECT count(*) FROM memory_episodes WHERE faded=0 AND kind IN (?,?,?)`,
		KindSession, KindPin, KindTeaching).Scan(&n)
	return n > 0
}

// isSelfEcho reports a short row that is substantially the caller's query
// (the just-logged prompt echoed back as a "memory"). Long diary bodies that
// merely contain the question stay ranked. A Working/Prompt copy of the full
// live question (including a "Working snapshot:" wrapper) drops regardless of
// length, because that wrapper is not knowledge.
func isSelfEcho(query string, ep Episode) bool {
	q := strings.TrimSpace(query)
	if q == "" {
		return false
	}
	blob := strings.TrimSpace(ep.Title + " " + ep.Text)
	if blob == "" {
		return false
	}
	qn := normalizeEcho(q)
	bn := normalizeEcho(blob)
	if qn == "" || bn == "" {
		return false
	}
	if qn == bn {
		return true
	}
	// A prompt/working row whose title is the exact live question and whose
	// body is the question (optionally a "Working snapshot:" wrapper) is the
	// caller's own turn echoed back. Length-independent, diary only.
	if ep.Kind == KindPrompt || ep.Kind == KindWorking {
		titleN := normalizeEcho(ep.Title)
		textN := normalizeEcho(ep.Text)
		if titleN == qn && (textN == qn || strings.HasPrefix(textN, "working snapshot")) {
			return true
		}
	}
	// Otherwise only a short row near the query's own length can be an echo.
	// Substantial bodies (session knowledge, long diary) that merely share
	// vocabulary with the question stay ranked — dropping them treated a
	// real memory as an echo of the live query.
	if EstimateTokens(ep.Text) > EstimateTokens(q)*3+48 {
		return false
	}
	qTerms := echoTerms(qn)
	bTerms := echoTerms(bn)
	if len(bTerms) == 0 {
		return false
	}
	hit := 0
	for t := range bTerms {
		if qTerms[t] {
			hit++
		}
	}
	return float64(hit)/float64(len(bTerms)) >= 0.8
}

var queryNameStop = map[string]struct{}{
	"a": {}, "an": {}, "the": {}, "and": {}, "or": {}, "but": {}, "if": {}, "of": {},
	"to": {}, "in": {}, "on": {}, "for": {}, "from": {}, "with": {}, "how": {},
	"what": {}, "when": {}, "where": {}, "which": {}, "who": {}, "why": {}, "did": {},
	"does": {}, "do": {}, "is": {}, "are": {}, "was": {}, "were": {}, "be": {},
	"this": {}, "that": {}, "these": {}, "those": {}, "please": {}, "show": {},
	"tell": {}, "explain": {}, "about": {},
	"monday": {}, "tuesday": {}, "wednesday": {}, "thursday": {}, "friday": {},
	"saturday": {}, "sunday": {},
	"january": {}, "february": {}, "march": {}, "april": {}, "may": {}, "june": {},
	"july": {}, "august": {}, "september": {}, "october": {}, "november": {},
	"december": {},
}

func queryLooksProperToken(tok string) bool {
	if len(tok) < 3 {
		return false
	}
	if _, stop := queryNameStop[strings.ToLower(tok)]; stop {
		return false
	}
	upper, lower := 0, 0
	runes := []rune(tok)
	for _, r := range runes {
		if unicode.IsUpper(r) {
			upper++
		}
		if unicode.IsLower(r) {
			lower++
		}
	}
	if upper == 0 {
		return false
	}
	return upper >= 2 || (unicode.IsUpper(runes[0]) && lower > 0)
}

func queryProperTokens(query string) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	var out []string
	seen := map[string]struct{}{}
	for _, piece := range strings.Fields(query) {
		tok := strings.Trim(piece, `"'`+"`.,:;!?()[]{}")
		if !queryLooksProperToken(tok) {
			continue
		}
		key := strings.ToLower(tok)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, tok)
	}
	return out
}

func hitContainsProper(ep Episode, names []string) bool {
	if len(names) == 0 {
		return true
	}
	hay := strings.ToLower(ep.Title + " " + ep.Text)
	for _, n := range names {
		if n != "" && strings.Contains(hay, strings.ToLower(n)) {
			return true
		}
	}
	return false
}

func normalizeEcho(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

func echoTerms(s string) map[string]bool {
	out := map[string]bool{}
	for _, p := range strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len(p) < 2 || isFTSStopword(p) {
			continue
		}
		out[p] = true
	}
	return out
}

func (s *Store) Preview(id int64) string {
	ep, err := s.Get(id)
	if err != nil {
		return fmt.Sprintf("memory %d not found", id)
	}
	return firstLine(ep.Title, 80)
}
