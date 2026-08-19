package engine

import "testing"

func TestCombinedSemanticScorePinnedWeightsAndClamp(t *testing.T) {
	t.Parallel()
	left := semanticFeatures{FilePath: "src/a.go", TFIDF: map[int]float32{1: 1}}
	right := left
	score := combinedSemanticScore(left, right, semanticScoreConfig{TFIDF: 1})
	if score != 1 {
		t.Fatalf("combined identical score = %f", score)
	}
	config := defaultSemanticScoreConfig
	if config.TFIDF != .20 || config.RI != .25 || config.MinHash != .10 || config.API != .15 || config.Type != .10 || config.Decorator != .05 || config.Structural != .10 || config.Threshold != .75 || config.MaxEdges != 10 {
		t.Fatalf("pinned semantic weights drifted: %#v", config)
	}
}

func TestCombinedSemanticScoreSuppressesNearClones(t *testing.T) {
	t.Parallel()
	var fingerprint minHashFingerprint
	for index := range fingerprint {
		fingerprint[index] = uint32(index + 1)
	}
	features := semanticFeatures{HasMinHash: true, MinHash: fingerprint, TFIDF: map[int]float32{1: 1}}
	if score := combinedSemanticScore(features, features, defaultSemanticScoreConfig); score != 0 {
		t.Fatalf("near-clone score = %f", score)
	}
}

func TestSparseCosine(t *testing.T) {
	t.Parallel()
	if got := sparseCosine(map[int]float32{1: 1}, map[int]float32{1: 1}); got != 1 {
		t.Fatalf("identical sparse cosine = %f", got)
	}
	if got := sparseCosine(map[int]float32{1: 1}, map[int]float32{2: 1}); got != 0 {
		t.Fatalf("orthogonal sparse cosine = %f", got)
	}
}
