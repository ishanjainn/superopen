package engine

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// Superopen graph query expansion and text rendering.
const (
	queryDefaultBudget  = 1200
	queryCharsPerToken  = 3
	queryBodyReserve    = 1200
	queryHubDegreeFloor = 50
	queryMaxNodeRows    = 16
	queryMaxEdgeRows    = 16
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
	node api.Node
	hop  int
	seed bool
	deg  int
}

func (s *Store) queryExpandBFS(
	ctx context.Context,
	expand []api.RankedNode,
	seedIDs map[int64]bool,
	depth int,
	degrees map[int64]int,
	nodesByID map[int64]queryNodeHit,
	seenEdges map[int64]bool,
	edges *[]api.Edge,
	terms []string,
) ([]string, error) {
	threshold := hubThreshold(degrees)
	var edgeLines []string
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
				return nil, err
			}
			neighbors = clipQueryNeighbors(node, neighbors, queryNeighborKeep, terms)
			for _, item := range neighbors {
				if skipDataLanguageVariable(item.node) && !seedIDs[item.node.ID] {
					continue
				}
				if item.edge.ID != 0 && !seenEdges[item.edge.ID] {
					seenEdges[item.edge.ID] = true
					if edges != nil {
						*edges = append(*edges, item.edge)
					}
					from, to := node.Name, item.node.Name
					if item.edge.SourceID == item.node.ID {
						from, to = to, from
					}
					at := ""
					if item.edge.Evidence != nil && item.edge.Evidence.Location != nil && item.edge.Evidence.Location.StartLine > 0 {
						at = fmt.Sprintf(" at=%s:L%d", item.edge.Evidence.Location.File, item.edge.Evidence.Location.StartLine)
					}
					line := fmt.Sprintf("EDGE %s --%s --> %s%s", from, item.edge.Type, to, at)
					edgeLines = append(edgeLines, line)
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
	return edgeLines, nil
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
		}
	}
	return score
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

func formatQueryNodeLine(hit queryNodeHit) string {
	return formatQueryNodeLineFact(hit, true)
}

func formatQueryNodeLineFact(hit queryNodeHit, withFact bool) string {
	qnField := ""
	if qn := strings.TrimSpace(hit.node.QualifiedName); qn != "" && hit.node.Label != "File" && hit.node.Label != "Folder" {
		qnField = "qn=" + qn + " "
	}
	if withFact {
		if fact := queryNodeFact(hit.node); fact != "" {
			qnField += fact + " "
		}
	}
	return fmt.Sprintf("NODE %s [%ssrc=%s loc=%s]\n",
		queryNodeDisplayName(hit.node), qnField, hit.node.Location.File, queryNodeLoc(hit.node))
}

func queryNodeBody(ordered []queryNodeHit, withFact bool) string {
	var b strings.Builder
	for _, hit := range ordered {
		b.WriteString(formatQueryNodeLineFact(hit, withFact))
	}
	return b.String()
}

func queryNodeBodyFit(ordered []queryNodeHit, header, edges string, maxChars int) string {
	var facts, plain strings.Builder
	for _, hit := range ordered {
		facts.WriteString(formatQueryNodeLineFact(hit, true))
		plain.WriteString(formatQueryNodeLineFact(hit, false))
	}
	withFacts := facts.String()
	if maxChars <= 0 || len(header)+len(withFacts)+len(edges) <= maxChars {
		return withFacts
	}
	return plain.String()
}

// preferQueryNodeBody keeps ~40-char signature/docstring facts only when they
// fit the existing token cap. Do not raise queryDefaultBudget to make room.
func preferQueryNodeBody(header, withFacts, plain, edges string, maxChars int) string {
	if maxChars <= 0 || len(header)+len(withFacts)+len(edges) <= maxChars {
		return withFacts
	}
	return plain
}

func queryNodeFact(n api.Node) string {
	raw := strings.TrimSpace(queryNodePropString(n, "signature"))
	if raw == "" {
		raw = firstQueryDocLine(queryNodePropString(n, "docstring"))
	}
	raw = strings.Join(strings.Fields(raw), " ")
	if raw == "" {
		return ""
	}
	const maxFact = 40
	if len(raw) > maxFact {
		raw = strings.TrimSpace(raw[:maxFact])
	}
	return "sig=" + raw
}

