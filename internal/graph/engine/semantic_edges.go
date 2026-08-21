package engine

import (
	"path/filepath"
	"sort"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func emitSemanticEdges(builder *Builder, graph goGraph, ids map[string]int64, project string) error {
	defer func() { semanticFeatureCache = nil }()
	type scored struct {
		indexA, indexB     int
		sourceID, targetID int64
		score              float64
		sameFile           bool
	}
	nodes := make(map[string]api.Node, len(graph.nodes))
	for _, node := range graph.nodes {
		nodes[node.QualifiedName] = node
	}
	entries := make([]lshEntry, 0, len(ids))
	for qn, id := range ids {
		node := nodes[qn]
		if !semanticNodeLabel(node.Label) || semanticPropertyBool(node.Properties, "external") {
			continue
		}
		fingerprint, ok := semanticFingerprint(qn, node)
		if !ok {
			continue
		}
		entries = append(entries, lshEntry{
			NodeID:        id,
			Fingerprint:   fingerprint,
			FilePath:      node.Location.File,
			FileExtension: filepath.Ext(node.Location.File),
			QualifiedName: qn,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].QualifiedName != entries[j].QualifiedName {
			return entries[i].QualifiedName < entries[j].QualifiedName
		}
		return entries[i].NodeID < entries[j].NodeID
	})
	keyIndex := map[string]int{}
	for i, entry := range entries {
		keyIndex[entry.QualifiedName] = i
	}
	if len(entries) < 2 {
		return nil
	}
	index := newLSHIndex()
	for _, entry := range entries {
		index.Insert(entry)
	}
	candidates := make([]scored, 0)
	seen := map[[2]int64]bool{}
	for _, source := range entries {
		left := nodes[source.QualifiedName]
		for _, candidate := range index.Candidates(source.Fingerprint, 256) {
			if candidate.NodeID == source.NodeID || source.FileExtension != candidate.FileExtension {
				continue
			}
			if source.QualifiedName >= candidate.QualifiedName {
				continue
			}
			pair := [2]int64{source.NodeID, candidate.NodeID}
			if seen[pair] {
				continue
			}
			seen[pair] = true
			right := nodes[candidate.QualifiedName]
			score := semanticPairScore(source.QualifiedName, candidate.QualifiedName, left, right)
			if score < float64(defaultSemanticScoreConfig.Threshold) {
				continue
			}
			candidates = append(candidates, scored{
				indexA: keyIndex[source.QualifiedName], indexB: keyIndex[candidate.QualifiedName],
				sourceID: source.NodeID, targetID: candidate.NodeID,
				score: score, sameFile: source.FilePath != "" && source.FilePath == candidate.FilePath,
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

func semanticFingerprint(qn string, node api.Node) (minHashFingerprint, bool) {
	if features, ok := semanticFeatureCache[qn]; ok && features.HasMinHash {
		return features.MinHash, true
	}
	raw := semanticPropertyString(node.Properties, "fp")
	if raw == "" {
		return minHashFingerprint{}, false
	}
	fingerprint, err := parseMinHashHex(raw)
	if err != nil {
		return minHashFingerprint{}, false
	}
	return fingerprint, true
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
