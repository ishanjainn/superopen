package engine

import (
	"path/filepath"
	"sort"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func emitSemanticEdges(builder *Builder, graph goGraph, ids map[string]int64, project string) error {
	type scored struct {
		indexA, indexB     int
		sourceID, targetID int64
		score              float64
		sameFile           bool
	}
	candidates := make([]scored, 0)
	nodes := make(map[string]api.Node, len(graph.nodes))
	for _, node := range graph.nodes {
		nodes[node.QualifiedName] = node
	}
	keys := make([]string, 0, len(ids))
	for qn := range ids {
		keys = append(keys, qn)
	}
	sort.Strings(keys)
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			left, right := nodes[keys[i]], nodes[keys[j]]
			if !semanticNodeLabel(left.Label) || !semanticNodeLabel(right.Label) {
				continue
			}
			if semanticPropertyBool(left.Properties, "external") || semanticPropertyBool(right.Properties, "external") {
				continue
			}
			// Superopen only pairs functions sharing a file extension.
			if filepath.Ext(left.Location.File) != filepath.Ext(right.Location.File) {
				continue
			}
			score := semanticPairScore(keys[i], keys[j], left, right)
			if score < float64(defaultSemanticScoreConfig.Threshold) {
				continue
			}
			candidates = append(candidates, scored{
				indexA: i, indexB: j,
				sourceID: ids[keys[i]], targetID: ids[keys[j]],
				score: score, sameFile: left.Location.File != "" && left.Location.File == right.Location.File,
			})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].indexA != candidates[j].indexA {
			return candidates[i].indexA < candidates[j].indexA
		}
		return candidates[i].indexB < candidates[j].indexB
	})
	budget := map[int]int{}
	maxEdges := defaultSemanticScoreConfig.MaxEdges
	for _, candidate := range candidates {
		if budget[candidate.indexA] >= maxEdges || budget[candidate.indexB] >= maxEdges {
			continue
		}
		properties := api.Properties{"score": roundThree(candidate.score), "same_file": candidate.sameFile}
		if _, err := builder.PutEdge(api.Edge{
			Project: project, SourceID: candidate.sourceID, TargetID: candidate.targetID,
			Type: "SEMANTICALLY_RELATED", Properties: properties,
		}); err != nil {
			return err
		}
		budget[candidate.indexA]++
		budget[candidate.indexB]++
	}
	return nil
}

// semanticPairScore uses the corpus-derived feature vectors when the semantic
// publish step produced them, in Superopen scoring.
func semanticPairScore(leftQN, rightQN string, left, right api.Node) float64 {
	leftFeatures, okLeft := semanticFeatureCache[leftQN]
	rightFeatures, okRight := semanticFeatureCache[rightQN]
	if okLeft && okRight {
		return float64(combinedSemanticScore(leftFeatures, rightFeatures, defaultSemanticScoreConfig))
	}
	leftFP := semanticPropertyString(left.Properties, "fp")
	rightFP := semanticPropertyString(right.Properties, "fp")
	if leftFP != "" && rightFP != "" {
		leftHash, errLeft := parseMinHashHex(leftFP)
		rightHash, errRight := parseMinHashHex(rightFP)
		if errLeft == nil && errRight == nil {
			return minHashJaccard(leftHash, rightHash)
		}
	}
	return 0
}
