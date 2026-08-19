package engine

import "math"

const (
	semanticEdgeThreshold = float32(.75)
	nearCloneThreshold    = float32(.95)
	semanticProfileDims   = 25
)

type semanticScoreConfig struct {
	TFIDF, RI, MinHash, API, Type, Decorator, Structural float32
	Threshold                                            float32
	MaxEdges                                             int
}

var defaultSemanticScoreConfig = semanticScoreConfig{
	TFIDF: .20, RI: .25, MinHash: .10, API: .15, Type: .10, Decorator: .05, Structural: .10,
	Threshold: semanticEdgeThreshold, MaxEdges: 10,
}

type semanticFeatures struct {
	FilePath                 string
	TFIDF                    map[int]float32
	RI, API, Type, Decorator rotSQCode
	Profile                  [semanticProfileDims]float32
	MinHash                  minHashFingerprint
	HasMinHash               bool
}

func combinedSemanticScore(left, right semanticFeatures, config semanticScoreConfig) float32 {
	if left.HasMinHash && right.HasMinHash {
		if similarity := float32(minHashJaccard(left.MinHash, right.MinHash)); similarity >= nearCloneThreshold {
			return 0
		}
	}
	score := config.TFIDF*sparseCosine(left.TFIDF, right.TFIDF) +
		config.RI*rotSQInnerProduct(left.RI, right.RI) +
		config.API*rotSQInnerProduct(left.API, right.API) +
		config.Type*rotSQInnerProduct(left.Type, right.Type) +
		config.Decorator*rotSQInnerProduct(left.Decorator, right.Decorator) +
		config.Structural*smallCosine(left.Profile[:], right.Profile[:])
	if left.HasMinHash && right.HasMinHash {
		score += config.MinHash * float32(minHashJaccard(left.MinHash, right.MinHash))
	}
	score *= semanticProximity(left.FilePath, right.FilePath)
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

func sparseCosine(left, right map[int]float32) float32 {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	var dot, leftMagnitude, rightMagnitude float32
	for index, value := range left {
		leftMagnitude += value * value
		dot += value * right[index]
	}
	for _, value := range right {
		rightMagnitude += value * value
	}
	denominator := float32(math.Sqrt(float64(leftMagnitude))) * float32(math.Sqrt(float64(rightMagnitude)))
	if denominator < 1e-10 {
		return 0
	}
	return dot / denominator
}

func smallCosine(left, right []float32) float32 {
	length := len(left)
	if len(right) < length {
		length = len(right)
	}
	var dot, leftMagnitude, rightMagnitude float32
	for index := 0; index < length; index++ {
		dot += left[index] * right[index]
		leftMagnitude += left[index] * left[index]
		rightMagnitude += right[index] * right[index]
	}
	denominator := float32(math.Sqrt(float64(leftMagnitude))) * float32(math.Sqrt(float64(rightMagnitude)))
	if denominator < 1e-10 {
		return 0
	}
	return dot / denominator
}
