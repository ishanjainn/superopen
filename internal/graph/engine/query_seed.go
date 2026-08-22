package engine

import (
	"context"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// Superopen query seeding constants.
const (
	exactMatchBonus     = 1000.0
	prefixMatchBonus    = 100.0
	substringMatchBonus = 1.0
	sourceMatchBonus    = 0.5
	seedMaxK            = 3
	seedGapRatio        = 0.2
)

var queryWordToken = regexp.MustCompile(`[\pL\pN_]+`)

// Common English stopwords for query term extraction.
var queryStopwords = map[string]struct{}{
	"a": {}, "an": {}, "as": {}, "at": {}, "be": {}, "by": {}, "do": {}, "does": {},
	"did": {}, "how": {}, "what": {}, "when": {}, "where": {}, "which": {}, "who": {},
	"whom": {}, "why": {}, "is": {}, "are": {}, "was": {}, "were": {}, "am": {},
	"can": {}, "could": {}, "should": {}, "would": {}, "will": {}, "shall": {},
	"may": {}, "might": {}, "must": {}, "of": {}, "on": {}, "or": {}, "if": {},
	"in": {}, "to": {}, "it": {}, "its": {}, "has": {}, "have": {}, "had": {},
	"the": {}, "and": {}, "but": {}, "not": {}, "for": {}, "from": {}, "with": {},
	"without": {}, "into": {}, "onto": {}, "off": {}, "that": {}, "this": {},
	"these": {}, "those": {}, "there": {}, "here": {}, "their": {}, "them": {},
	"they": {}, "about": {}, "any": {}, "all": {}, "some": {}, "work": {},
	"works": {}, "working": {}, "please": {}, "explain": {}, "show": {}, "tell": {},
	"me": {}, "you": {}, "your": {}, "we": {}, "our": {},
	"calls": {}, "callers": {}, "callee": {}, "uses": {}, "using": {}, "find": {},
	"trace": {}, "path": {}, "configure": {}, "load": {}, "register": {},
}

type seedCandidate struct {
	node   api.Node
	score  float64
	degree int
}

type querySeedResult struct {
	seeds          []api.RankedNode
	primary        []api.RankedNode
	bestSeedByTerm map[string]api.RankedNode
	ranked         []api.RankedNode
}

func queryTerms(question string, extra []string) []string {
	raw := strings.TrimSpace(question + " " + strings.Join(extra, " "))
	var terms []string
	seen := map[string]struct{}{}
	add := func(tok string) {
		tok = strings.ToLower(strings.TrimSpace(tok))
		if tok == "" {
			return
		}
		if _, ok := seen[tok]; ok {
			return
		}
		seen[tok] = struct{}{}
		terms = append(terms, tok)
	}
	// Preserve dotted identifiers (Foo.bar) as a single term plus split parts.
	for _, piece := range strings.Fields(raw) {
		clean := strings.Trim(piece, `"'`+"`")
		if strings.Contains(clean, ".") {
			add(strings.ToLower(clean))
		}
		for _, tok := range queryWordToken.FindAllString(strings.ToLower(clean), -1) {
			if len(tok) > 2 || strings.Contains(clean, ".") {
				add(tok)
			}
		}
	}
	content := make([]string, 0, len(terms))
	for _, t := range terms {
		if _, stop := queryStopwords[t]; stop && !strings.Contains(t, ".") {
			continue
		}
		if !strings.Contains(t, ".") && len(t) <= 2 {
			continue
		}
		content = append(content, t)
	}
	if len(content) == 0 {
		return terms
	}
	return content
}

func queryNodeDisplayName(n api.Node) string {
	if n.Label != "File" && n.Label != "Folder" {
		return n.Name
	}
	p := strings.ReplaceAll(n.Location.File, "\\", "/")
	if p == "" {
		return n.Name
	}
	parts := strings.Split(p, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}
	return parts[len(parts)-1]
}

const queryFTSSeedLimit = 50

func (s *Store) querySeedCandidates(ctx context.Context, project, question string, terms []string) ([]seedCandidate, error) {
	seen := map[int64]bool{}
	var nodes []api.Node
	q := strings.TrimSpace(question)
	if q == "" {
		q = strings.Join(terms, " ")
	}
	if q != "" {
		res, err := s.Search(ctx, api.SearchRequest{Project: project, Query: q, Limit: queryFTSSeedLimit})
		if err != nil {
			return nil, err
		}
		for _, match := range res.Matches {
			if seen[match.ID] {
				continue
			}
			seen[match.ID] = true
			nodes = append(nodes, match.Node)
		}
	}
	degree, err := s.nodeDegrees(ctx, project)
	if err != nil {
		return nil, err
	}
	out := make([]seedCandidate, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, seedCandidate{node: node, degree: degree[node.ID]})
	}
	return out, nil
}

