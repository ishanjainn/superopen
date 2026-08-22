package engine

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// Superopen graph query expansion and text rendering.
const (
	queryDefaultBudget  = 2000
	queryCharsPerToken  = 3
	queryHubDegreeFloor = 50
	// Walk neighbors up to a high cap; hub skip keeps transit noise down.
	queryNeighborLimit = 10000
)

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
			neighbors, err := s.neighbors(ctx, node, "both", nil, queryNeighborLimit)
			if err != nil {
				return nil, err
			}
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

func orderQueryNodes(seedOrder []int64, nodesByID map[int64]queryNodeHit) []queryNodeHit {
	ordered := make([]queryNodeHit, 0, len(nodesByID))
	for _, id := range seedOrder {
		if hit, ok := nodesByID[id]; ok {
			ordered = append(ordered, hit)
		}
	}
	rest := make([]queryNodeHit, 0, len(nodesByID))
	for _, hit := range nodesByID {
		if hit.seed {
			continue
		}
		rest = append(rest, hit)
	}
	sort.SliceStable(rest, func(i, j int) bool {
		if rest[i].hop != rest[j].hop {
			return rest[i].hop < rest[j].hop
		}
		if rest[i].deg != rest[j].deg {
			return rest[i].deg > rest[j].deg
		}
		return rest[i].node.QualifiedName < rest[j].node.QualifiedName
	})
	return append(ordered, rest...)
}

func formatQueryNodeLine(hit queryNodeHit, communityByID map[int64]string) string {
	community := communityByID[hit.node.ID]
	if community == "" {
		community = packagePrefix(hit.node.QualifiedName)
	}
	qnField := ""
	if qn := strings.TrimSpace(hit.node.QualifiedName); qn != "" && hit.node.Label != "File" && hit.node.Label != "Folder" {
		qnField = "qn=" + qn + " "
	}
	return fmt.Sprintf("NODE %s [%ssrc=%s loc=%s community=%s]\n",
		queryNodeDisplayName(hit.node), qnField, hit.node.Location.File, queryNodeLoc(hit.node), community)
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

func applyQueryBudget(header, body string, seedCount int, ordered []queryNodeHit, communityByID map[int64]string, budget, maxChars int) (string, bool) {
	output := header + body
	if len(output) <= maxChars {
		return output, false
	}
	cutAt := strings.LastIndex(output[:maxChars], "\n")
	if cutAt < len(header) {
		cutAt = maxChars
	}
	seedBlockEnd := len(header)
	for i := 0; i < seedCount && i < len(ordered); i++ {
		seedBlockEnd += len(formatQueryNodeLine(ordered[i], communityByID))
	}
	if cutAt < seedBlockEnd {
		cutAt = seedBlockEnd
		if cutAt > len(output) {
			cutAt = len(output)
		}
	}
	shownNodes := strings.Count(output[:cutAt], "NODE ")
	totalNodes := len(ordered)
	cutCount := totalNodes - shownNodes
	if cutCount < 0 {
		cutCount = 0
	}
	if cutCount == 0 {
		// Every NODE fits within budget: return the full answer (edges included)
		// with an over-budget notice — never a "0 cut nodes" truncation.
		totalEdges := strings.Count(output, "EDGE ")
		estTokens := len(output) / queryCharsPerToken
		return fmt.Sprintf(
			"[i] Complete answer over budget: all %d nodes and %d edges shown (~%d tokens vs the requested ~%d-token budget). Edges are never dropped once every node fits — this is already the full answer. Run `so graph snippet <qn>` on a NODE below, or narrow the question. Do not pipe through head/tail.\n\n%s",
			totalNodes, totalEdges, estTokens, budget, output,
		), false
	}
	output = fmt.Sprintf(
		"[!] TRUNCATED: showing %d of %d nodes (~%d-token budget). The answer may be among the %d cut nodes — run `so graph snippet <qn>` from a NODE below, or narrow the question. Read a NODE src= path for the file.\n\n%s\n... (%d more nodes omitted.)",
		shownNodes, totalNodes, budget, cutCount, output[:cutAt], cutCount,
	)
	return output, true
}
