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

// Question ranking is BM25F over five fields of a symbol: its name, the
// module part of its qualified name, its file path, its docstring and the
// identifiers its body uses. A question describes behavior ("strip unused
// annotations from count queries"); the code that does it is named for one
// word of that at most, but its body uses the rest. Name-only scoring put
// every method called count above the one that builds the count query.
//
// Two adjustments sit on top of the field sum. Coverage scales a symbol by
// the share of the question (IDF-weighted) it matches, so one exact common
// word does not beat several rarer ones. An identifier the question spells
// as code (QuerySet, get_choices, count()) multiplies a symbol of that name.
const (
	queryRankPool        = 2000
	queryRankK1          = 1.2
	queryRankIdentBoost  = 3.0
	queryRankVendorScale = 0.05
)

const (
	rankFieldName = iota
	rankFieldModule
	rankFieldPath
	rankFieldDoc
	rankFieldBody
	rankFieldCount
)

var (
	rankFieldWeight = [rankFieldCount]float64{3, 1.5, 1, 1.5, 1}
	// Length normalization is light for names and paths, where length is
	// structure, and fuller for prose and bodies, where it is verbosity.
	rankFieldB = [rankFieldCount]float64{0.3, 0.5, 0.5, 0.75, 0.75}
)

// queryRankStopwords are question words that say nothing about code. get,
// set and use open half the names in a codebase, so they are dropped too;
// an identifier such as get_choices still matches through its other part
// and the identifier boost.
var queryRankStopwords = map[string]bool{
	"about": true, "after": true, "all": true, "also": true, "and": true, "any": true, "are": true,
	"before": true, "both": true, "but": true, "can": true, "could": true, "did": true, "does": true,
	"each": true, "explain": true, "for": true, "from": true, "get": true, "gets": true, "had": true,
	"has": true, "have": true, "here": true, "how": true, "into": true, "its": true, "just": true,
	"may": true, "might": true, "more": true, "must": true, "not": true, "off": true, "only": true,
	"onto": true, "our": true, "please": true, "set": true, "shall": true, "should": true, "show": true,
	"some": true, "such": true, "tell": true, "than": true, "that": true, "the": true, "their": true,
	"them": true, "then": true, "there": true, "these": true, "they": true, "this": true, "those": true,
	"too": true, "use": true, "used": true, "using": true, "very": true, "via": true, "was": true,
	"were": true, "what": true, "when": true, "where": true, "which": true, "while": true, "who": true,
	"whom": true, "why": true, "will": true, "with": true, "without": true, "would": true, "yes": true,
	"you": true, "your": true,
}