func computeIDF(candidates []seedCandidate, terms []string) map[string]float64 {
	n := float64(len(candidates))
	if n == 0 {
		n = 1
	}
	df := map[string]int{}
	for _, term := range terms {
		for _, cand := range candidates {
			hay := strings.ToLower(cand.node.Name + "\x00" + cand.node.QualifiedName + "\x00" + cand.node.Location.File)
			if strings.Contains(hay, term) {
				df[term]++
			}
		}
	}
	idf := map[string]float64{}
	for _, term := range terms {
		count := df[term]
		if count == 0 {
			count = 1
		}
		idf[term] = math.Log(1+n/float64(count)) + 1
	}
	return idf
}

func scoreQuerySeeds(candidates []seedCandidate, terms []string) querySeedResult {
	if len(candidates) == 0 || len(terms) == 0 {
		return querySeedResult{}
	}
	normTerms := make([]string, 0, len(terms))
	seenTerm := map[string]struct{}{}
	for _, t := range terms {
		for _, tok := range searchTokens(t) {
			if _, ok := seenTerm[tok]; ok {
				continue
			}
			seenTerm[tok] = struct{}{}
			normTerms = append(normTerms, tok)
		}
	}
	if len(normTerms) == 0 {
		return querySeedResult{}
	}
	idf := computeIDF(candidates, normTerms)
	joined := strings.Join(normTerms, " ")
	joinedW := 1.0
	for _, t := range normTerms {
		if w := idf[t]; w > joinedW {
			joinedW = w
		}
	}
	type scored struct {
		cand  seedCandidate
		score float64
	}
	var ranked []scored
	bestByTerm := map[string]scored{}

	for _, cand := range candidates {
		normLabel := strings.ToLower(cand.node.Name)
		bareLabel := strings.TrimRight(normLabel, "()")
		labelTokens := strings.Join(searchTokens(cand.node.Name), " ")
		qnLower := strings.ToLower(cand.node.QualifiedName)
		source := strings.ToLower(cand.node.Location.File)
		score := 0.0
		if joined != "" {
			if joined == normLabel || joined == bareLabel || joined == labelTokens || joined == qnLower || strings.HasSuffix(qnLower, "."+joined) {
				score += exactMatchBonus * 10 * joinedW
			} else if strings.HasPrefix(normLabel, joined) || strings.HasPrefix(bareLabel, joined) || strings.HasPrefix(labelTokens, joined) || strings.HasSuffix(qnLower, "."+joined) {
				score += prefixMatchBonus * 10 * joinedW
			}
		}
		// Dotted phrase exact boost against qualified_name / Name.Name
		for _, term := range terms {
			if strings.Contains(term, ".") {
				w := idf[term]
				if w == 0 {
					w = 1
				}
				if strings.EqualFold(cand.node.QualifiedName, term) || strings.HasSuffix(strings.ToLower(cand.node.QualifiedName), "."+strings.ToLower(term)) {
					score += exactMatchBonus * 20 * w
				} else if strings.EqualFold(cand.node.Name, lastDotted(term)) && strings.Contains(strings.ToLower(cand.node.QualifiedName), strings.ToLower(strings.TrimSuffix(term, "."+lastDotted(term)))) {
					score += exactMatchBonus * 15 * w
				}
			}
		}
		matched := 0
		tiered := 0.0
		for _, t := range normTerms {
			w := idf[t]
			if w == 0 {
				w = 1
			}
			tierValue := 0.0
			substrValue := 0.0
			sourceValue := 0.0
			if t == normLabel || t == bareLabel {
				tierValue = exactMatchBonus * w
				matched++
			} else if strings.HasPrefix(normLabel, t) || strings.HasPrefix(bareLabel, t) {
				tierValue = prefixMatchBonus * w
				matched++
			} else if strings.Contains(normLabel, t) || strings.Contains(qnLower, t) {
				substrValue = substringMatchBonus * w
				score += substrValue
				matched++
			}
			if strings.Contains(source, t) {
				sourceValue = sourceMatchBonus * w
				score += sourceValue
			}
			tiered += tierValue

			singleton := 0.0
			if t == normLabel || t == bareLabel || t == labelTokens || strings.HasSuffix(qnLower, "."+t) {
				singleton = exactMatchBonus * 10 * w
			} else if strings.HasPrefix(normLabel, t) || strings.HasPrefix(bareLabel, t) || strings.HasPrefix(labelTokens, t) {
				singleton = prefixMatchBonus * 10 * w
			}
			singleton += tierValue + substrValue + sourceValue
			if singleton > 0 {
				cur, ok := bestByTerm[t]
				better := !ok || singleton > cur.score ||
					(singleton == cur.score && cand.degree > cur.cand.degree) ||
					(singleton == cur.score && cand.degree == cur.cand.degree && len(cand.node.Name) < len(cur.cand.node.Name))
				if better {
					bestByTerm[t] = scored{cand: cand, score: singleton}
				}
			}
		}
		if tiered > 0 {
			score += tiered * math.Pow(float64(matched)/float64(len(normTerms)), 2)
		}
		// Prefer exported callables on ties via small label boost.
		switch cand.node.Label {
		case "Method", "Function", "Constructor":
			if score > 0 {
				score += 0.01
			}
		}
		if score > 0 {
			ranked = append(ranked, scored{cand: cand, score: score})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		if len(ranked[i].cand.node.Name) != len(ranked[j].cand.node.Name) {
			return len(ranked[i].cand.node.Name) < len(ranked[j].cand.node.Name)
		}
		return ranked[i].cand.node.QualifiedName < ranked[j].cand.node.QualifiedName
	})

	toRanked := func(item scored) api.RankedNode {
		return api.RankedNode{Node: item.cand.node, Score: item.score}
	}
	out := querySeedResult{ranked: make([]api.RankedNode, 0, len(ranked)), bestSeedByTerm: map[string]api.RankedNode{}}
	for _, item := range ranked {
		out.ranked = append(out.ranked, toRanked(item))
	}
	for term, item := range bestByTerm {
		out.bestSeedByTerm[term] = toRanked(item)
	}
	out.seeds = pickQuerySeeds(out.ranked, out.bestSeedByTerm)
	out.primary = pickQuerySeeds(out.ranked, nil)
	if len(out.primary) > seedMaxK {
		out.primary = out.primary[:seedMaxK]
	}
	return out
}

