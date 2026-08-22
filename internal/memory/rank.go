package memory

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	staleDownweight    = 0.5
	supersedeCapEps    = 1e-4
	supersedeCapWindow = 10
	lexFusionW         = 0.35
	cosineWeight       = 0.6
	centralityWeight   = 0.4
	shapeFusionW       = 0.25
	pinWeight          = 0.35
	recallBudgetTok    = 1500
)

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

func trimToBudget(hits []Hit, budget int) []Hit {
	if budget <= 0 {
		return hits
	}
	used := 0
	out := hits[:0]
	for _, h := range hits {
		cost := h.Tokens
		if cost <= 0 {
			cost = EstimateTokens(h.Title + " " + h.Text)
		}
		if used+cost > budget && len(out) > 0 {
			break
		}
		out = append(out, h)
		used += cost
	}
	return out
}

func parseID(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}