var queryRankCallToken = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\(`)

// rankTokens splits text the way a reader splits identifiers: on
// punctuation and underscores, at camelCase and letter-digit boundaries,
// lowercased and stemmed with ftsStem.
func rankTokens(text string) []string {
	var out []string
	var word []rune
	flush := func() {
		if len(word) >= 2 {
			out = append(out, ftsStem(strings.ToLower(string(word))))
		}
		word = word[:0]
	}
	runes := []rune(text)
	for i, r := range runes {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			flush()
			continue
		}
		if len(word) > 0 {
			prev := runes[i-1]
			switch {
			case unicode.IsUpper(r) && unicode.IsLower(prev),
				unicode.IsDigit(r) != unicode.IsDigit(prev),
				unicode.IsUpper(r) && unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1]):
				flush()
			}
		}
		word = append(word, r)
	}
	flush()
	return out
}

// queryRankTerms are the distinct content tokens of a question.
func queryRankTerms(question string) []string {
	var terms []string
	seen := map[string]bool{}
	for _, tok := range rankTokens(question) {
		if len([]rune(tok)) < 3 || queryRankStopwords[tok] || ftsStopwords[tok] || seen[tok] {
			continue
		}
		seen[tok] = true
		terms = append(terms, tok)
	}
	return terms
}

// queryRankIdents are the names a question spells as code: CamelCase or
// ALLCAPS words, snake_case words, dotted parts of those, and name( calls.
func queryRankIdents(question string) map[string]bool {
	out := map[string]bool{}
	for _, name := range queryProperNames(question) {
		for _, part := range strings.Split(name, ".") {
			if queryLooksProper(part) || strings.Contains(part, "_") {
				out[strings.ToLower(part)] = true
			}
		}
	}
	for _, tok := range queryWordToken.FindAllString(question, -1) {
		if strings.Trim(tok, "_") != "" && strings.Contains(strings.Trim(tok, "_"), "_") {
			out[strings.ToLower(tok)] = true
		}
	}
	for _, m := range queryRankCallToken.FindAllStringSubmatch(question, -1) {
		out[strings.ToLower(m[1])] = true
	}
	return out
}

type rankDoc struct {
	node   api.Node
	fields [rankFieldCount][]string
}

func newRankDoc(n api.Node) rankDoc {
	doc, _ := n.Properties["docstring"].(string)
	bt, _ := n.Properties["bt"].(string)
	file := n.Location.File
	if dot := strings.LastIndex(file, "."); dot > strings.LastIndex(file, "/") {
		file = file[:dot]
	}
	var d rankDoc
	d.node = n
	d.fields[rankFieldName] = rankTokens(n.Name)
	d.fields[rankFieldModule] = rankTokens(packagePrefix(n.QualifiedName))
	d.fields[rankFieldPath] = rankTokens(file)
	d.fields[rankFieldDoc] = rankTokens(doc)
	d.fields[rankFieldBody] = rankTokens(bt)
	return d
}

// rankQueryRows ranks the symbols a question's words retrieve. Rows a reader
// cannot use (tests unless asked, synthetic and structural nodes) are left
// out, and dependency trees sort last.
func (s *Store) rankQueryRows(ctx context.Context, project, question string, withTests bool) ([]api.Node, error) {
	terms := queryRankTerms(question)
	if len(terms) == 0 {
		return nil, nil
	}
	pool, err := s.queryRankPool(ctx, project, terms, withTests)
	if err != nil || len(pool) == 0 {
		return nil, err
	}
	df, total, err := s.queryRankDocFreq(ctx, project, terms)
	if err != nil {
		return nil, err
	}
	return scoreRankDocs(pool, terms, df, total, queryRankIdents(question), question), nil
}

func scoreRankDocs(pool []rankDoc, terms []string, df map[string]int, total int, idents map[string]bool, question string) []api.Node {
	var avg [rankFieldCount]float64
	for _, d := range pool {
		for f := range d.fields {
			avg[f] += float64(len(d.fields[f]))
		}
	}
	for f := range avg {
		avg[f] = math.Max(avg[f]/float64(len(pool)), 1e-9)
	}
	idf := map[string]float64{}
	idfSum := 0.0
	for _, t := range terms {
		n := float64(df[t])
		idf[t] = math.Log(1 + (float64(total)-n+0.5)/(n+0.5))
		idfSum += idf[t]
	}
	type scored struct {
		node  api.Node
		score float64
	}
	var ranked []scored
	for _, d := range pool {
		score, covered := 0.0, 0.0
		for _, t := range terms {
			tf := 0.0
			for f, toks := range d.fields {
				count := 0
				for _, tok := range toks {
					if tok == t {
						count++
					}
				}
				if count > 0 {
					norm := 1 - rankFieldB[f] + rankFieldB[f]*float64(len(toks))/avg[f]
					tf += rankFieldWeight[f] * float64(count) / norm
				}
			}
			if tf > 0 {
				covered += idf[t]
				score += idf[t] * tf * (queryRankK1 + 1) / (tf + queryRankK1)
			}
		}
		if score <= 0 {
			continue
		}
		if idfSum > 0 {
			score *= covered / idfSum
		}
		if idents[strings.ToLower(d.node.Name)] {
			score *= queryRankIdentBoost
		}
		if seg := queryVendoredSegment(d.node.Location.File); seg != "" && !queryMentionsTerm(question, nil, seg) {
			score *= queryRankVendorScale
		}
		ranked = append(ranked, scored{node: d.node, score: score})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].node.QualifiedName < ranked[j].node.QualifiedName
	})
	out := make([]api.Node, len(ranked))
	for i, r := range ranked {
		out[i] = r.node
	}
	return out
}

// queryRankFTS ORs the terms for FTS. The index holds unstemmed words, so a
// stem of four or more letters prefix-matches its forms.
func queryRankFTS(terms []string) string {
	parts := make([]string, 0, len(terms))
	for _, t := range terms {
		parts = append(parts, queryRankFTSTerm(t))
	}
	return strings.Join(parts, " OR ")
}

func queryRankFTSTerm(t string) string {
	quoted := `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
	if len([]rune(t)) >= 4 {
		quoted += "*"
	}
	return quoted
}

// queryRankPool fetches the symbols any term matches, best FTS score first.
func (s *Store) queryRankPool(ctx context.Context, project string, terms []string, withTests bool) ([]rankDoc, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT n.id,n.project,n.label,n.name,n.qualified_name,n.file_path,
		n.start_line,n.start_column,n.end_line,n.end_column,n.properties
		FROM nodes_fts JOIN nodes n ON n.id=nodes_fts.rowid
		WHERE nodes_fts MATCH ? AND n.project=? AND n.label NOT IN ('File','Folder','Module','Section','Project')
		ORDER BY bm25(nodes_fts,3.0,1.5,0.0,1.0,1.0) LIMIT ?`, queryRankFTS(terms), project, queryRankPool)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pool []rankDoc
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		if !queryRowEligible(n, withTests) {
			continue
		}
		pool = append(pool, newRankDoc(n))
	}
	return pool, rows.Err()
}

// queryRankDocFreq counts the symbols each term matches, and all symbols.
func (s *Store) queryRankDocFreq(ctx context.Context, project string, terms []string) (map[string]int, int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM nodes WHERE project=? AND label NOT IN ('File','Folder','Module','Section','Project')`, project).Scan(&total); err != nil {
		return nil, 0, err
	}
	df := map[string]int{}
	for _, t := range terms {
		var n int
		if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM nodes_fts WHERE nodes_fts MATCH ?`, queryRankFTSTerm(t)).Scan(&n); err != nil {
			return nil, 0, err
		}
		df[t] = n
	}
	return df, total, nil
}

// queryRowEligible is true for a row an agent can open and use: a real
// symbol with a file, not a test unless the question asks for tests.
func queryRowEligible(n api.Node, withTests bool) bool {
	if isSyntheticQueryNode(n) || skipDataLanguageVariable(n) || n.Label == "Folder" || n.Label == "Section" ||
		strings.TrimSpace(n.Location.File) == "" {
		return false
	}
	return withTests || !queryNodeLooksLikeTest(n)
}
