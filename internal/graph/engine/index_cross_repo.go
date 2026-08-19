package engine

import "github.com/ishanjainn/superopen/internal/graph/api"

// annotateCrossRepositoryEdges reserves the pinned cross-repo relationship
// families on the development graph. Matching against external project graphs
// remains gated until projects.cross-repository verifies.
func annotateCrossRepositoryEdges(graph *goGraph) {
	for _, edge := range graph.edges {
		if edge.kind != "HTTP_CALLS" && edge.kind != "ASYNC_CALLS" && edge.kind != "GRPC_CALLS" {
			continue
		}
		if edge.properties == nil {
			edge.properties = api.Properties{}
		}
		edge.properties["cross_repo_candidate"] = true
	}
}
