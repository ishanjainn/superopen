package engine

import (
	"context"
	"sort"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

const queryLinkedLimit = 6

// queryLinkedNodes shows a few relevant call targets outside the current page.
// They cost one row each, leaving the 15-node page and source-body budget
// intact while giving the agent an exact qualified name to open next.
func (s *Store) queryLinkedNodes(ctx context.Context, rows []api.Node, bodies []api.QueryBody, question string) ([]api.QueryLink, error) {
	terms := queryOverlapTerms(question, nil)
	listed := map[int64]bool{}
	focus := map[string]bool{}
	for _, body := range bodies {
		focus[body.QualifiedName] = true
	}
	for _, n := range rows {
		listed[n.ID] = true
	}
	type candidate struct {
		link  api.QueryLink
		id    int64
		score int
	}
	byID := map[int64]candidate{}
	for i, anchor := range rows {
		if !queryAttachableNode(anchor) || queryNodeLooksLikeTest(anchor) || anchor.ID == 0 || len(focus) > 0 && !focus[anchor.QualifiedName] {
			continue
		}
		neighbors, err := s.neighbors(ctx, anchor, "outgoing", []string{"CALLS", "USAGE", "CONFIGURES", "OVERRIDE"}, queryNeighborFetch)
		if err != nil {
			return nil, err
		}
		for _, item := range neighbors {
			n := item.node
			if edgeConfidence(item.edge) < traceMinConfidence {
				continue
			}
			if listed[n.ID] || !queryAttachableNode(n) || !queryRowEligible(n, false) || queryVendoredSegment(n.Location.File) != "" {
				continue
			}
			overlap := queryNameOverlap(n, terms)
			if overlap == 0 {
				continue
			}
			score := overlap*10 + len(rows) - i
			if item.edge.Type == "CALLS" {
				score += 20
			}
			if n.Location.File != anchor.Location.File {
				score += 10
			}
			if prior, ok := byID[n.ID]; ok && prior.score >= score {
				continue
			}
			byID[n.ID] = candidate{id: n.ID, score: score, link: api.QueryLink{
				From: anchor.QualifiedName, Rel: item.edge.Type, To: n.QualifiedName,
				File: n.Location.File, Start: n.Location.StartLine, End: n.Location.EndLine,
			}}
		}
	}
	all := make([]candidate, 0, len(byID))
	for _, c := range byID {
		all = append(all, c)
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].score != all[j].score {
			return all[i].score > all[j].score
		}
		return all[i].link.To < all[j].link.To
	})
	var out []api.QueryLink
	used := map[int64]bool{}
	files := map[string]int{}
	for pass := 1; pass <= 2 && len(out) < queryLinkedLimit; pass++ {
		for _, c := range all {
			if len(out) >= queryLinkedLimit {
				break
			}
			if used[c.id] || files[c.link.File] >= pass {
				continue
			}
			out = append(out, c.link)
			used[c.id] = true
			files[c.link.File]++
		}
	}
	return out, nil
}
