package memory

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	staleDownweight    = 0.5
	supersedeCapEps    = 1e-4
	supersedeCapWindow = 10
	pinWeight          = 0.35
	recallBudgetTok    = 1500
	// Ten ranked ids (index + bodies). Top hits get a usable body; the rest
	// stay fetchable with so memory get --full.
	recallHitCap        = 10
	recallDeepHits      = 3
	recallDeepTok       = 350
	rrfK                = 60
	ftsRRFNum           = 2.0
	denseRRFNum         = 1.0
	denseRRFLexical     = 0.05
	ftsCandidateLimit   = 400
	denseCandidateLimit = 400
	// Lexical-first blend: identifiers, names, and error strings usually
	// are the right memories in any repo. Two slots stay open for hits
	// that only matched by meaning.
	ftsPrimaryKeep  = 8
	denseComplement = 2
)

// blendLexicalSemantic keeps the strongest keyword matches in front and
// reserves a couple of slots for memories that only matched by meaning.
func blendLexicalSemantic(hits []Hit, ftsRank, cosRank map[int64]int, lexKeep, semKeep int) []Hit {
	if len(hits) == 0 || (len(ftsRank) == 0 && len(cosRank) == 0) {
		return hits
	}
	if lexKeep < 0 {
		lexKeep = ftsPrimaryKeep
	}
	if semKeep < 0 {
		semKeep = denseComplement
	}
	var lex, sem, rest []Hit
	for _, h := range hits {
		ftsR, inFTS := ftsRank[h.ID]
		_, inDense := cosRank[h.ID]
		inCore := inFTS && ftsR < lexKeep
		switch {
		case inCore:
			lex = append(lex, h)
		case inFTS:
			lex = append(lex, h)
		case inDense:
			sem = append(sem, h)
		default:
			rest = append(rest, h)
		}
	}
	if lexKeep > len(lex) {
		lexKeep = len(lex)
	}
	if semKeep > len(sem) {
		semKeep = len(sem)
	}
	out := make([]Hit, 0, len(hits))
	out = append(out, lex[:lexKeep]...)
	out = append(out, sem[:semKeep]...)
	out = append(out, lex[lexKeep:]...)
	out = append(out, sem[semKeep:]...)
	out = append(out, rest...)
	return out
}

type scoredHit struct {
	Hit
	stale bool
}

func applyStaleDownweight(hits []Hit, stale map[int64]bool, historical bool, weight float64) []Hit {
	if historical {
		return hits
	}
	if weight <= 0 {
		weight = staleDownweight
	}
	now := time.Now().UTC()
	for i := range hits {
		if !stale[hits[i].ID] && !expired(hits[i].ValidTo, now) {
			continue
		}
		hits[i].Score *= weight
	}
	return hits
}