func pickQuerySeeds(ranked []api.RankedNode, bestByTerm map[string]api.RankedNode) []api.RankedNode {
	if len(ranked) == 0 {
		return nil
	}
	top := ranked[0].Score
	var seeds []api.RankedNode
	seenLabel := map[string]struct{}{}
	labelKey := func(n api.RankedNode) string {
		return strings.ToLower(n.Name)
	}
	for _, item := range ranked {
		if len(seeds) >= seedMaxK {
			break
		}
		if len(seeds) > 0 && item.Score < top*seedGapRatio {
			break
		}
		key := labelKey(item)
		if _, ok := seenLabel[key]; ok {
			continue
		}
		seenLabel[key] = struct{}{}
		seeds = append(seeds, item)
	}
	terms := make([]string, 0, len(bestByTerm))
	for term := range bestByTerm {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	for _, term := range terms {
		item := bestByTerm[term]
		key := labelKey(item)
		if _, ok := seenLabel[key]; ok {
			continue
		}
		dup := false
		for _, seed := range seeds {
			if seed.QualifiedName == item.QualifiedName {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		seenLabel[key] = struct{}{}
		seeds = append(seeds, item)
	}
	return seeds
}

func searchTokens(value string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		tok := strings.ToLower(b.String())
		b.Reset()
		if tok != "" {
			out = append(out, tok)
		}
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		flush()
	}
	flush()
	return out
}

func (s *Store) nodeDegrees(ctx context.Context, project string) (map[int64]int, error) {
	degRows, err := s.db.QueryContext(ctx, `SELECT source_id, COUNT(*) FROM edges WHERE project=? GROUP BY source_id
		UNION ALL SELECT target_id, COUNT(*) FROM edges WHERE project=? GROUP BY target_id`, project, project)
	if err != nil {
		return nil, err
	}
	defer degRows.Close()
	degree := map[int64]int{}
	for degRows.Next() {
		var id int64
		var count int
		if err := degRows.Scan(&id, &count); err != nil {
			return nil, err
		}
		degree[id] += count
	}
	return degree, degRows.Err()
}

func lastDotted(value string) string {
	if i := strings.LastIndex(value, "."); i >= 0 && i+1 < len(value) {
		return value[i+1:]
	}
	return value
}

func packagePrefix(qn string) string {
	if i := strings.LastIndex(qn, "."); i > 0 {
		return qn[:i]
	}
	return qn
}
