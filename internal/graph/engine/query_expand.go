package engine

import (
	"context"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// Superopen graph query expansion.
const (
	queryHubDegreeFloor = 50
	queryMaxNodeRows    = 16
	// Fetch a window, rank by same-file / same-package / CALLS, keep this many.
	queryNeighborFetch = 200
	queryNeighborKeep  = 48
)

func queryRowCap() int {
	if v := strings.TrimSpace(os.Getenv("SUPEROPEN_GRAPH_QUERY_MAX_ROWS")); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil && n > 0 {
			return n
		}
	}
	return queryMaxNodeRows
}

func hubThreshold(degrees map[int64]int) int {
	if len(degrees) == 0 {
		return queryHubDegreeFloor
	}
	sorted := make([]int, 0, len(degrees))
	for _, d := range degrees {
		sorted = append(sorted, d)
	}
	sort.Ints(sorted)
	idx := int(float64(len(sorted)) * 0.99)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	th := sorted[idx]
	if th < queryHubDegreeFloor {
		return queryHubDegreeFloor
	}
	return th
}

type queryNodeHit struct {
	node   api.Node
	hop    int
	seed   bool
	screen bool
	deg    int
}

func (s *Store) queryExpandBFS(
	ctx context.Context,
	expand []api.RankedNode,
	seedIDs map[int64]bool,
	depth int,
	degrees map[int64]int,
	nodesByID map[int64]queryNodeHit,
	terms []string,
) error {
	threshold := hubThreshold(degrees)
	frontier := make([]api.Node, 0, len(expand))
	for _, seed := range expand {
		frontier = append(frontier, seed.Node)
	}
	for hop := 0; hop < depth; hop++ {
		var next []api.Node
		seenNext := map[int64]bool{}
		for _, node := range frontier {
			// Don't expand through hubs except seeds.
			if !seedIDs[node.ID] && degrees[node.ID] >= threshold {
				continue
			}
			neighbors, err := s.neighbors(ctx, node, "both", nil, queryNeighborFetch)
			if err != nil {
				return err
			}
			neighbors = clipQueryNeighbors(node, neighbors, queryNeighborKeep, terms)
			for _, item := range neighbors {
				if skipDataLanguageVariable(item.node) && !seedIDs[item.node.ID] {
					continue
				}
				if _, ok := nodesByID[item.node.ID]; ok {
					existing := nodesByID[item.node.ID]
					if !existing.seed && hop+1 < existing.hop {
						existing.hop = hop + 1
						nodesByID[item.node.ID] = existing
					}
					continue
				}
				nodesByID[item.node.ID] = queryNodeHit{
					node: item.node,
					hop:  hop + 1,
					deg:  degrees[item.node.ID],
				}
				if !seenNext[item.node.ID] {
					seenNext[item.node.ID] = true
					next = append(next, item.node)
				}
			}
		}
		frontier = next
	}
	return nil
}

func preferQueryNeighbor(from api.Node, item neighbor, terms []string) int {
	score := 0
	if item.node.Location.File != "" && item.node.Location.File == from.Location.File {
		score += 100
	}
	fromPkg := packagePrefix(from.QualifiedName)
	toPkg := packagePrefix(item.node.QualifiedName)
	if fromPkg != "" && fromPkg == toPkg {
		score += 40
	}
	switch item.edge.Type {
	case "CALLS", "DEFINES", "DEFINES_METHOD", "INHERITS", "IMPLEMENTS":
		score += 20
	}
	score += queryNameOverlap(item.node, terms)
	return score
}