func applySupersedeCap(hits []Hit, outgoing map[int64][]int64, stale map[int64]bool, historical bool, window int) []Hit {
	if historical || len(outgoing) == 0 || len(hits) == 0 {
		return hits
	}
	if window <= 0 {
		window = supersedeCapWindow
	}
	now := time.Now().UTC()
	byID := map[int64]*Hit{}
	for i := range hits {
		byID[hits[i].ID] = &hits[i]
	}
	for round := 0; round < len(hits); round++ {
		ordered := append([]Hit(nil), hits...)
		sortHits(ordered)
		win := window
		if win > len(ordered) {
			win = len(ordered)
		}
		top := map[int64]struct{}{}
		for _, h := range ordered[:win] {
			top[h.ID] = struct{}{}
		}
		changed := false
		for src, dsts := range outgoing {
			srcHit := byID[src]
			if srcHit == nil {
				continue
			}
			if !stale[src] && !expired(srcHit.ValidTo, now) {
				continue
			}
			best := math.Inf(-1)
			found := false
			for _, dst := range dsts {
				if _, ok := top[dst]; !ok {
					continue
				}
				if dstHit := byID[dst]; dstHit != nil {
					if !found || dstHit.Score > best {
						best = dstHit.Score
						found = true
					}
				}
			}
			if found && srcHit.Score >= best {
				srcHit.Score = best - supersedeCapEps
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return hits
}

func expired(validTo string, now time.Time) bool {
	if validTo == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339Nano, validTo)
	if err != nil {
		t, err = time.Parse(time.RFC3339, validTo)
		if err != nil {
			return false
		}
	}
	return t.Before(now)
}

func historicalCue(query string) bool {
	q := strings.ToLower(query)
	for _, p := range []string{"what did we used to", "old wording", "superseded", "previously said", "historical"} {
		if strings.Contains(q, p) {
			return true
		}
	}
	return false
}

func antiHits(hits []Hit, neighbors map[int64][]int64, byID map[int64]Hit, limit int) []Hit {
	seen := map[int64]struct{}{}
	for _, h := range hits {
		seen[h.ID] = struct{}{}
	}
	var out []Hit
	for _, h := range hits {
		for _, dst := range neighbors[h.ID] {
			if _, ok := seen[dst]; ok {
				continue
			}
			if ep, ok := byID[dst]; ok {
				out = append(out, ep)
				seen[dst] = struct{}{}
				if len(out) >= limit {
					return out
				}
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

func trimToBudget(hits []Hit, budget int, query string) []Hit {
	if len(hits) == 0 {
		return hits
	}
	keep := hits
	if len(keep) > recallHitCap {
		keep = append([]Hit(nil), hits[:recallHitCap]...)
	} else {
		keep = append([]Hit(nil), hits...)
	}
	n := len(keep)
	deep := recallDeepHits
	if deep > n {
		deep = n
	}
	used := 0
	for i := range keep {
		if i < deep {
			share := recallDeepTok
			if budget > 0 {
				remain := budget - used
				if remain < 32 {
					remain = 32
				}
				if share > remain {
					share = remain
				}
			}
			keep[i] = clipHitToTokens(keep[i], share, query)
			used += keep[i].Tokens
			continue
		}
		origTok := keep[i].Tokens
		keep[i].Text = ""
		keep[i].Snippet = firstLine(keep[i].Title, 140)
		if origTok > 0 {
			keep[i].Tokens = origTok
		}
	}
	return keep
}

func clipHitToTokens(h Hit, maxTok int, query string) Hit {
	titleCost := EstimateTokens(h.Title)
	remain := maxTok - titleCost
	if remain < 16 {
		remain = 16
	}
	if EstimateTokens(h.Text) > remain {
		h.Text = clipAroundQuery(h.Text, query, remain)
	}
	h.Tokens = EstimateTokens(h.Title + " " + h.Text)
	h.Snippet = firstLine(h.Text, 140)
	return h
}

type queryAnchor struct {
	term    string
	bytePos int
	count   int
}

// clipAroundQuery keeps the distinctive query span(s) so a fact in the tail of
// a long diary body survives. No match → head clip. The first line (date,
// title, or path) is always retained when the body is trimmed.
func clipAroundQuery(s, query string, maxTok int) string {
	if maxTok <= 0 || EstimateTokens(s) <= maxTok {
		return s
	}
	head, rest := firstBodyLine(s)
	if head != "" && rest != "" {
		remain := maxTok - EstimateTokens(head) - 1
		if remain >= 12 {
			return head + "\n" + clipAroundQueryInner(rest, query, remain)
		}
	}
	return clipAroundQueryInner(s, query, maxTok)
}

func firstBodyLine(s string) (head, rest string) {
	raw := strings.TrimPrefix(s, "\n")
	i := strings.IndexByte(raw, '\n')
	if i <= 0 {
		return "", s
	}
	head = strings.TrimSpace(raw[:i])
	if head == "" {
		return "", s
	}
	runes := []rune(head)
	if len(runes) > 160 {
		head = string(runes[:160])
	}
	return head, raw[i+1:]
}

func clipAroundQueryInner(s, query string, maxTok int) string {
	if maxTok <= 0 || EstimateTokens(s) <= maxTok {
		return s
	}
	maxRunes := maxTok * 4
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	anchors := queryAnchors(s, query)
	if len(anchors) == 0 {
		return string(runes[:maxRunes])
	}
	w1Budget := maxRunes
	if len(anchors) > 1 {
		w1Budget = maxRunes * 2 / 3
		if w1Budget < 48 {
			w1Budget = maxRunes
		}
	}
	w1 := clipWindow(runes, utf8.RuneCountInString(s[:anchors[0].bytePos]), w1Budget)
	second := -1
	for i := 1; i < len(anchors); i++ {
		rp := utf8.RuneCountInString(s[:anchors[i].bytePos])
		if rp < w1.start || rp >= w1.end {
			second = i
			break
		}
	}
	if second < 0 {
		w := clipWindow(runes, utf8.RuneCountInString(s[:anchors[0].bytePos]), maxRunes)
		return w.render()
	}
	remain := maxRunes - (w1.end - w1.start)
	if remain < 32 {
		return w1.render()
	}
	w2 := clipWindow(runes, utf8.RuneCountInString(s[:anchors[second].bytePos]), remain)
	if w2.end > w1.start && w2.start < w1.end {
		start := w1.start
		if w2.start < start {
			start = w2.start
		}
		end := w1.end
		if w2.end > end {
			end = w2.end
		}
		if end-start > maxRunes {
			end = start + maxRunes
			if end > len(runes) {
				end = len(runes)
				start = end - maxRunes
				if start < 0 {
					start = 0
				}
			}
		}
		return (clipSpan{start: start, end: end, runes: runes}).render()
	}
	if w2.start < w1.start {
		return strings.TrimSuffix(w2.render(), "…") + "…" + strings.TrimPrefix(w1.render(), "…")
	}
	return strings.TrimSuffix(w1.render(), "…") + "…" + strings.TrimPrefix(w2.render(), "…")
}

type clipSpan struct {
	start int
	end   int
	runes []rune
}

func clipWindow(runes []rune, runePos, maxRunes int) clipSpan {
	if maxRunes <= 0 || len(runes) == 0 {
		return clipSpan{runes: runes}
	}
	if maxRunes > len(runes) {
		maxRunes = len(runes)
	}
	lead := maxRunes / 3
	start := runePos - lead
	if start < 0 {
		start = 0
	}
	end := start + maxRunes
	if end > len(runes) {
		end = len(runes)
		start = end - maxRunes
		if start < 0 {
			start = 0
		}
	}
	return clipSpan{start: start, end: end, runes: runes}
}

func (w clipSpan) render() string {
	if w.end <= w.start || len(w.runes) == 0 {
		return ""
	}
	out := string(w.runes[w.start:w.end])
	if w.start > 0 {
		out = "…" + out
	}
	if w.end < len(w.runes) {
		out += "…"
	}
	return out
}

// queryAnchors ranks query terms by in-document frequency (rarest first) so a
// common name at the start of a diary does not steal the clip window from the
// distinctive fact in the tail.
func queryAnchors(s, query string) []queryAnchor {
	if strings.TrimSpace(query) == "" || s == "" {
		return nil
	}
	lower := strings.ToLower(s)
	terms := ftsTerms(query)
	if len(terms) == 0 {
		q := strings.ToLower(strings.TrimSpace(query))
		if q == "" {
			return nil
		}
		i := strings.Index(lower, q)
		if i < 0 {
			return nil
		}
		return []queryAnchor{{term: q, bytePos: i, count: 1}}
	}
	var out []queryAnchor
	for _, term := range terms {
		pos := strings.Index(lower, term)
		if pos < 0 {
			continue
		}
		out = append(out, queryAnchor{term: term, bytePos: pos, count: strings.Count(lower, term)})
	}
	if len(out) == 0 {
		q := strings.ToLower(strings.TrimSpace(query))
		if q == "" {
			return nil
		}
		i := strings.Index(lower, q)
		if i < 0 {
			return nil
		}
		return []queryAnchor{{term: q, bytePos: i, count: 1}}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].count != out[j].count {
			return out[i].count < out[j].count
		}
		if len(out[i].term) != len(out[j].term) {
			return len(out[i].term) > len(out[j].term)
		}
		return out[i].bytePos > out[j].bytePos
	})
	return out
}

func parseID(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}
