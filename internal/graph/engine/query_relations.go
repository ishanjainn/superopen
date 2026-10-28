package engine

import (
	"context"
	"sort"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

const (
	queryRelationAnchors    = 8
	queryRelationPromotions = 4
	queryRelationRankWindow = 120
)

// prioritizeQueryRelations moves a few direct callees or overrides beside
// their high-ranked callers. Text ranking alone can put the decisive callee
// just past the first page even when the graph already knows the call.
func (s *Store) prioritizeQueryRelations(ctx context.Context, project string, rows []api.Node, question string, terms []string) ([]api.Node, error) {
	if len(rows) < 2 {
		return rows, nil
	}
	byID := make(map[int64]int, len(rows))
	for i, n := range rows {
		byID[n.ID] = i
	}
	limit := queryRelationAnchors
	if len(rows) < limit {
		limit = len(rows)
	}
	promoted := map[int64]bool{}
	after := map[int64][]api.Node{}
	withTests := queryMentionsTests(question, queryTerms(question, nil))
	for _, anchor := range rows[:limit] {
		if len(promoted) >= queryRelationPromotions {
			break
		}
		if !queryAttachableNode(anchor) || queryNodeLooksLikeTest(anchor) || anchor.ID == 0 {
			continue
		}
		neighbors, err := s.neighbors(ctx, anchor, "both", []string{"CALLS", "USAGE", "CONFIGURES", "OVERRIDE"}, queryNeighborFetch)
		if err != nil {
			return nil, err
		}
		type candidate struct {
			node  api.Node
			score int
		}
		var choices []candidate
		for _, item := range neighbors {
			n := item.node
			if edgeConfidence(item.edge) < traceMinConfidence {
				continue
			}
			if n.ID == 0 || n.ID == anchor.ID || promoted[n.ID] || !queryAttachableNode(n) ||
				!queryRowEligible(n, withTests) || queryNodeLooksLikeTest(n) || queryVendoredSegment(n.Location.File) != "" {
				continue
			}
			if item.edge.Type != "OVERRIDE" && item.edge.SourceID != anchor.ID {
				continue
			}
			rank, known := byID[n.ID]
			overlap := queryNameOverlap(n, terms)
			sameFile := n.Location.File == anchor.Location.File
			shortWrapper := anchor.Location.StartLine > 0 &&
				anchor.Location.EndLine-anchor.Location.StartLine <= 2 &&
				item.edge.Type == "CALLS"
			shortEntry := anchor.Location.StartLine > 0 &&
				anchor.Location.EndLine-anchor.Location.StartLine < 10 &&
				n.Location.EndLine-n.Location.StartLine >= 10 &&
				item.edge.Type == "CALLS"
			if sameFile && overlap == 0 && !shortWrapper && !shortEntry {
				continue
			}
			if (!known || rank >= queryRelationRankWindow) && !sameFile && overlap == 0 && len(neighbors) > 3 {
				continue
			}
			score := overlap * 5
			if known {
				score += queryRelationRankWindow - min(rank, queryRelationRankWindow)
			}
			if sameFile {
				score += 15
			}
			if item.edge.Type == "CALLS" {
				score += 80
			}
			if shortEntry && sameFile && item.edge.Evidence != nil && item.edge.Evidence.Location != nil {
				gap := anchor.Location.EndLine - item.edge.Evidence.Location.StartLine
				if gap >= 0 && gap <= 2 {
					score += (3 - gap) * 30
				}
			}
			span := n.Location.EndLine - n.Location.StartLine + 1
			if sameFile && span > 10 {
				score += min(span, 40)
			}
			choices = append(choices, candidate{node: n, score: score})
		}
		sort.Slice(choices, func(i, j int) bool {
			if choices[i].score != choices[j].score {
				return choices[i].score > choices[j].score
			}
			return strings.Compare(choices[i].node.QualifiedName, choices[j].node.QualifiedName) < 0
		})
		for _, choice := range choices {
			if len(after[anchor.ID]) >= 2 || len(promoted) >= queryRelationPromotions {
				break
			}
			if promoted[choice.node.ID] {
				continue
			}
			promoted[choice.node.ID] = true
			after[anchor.ID] = append(after[anchor.ID], choice.node)
		}
	}
	if len(promoted) == 0 {
		return rows, nil
	}
	out := make([]api.Node, 0, len(rows)+len(after))
	for _, n := range rows {
		if !promoted[n.ID] {
			out = append(out, n)
		}
		out = append(out, after[n.ID]...)
	}
	return out, nil
}
