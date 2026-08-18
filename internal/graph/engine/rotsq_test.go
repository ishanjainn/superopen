package engine

import (
	"math"
	"testing"
)

func TestRotSQEstimatorPinnedErrorBounds(t *testing.T) {
	const count = 64
	vectors := make([][rotSQInputDimensions]float32, count)
	codes := make([]rotSQCode, count)
	state := uint32(0xC0FFEE)
	for index := range vectors {
		norm := float64(0)
		for dimension := range vectors[index] {
			state ^= state << 13
			state ^= state >> 17
			state ^= state << 5
			vectors[index][dimension] = float32(state&0xFFFFFF)/float32(0x7FFFFF) - 1
			norm += float64(vectors[index][dimension]) * float64(vectors[index][dimension])
		}
		inverse := float32(0)
		if norm > 0 {
			inverse = float32(1 / math.Sqrt(norm))
		}
		for dimension := range vectors[index] {
			vectors[index][dimension] *= inverse
		}
		codes[index] = encodeRotSQ(vectors[index][:])
	}
	maxError, sumError, pairs := float64(0), float64(0), 0
	for left := range vectors {
		for right := left; right < len(vectors); right++ {
			exact := float64(0)
			for dimension := range vectors[left] {
				exact += float64(vectors[left][dimension]) * float64(vectors[right][dimension])
			}
			err := math.Abs(float64(rotSQInnerProduct(codes[left], codes[right])) - exact)
			sumError += err
			if err > maxError {
				maxError = err
			}
			pairs++
		}
	}
	if self := float64(rotSQInnerProduct(codes[0], codes[0])); math.Abs(self-1) >= 0.05 {
		t.Fatalf("self inner product = %f", self)
	}
	if mean := sumError / float64(pairs); mean >= 0.01 || maxError >= 0.04 {
		t.Fatalf("estimator error mean=%f max=%f", mean, maxError)
	}
}

func TestRotSQDeterministicAndDecodable(t *testing.T) {
	vector := make([]float32, rotSQInputDimensions)
	for index := range vector {
		vector[index] = float32(index) / rotSQInputDimensions
	}
	first, second := encodeRotSQ(vector), encodeRotSQ(vector)
	if first != second || first.Scale <= 0 {
		t.Fatal("encoding is not deterministic")
	}
	decoded := decodeRotSQ(first)
	if decoded[0] < first.Offset || decoded[0] > first.Offset+first.Scale*rotSQLevels {
		t.Fatal("decoded coordinate outside quantization range")
	}
}