func queryNodePropString(n api.Node, key string) string {
	if n.Properties == nil {
		return ""
	}
	v, ok := n.Properties[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func firstQueryDocLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
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

// spliceWideTypeMethods keeps seed Class/Module rows, then fills remaining
// NODE slots with same-file methods so snippet stays within the 80-line cap
// instead of pointing at a 1700-line class. The 16-row cap still happens
// after this reorder so hub queries keep their TRUNCATED signal.
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

func queryNodeLoc(node api.Node) string {
	start := node.Location.StartLine
	end := node.Location.EndLine
	if start <= 0 {
		return "-"
	}
	if end > start {
		return fmt.Sprintf("L%d-%d", start, end)
	}
	return fmt.Sprintf("L%d", start)
}

func applyQueryBudget(header, nodeBody, edgeBody string, seedCount int, ordered []queryNodeHit, budget, maxChars, listed int) (string, bool) {
	edgeReserve := 0
	if strings.TrimSpace(edgeBody) != "" {
		edgeReserve = maxChars / 4
		if edgeReserve > len(edgeBody)+1 {
			edgeReserve = len(edgeBody) + 1
		}
		if edgeReserve < 96 {
			edgeReserve = 96
		}
		if edgeReserve > maxChars/3 {
			edgeReserve = maxChars / 3
		}
	}
	nodeCap := maxChars - edgeReserve
	if nodeCap < len(header)+64 {
		nodeCap = maxChars * 3 / 4
	}
	nodeOutput := header + nodeBody
	cutAt := len(nodeOutput)
	nodeTrunc := false
	if len(nodeOutput) > nodeCap {
		nodeTrunc = true
		cutAt = strings.LastIndex(nodeOutput[:nodeCap], "\n")
		if cutAt < len(header) {
			cutAt = nodeCap
			if cutAt > len(nodeOutput) {
				cutAt = len(nodeOutput)
			}
		}
		seedBlockEnd := len(header)
		for i := 0; i < seedCount && i < len(ordered); i++ {
			seedBlockEnd += len(formatQueryNodeLine(ordered[i]))
		}
		if cutAt < seedBlockEnd {
			cutAt = seedBlockEnd
			if cutAt > len(nodeOutput) {
				cutAt = len(nodeOutput)
			}
		}
		nodeOutput = nodeOutput[:cutAt]
	}
	remain := maxChars - len(nodeOutput)
	keptEdges := edgeBody
	if remain < len(edgeBody) {
		kept := edgeBody
		if remain > 0 {
			cut := strings.LastIndex(edgeBody[:min(remain, len(edgeBody))], "\n")
			if cut > 0 {
				kept = edgeBody[:cut+1]
			} else {
				kept = ""
			}
		} else {
			kept = ""
		}
		keptEdges = kept
	}
	body := nodeOutput
	if keptEdges != "" {
		if !strings.HasSuffix(body, "\n") {
			body += "\n"
		}
		body += keptEdges
	}
	shownNodes := strings.Count(body, "NODE ")
	pageNodes := len(ordered)
	foundNodes := listed
	if foundNodes < pageNodes {
		foundNodes = pageNodes
	}
	cutCount := pageNodes - shownNodes
	if cutCount < 0 {
		cutCount = 0
	}
	shownEdges := strings.Count(body, "EDGE ")
	if !nodeTrunc && shownNodes >= pageNodes && (edgeBody == "" || shownEdges == strings.Count(edgeBody, "EDGE ") || strings.Count(edgeBody, "EDGE ") == 0) {
		if len(header+nodeBody+edgeBody) <= maxChars {
			return header + nodeBody + edgeBody, false
		}
	}
	if cutCount == 0 && shownEdges > 0 {
		estTokens := len(body) / queryCharsPerToken
		return fmt.Sprintf(
			"[i] Complete answer over budget: all %d nodes and %d edges shown (~%d tokens vs the requested ~%d-token budget). Narrow the question if you need a smaller subgraph, or run `so graph snippet <qn>` on a NODE below. Do not pipe through head/tail.\n\n%s",
			pageNodes, shownEdges, estTokens, budget, body,
		), false
	}
	omitted := foundNodes - shownNodes
	if omitted < 0 {
		omitted = 0
	}
	return fmt.Sprintf(
		"[!] TRUNCATED: showing %d of %d listed nodes (~%d-token budget). Narrow the question first. The answer may be among the %d cut nodes — `so graph snippet <qn>` only for a NODE already shown.\n\n%s\n... (%d more nodes omitted.)",
		shownNodes, foundNodes, budget, omitted, strings.TrimRight(body, "\n"), omitted,
	), true
}