func clipQueryNeighbors(from api.Node, items []neighbor, keep int, terms []string) []neighbor {
	if keep <= 0 || len(items) <= keep {
		return items
	}
	sorted := append([]neighbor(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool {
		si, sj := preferQueryNeighbor(from, sorted[i], terms), preferQueryNeighbor(from, sorted[j], terms)
		if si != sj {
			return si > sj
		}
		return sorted[i].node.QualifiedName < sorted[j].node.QualifiedName
	})
	return sorted[:keep]
}

func orderQueryNodes(seedOrder []int64, nodesByID map[int64]queryNodeHit, terms []string) []queryNodeHit {
	ordered := make([]queryNodeHit, 0, len(nodesByID))
	for _, id := range seedOrder {
		if hit, ok := nodesByID[id]; ok && !isSyntheticQueryFile(hit.node.Location.File) {
			ordered = append(ordered, hit)
		}
	}
	rest := make([]queryNodeHit, 0, len(nodesByID))
	for _, hit := range nodesByID {
		if hit.seed || isSyntheticQueryFile(hit.node.Location.File) {
			continue
		}
		rest = append(rest, hit)
	}
	sort.SliceStable(rest, func(i, j int) bool {
		if rest[i].hop != rest[j].hop {
			return rest[i].hop < rest[j].hop
		}
		oi, oj := queryNameOverlap(rest[i].node, terms), queryNameOverlap(rest[j].node, terms)
		if oi != oj {
			return oi > oj
		}
		if rest[i].deg != rest[j].deg {
			return rest[i].deg > rest[j].deg
		}
		return rest[i].node.QualifiedName < rest[j].node.QualifiedName
	})
	return append(ordered, rest...)
}

// queryNameOverlap scores a symbol against the question. Only the short name
// and last qualified-name segment count, so package path tokens (models,
// contrib) do not promote every node in that directory.
func queryNameOverlap(n api.Node, terms []string) int {
	if len(terms) == 0 {
		return 0
	}
	name, last := querySymbolNames(n)
	score := 0
	for _, t := range terms {
		tl := strings.ToLower(strings.TrimSpace(t))
		if len(tl) < 4 {
			continue
		}
		if name == tl || last == tl {
			score += 10
			continue
		}
		if strings.Contains(name, tl) || strings.Contains(last, tl) {
			score += 4
			continue
		}
		if strings.HasSuffix(tl, "s") && len(tl) > 5 {
			stem := strings.TrimSuffix(tl, "s")
			if strings.Contains(name, stem) || strings.Contains(last, stem) {
				score += 3
			}
		}
	}
	return score
}

func isSyntheticQueryNode(n api.Node) bool {
	qn := strings.TrimSpace(n.QualifiedName)
	if strings.HasPrefix(qn, "<") {
		return true
	}
	return isSyntheticQueryFile(n.Location.File)
}

func isSyntheticQueryFile(path string) bool {
	p := strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
	if p == "" {
		return false
	}
	base := p
	if i := strings.LastIndex(p, "/"); i >= 0 {
		base = p[i+1:]
	}
	if strings.HasPrefix(base, "<") && strings.HasSuffix(base, ">") {
		return true
	}
	return strings.Contains(p, "<python-builtins>") || strings.Contains(p, "<builtins>")
}

func queryNodeSpan(n api.Node) int {
	if n.Location.StartLine <= 0 {
		return 0
	}
	end := n.Location.EndLine
	if end < n.Location.StartLine {
		return 1
	}
	return end - n.Location.StartLine + 1
}

func queryWideType(n api.Node) bool {
	switch n.Label {
	case "Class", "Module":
		return queryNodeSpan(n) > 80
	default:
		return false
	}
}

// spliceWideTypeMethods keeps seed Class/Module rows, then adds same-file
// methods the question actually names so snippet stays within its line cap
// instead of pointing at a 1700-line class. Methods with no name overlap stay
// off the first screen. The 16-row cap still applies to result.Nodes.
func (s *Store) spliceWideTypeMethods(ctx context.Context, ordered []queryNodeHit, terms []string) []queryNodeHit {
	if len(ordered) == 0 {
		return ordered
	}
	seen := map[int64]bool{}
	out := make([]queryNodeHit, 0, len(ordered)+queryRowCap())
	var wide []queryNodeHit
	for _, hit := range ordered {
		if !hit.seed {
			continue
		}
		if seen[hit.node.ID] {
			continue
		}
		hit.screen = true
		out = append(out, hit)
		seen[hit.node.ID] = true
		if queryWideType(hit.node) {
			wide = append(wide, hit)
		}
	}
	for _, owner := range wide {
		remain := queryRowCap() - len(out)
		if remain <= 0 {
			break
		}
		for _, m := range s.wideTypeMethods(ctx, owner.node, seen, remain, terms) {
			if queryNameOverlap(m.node, terms) <= 0 {
				continue
			}
			m.screen = true
			out = append(out, m)
			seen[m.node.ID] = true
		}
	}
	for _, hit := range ordered {
		if seen[hit.node.ID] {
			continue
		}
		out = append(out, hit)
		seen[hit.node.ID] = true
		if !queryWideType(hit.node) {
			continue
		}
		remain := queryRowCap() - len(out)
		if remain <= 0 {
			continue
		}
		for _, m := range s.wideTypeMethods(ctx, hit.node, seen, remain, terms) {
			out = append(out, m)
			seen[m.node.ID] = true
		}
	}
	return out
}

func (s *Store) wideTypeMethods(ctx context.Context, owner api.Node, seen map[int64]bool, limit int, terms []string) []queryNodeHit {
	if limit <= 0 {
		return nil
	}
	var cand []queryNodeHit
	picked := map[int64]bool{}
	file := owner.Location.File
	appendItems := func(items []neighbor) {
		for _, item := range items {
			if seen[item.node.ID] || picked[item.node.ID] {
				continue
			}
			if skipDataLanguageVariable(item.node) {
				continue
			}
			if file != "" && item.node.Location.File != "" && item.node.Location.File != file {
				continue
			}
			switch item.node.Label {
			case "Method", "Function":
			default:
				continue
			}
			picked[item.node.ID] = true
			cand = append(cand, queryNodeHit{node: item.node, hop: 1})
		}
	}
	if items, err := s.neighbors(ctx, owner, "outgoing", []string{"DEFINES_METHOD", "DEFINES"}, queryNeighborFetch); err == nil {
		appendItems(items)
	}
	sort.SliceStable(cand, func(i, j int) bool {
		si, sj := queryNameOverlap(cand[i].node, terms), queryNameOverlap(cand[j].node, terms)
		if si != sj {
			return si > sj
		}
		return cand[i].node.QualifiedName < cand[j].node.QualifiedName
	})
	if len(cand) > limit {
		cand = cand[:limit]
	}
	return cand
}

// screenQueryNodes is the first answer: seeds, plus same-file methods of a
// wide seed class. BFS neighbors stay on result.Nodes and off this list.
// omitUnmentionedTests drops test nodes from the first screen. The capped
// Nodes page is unchanged. A question that names tests keeps them.
func omitUnmentionedTests(screen []queryNodeHit, question string, terms []string) []queryNodeHit {
	if queryMentionsTests(question, terms) || len(screen) == 0 {
		return screen
	}
	n := 0
	for _, hit := range screen {
		if queryNodeLooksLikeTest(hit.node) {
			continue
		}
		screen[n] = hit
		n++
	}
	return screen[:n]
}

// fillEmptyScreen lists nodes when the first screen was emptied. A broad
// question can match only tests, or only neighbors that were not seeds.
// The footer then says to snippet a node that was never printed.
func fillEmptyScreen(ordered []queryNodeHit, question string, terms []string) []queryNodeHit {
	if len(ordered) == 0 {
		return nil
	}
	capN := queryRowCap()
	var primary, tests []queryNodeHit
	mentionTests := queryMentionsTests(question, terms)
	for _, hit := range ordered {
		if !mentionTests && queryNodeLooksLikeTest(hit.node) {
			tests = append(tests, hit)
			continue
		}
		primary = append(primary, hit)
		if len(primary) >= capN {
			break
		}
	}
	if len(primary) > 0 {
		return primary
	}
	if len(tests) > capN {
		tests = tests[:capN]
	}
	return tests
}

func screenQueryNodes(ordered []queryNodeHit) []queryNodeHit {
	seen := map[int64]bool{}
	out := make([]queryNodeHit, 0, len(ordered))
	for _, hit := range ordered {
		if !hit.seed && !hit.screen {
			continue
		}
		if hit.node.ID != 0 {
			if seen[hit.node.ID] {
				continue
			}
			seen[hit.node.ID] = true
		}
		out = append(out, hit)
	}
	return out
}
