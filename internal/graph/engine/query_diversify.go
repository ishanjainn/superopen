package engine

import "github.com/ishanjainn/superopen/internal/graph/api"

const queryDiverseLockedPrefix = 4

// diversifyQueryRows keeps the top evidence together, then gives other files
// one slot each before repeating a file. This keeps large classes or modules
// from filling a short answer with sibling methods from one source file.
func diversifyQueryRows(rows []api.Node) []api.Node {
	if len(rows) <= queryDiverseLockedPrefix {
		return rows
	}
	locked := queryDiverseLockedPrefix
	if len(rows) < locked {
		locked = len(rows)
	}
	out := append([]api.Node(nil), rows[:locked]...)
	used := map[int64]bool{}
	files := map[string]bool{}
	tests := 0
	for _, n := range out {
		used[n.ID] = true
		files[n.Location.File] = true
		if queryNodeLooksLikeTest(n) {
			tests++
		}
	}
	for pass := 0; pass < 2; pass++ {
		for _, n := range rows[locked:] {
			if len(out) >= queryDefaultLimit {
				break
			}
			auxiliary := queryNodeLooksLikeAuxiliary(n)
			if pass == 0 && auxiliary || pass == 1 && !auxiliary {
				continue
			}
			isTest := queryNodeLooksLikeTest(n)
			if files[n.Location.File] || isTest && tests >= 2 {
				continue
			}
			out = append(out, n)
			used[n.ID] = true
			files[n.Location.File] = true
			if isTest {
				tests++
			}
		}
	}
	for _, n := range rows[locked:] {
		if !used[n.ID] {
			out = append(out, n)
		}
	}
	return out
}
