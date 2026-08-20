package engine

import "github.com/ishanjainn/superopen/internal/graph/leiden"

// LeidenEdge is an unweighted graph edge. Leiden treats edges as undirected.
type LeidenEdge = leiden.Edge

type LeidenMembership = leiden.Membership

// Leiden reproduces the pinned engine's deterministic Leiden-style community
// detection. A non-positive resolution uses the Superopen default of 1.0.
func Leiden(nodes []int64, edges []LeidenEdge, resolution float64) []LeidenMembership {
	return leiden.Detect(nodes, edges, resolution)
}
