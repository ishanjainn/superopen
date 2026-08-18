package engine

import (
	"encoding/binary"
	"math"
	"strings"

	"github.com/zeebo/xxh3"
)

const semanticDimensions = 768

type semanticVector [semanticDimensions]float32

func semanticCosine(left, right semanticVector) float32 {
	var dot, leftMagnitude, rightMagnitude float32
	for dimension := range left {
		dot += left[dimension] * right[dimension]
		leftMagnitude += left[dimension] * left[dimension]
		rightMagnitude += right[dimension] * right[dimension]
	}
	denominator := float32(math.Sqrt(float64(leftMagnitude))) * float32(math.Sqrt(float64(rightMagnitude)))
	if denominator < 1e-10 {
		return 0
	}
	return dot / denominator
}

func normalizeSemantic(vector *semanticVector) {
	if vector == nil {
		return
	}
	var magnitude float32
	for _, value := range vector {
		magnitude += value * value
	}
	magnitude = float32(math.Sqrt(float64(magnitude)))
	if magnitude < 1e-10 {
		return
	}
	inverse := float32(1) / magnitude
	for dimension := range vector {
		vector[dimension] *= inverse
	}
}

func addScaledSemantic(destination, source *semanticVector, scale float32) {
	if destination == nil || source == nil {
		return
	}
	for dimension := range destination {
		destination[dimension] += scale * source[dimension]
	}
}

// sparseSemanticIndex is the exact fallback for tokens absent from the pinned
// pretrained vocabulary. The pretrained lookup is added by the side-asset loader.
func sparseSemanticIndex(token string) semanticVector {
	var result semanticVector
	if token == "" {
		return result
	}
	seed := xxh3.Hash([]byte(token))
	for index := uint32(0); index < 8; index++ {
		var encoded [4]byte
		binary.LittleEndian.PutUint32(encoded[:], index)
		hash := xxh3.HashSeed(encoded[:], seed+0x52494E44)
		position := int(hash % semanticDimensions)
		sign := float32(-1)
		if hash&1 != 0 {
			sign = 1
		}
		result[position] += sign
	}
	return result
}

func semanticProximity(left, right string) float32 {
	if left == "" || right == "" {
		return 1
	}
	shared := 0
	for index := 0; index < len(left) && index < len(right) && left[index] == right[index]; index++ {
		if left[index] == '/' {
			shared++
		}
	}
	leftComponents := strings.Count(left, "/")
	rightComponents := strings.Count(right, "/")
	total := maxInt(leftComponents, rightComponents)
	if total == 0 {
		return 1
	}
	return 1 + float32(shared)/float32(total)*0.1
}

func diffuseSemantic(combined *semanticVector, neighbors []semanticVector, alpha float32) {
	if combined == nil || len(neighbors) == 0 {
		return
	}
	var mean semanticVector
	for _, neighbor := range neighbors {
		addScaledSemantic(&mean, &neighbor, 1)
	}
	inverse := float32(1) / float32(len(neighbors))
	for dimension := range combined {
		combined[dimension] = (1-alpha)*combined[dimension] + alpha*mean[dimension]*inverse
	}
	normalizeSemantic(combined)
}

type semanticCorpus struct {
	documents int
	frequency map[string]int
	tokens    []string
	index     map[string]int
	docs      [][]int
	enriched  []semanticVector
	finalized bool
}

func newSemanticCorpus() *semanticCorpus {
	return &semanticCorpus{frequency: map[string]int{}, index: map[string]int{}}
}

func (corpus *semanticCorpus) Add(tokens []string) {
	if corpus == nil || len(tokens) == 0 {
		return
	}
	corpus.documents++
	seen := map[string]bool{}
	doc := make([]int, 0, len(tokens))
	for _, token := range tokens {
		if token == "" {
			continue
		}
		id, ok := corpus.index[token]
		if !ok {
			id = len(corpus.tokens)
			corpus.index[token] = id
			corpus.tokens = append(corpus.tokens, token)
		}
		doc = append(doc, id)
		if !seen[token] {
			seen[token] = true
			corpus.frequency[token]++
		}
	}
	corpus.docs = append(corpus.docs, doc)
	corpus.finalized = false
}

type semanticOccurrence struct{ document, position int }

// Finalize performs Superopen's two-pass reflective random-index enrichment.
// It is deterministic: vocabulary IDs follow document insertion order, while
// each token owns its output vector and reads immutable pass data.
func (corpus *semanticCorpus) Finalize(model *pretrainedVectors) {
	if corpus == nil || corpus.finalized {
		return
	}
	occurrences := make([][]semanticOccurrence, len(corpus.tokens))
	for document, ids := range corpus.docs {
		for position, id := range ids {
			occurrences[id] = append(occurrences[id], semanticOccurrence{document: document, position: position})
		}
	}
	base := make([]semanticVector, len(corpus.tokens))
	pass1 := make([]semanticVector, len(corpus.tokens))
	for id, token := range corpus.tokens {
		base[id] = semanticIndex(token, model)
		pass1[id] = base[id]
		accumulateSemanticContext(&pass1[id], occurrences[id], corpus.docs, func(neighbor int) semanticVector { return base[neighbor] })
		normalizeSemantic(&pass1[id])
	}
	quantized := make([][semanticDimensions]int8, len(pass1))
	for id := range pass1 {
		for dimension, value := range pass1[id] {
			scaled := value * 127
			if scaled > 127 {
				scaled = 127
			} else if scaled < -127 {
				scaled = -127
			}
			if scaled >= 0 {
				quantized[id][dimension] = int8(scaled + .5)
			} else {
				quantized[id][dimension] = int8(scaled - .5)
			}
		}
	}
	corpus.enriched = make([]semanticVector, len(corpus.tokens))
	for id := range corpus.tokens {
		var second semanticVector
		accumulateSemanticContext(&second, occurrences[id], corpus.docs, func(neighbor int) semanticVector {
			var decoded semanticVector
			for dimension, value := range quantized[neighbor] {
				decoded[dimension] = float32(value) / 127
			}
			return decoded
		})
		normalizeSemantic(&second)
		for dimension := range second {
			second[dimension] = .7*pass1[id][dimension] + .3*second[dimension]
		}
		normalizeSemantic(&second)
		corpus.enriched[id] = second
	}
	corpus.finalized = true
}

func accumulateSemanticContext(target *semanticVector, occurrences []semanticOccurrence, docs [][]int, vector func(int) semanticVector) {
	step := 1
	if len(occurrences) > 512 {
		step = len(occurrences) / 512
	}
	for occurrence := 0; occurrence < len(occurrences); occurrence += step {
		current := occurrences[occurrence]
		ids := docs[current.document]
		for distance := -5; distance <= 5; distance++ {
			position := current.position + distance
			if distance == 0 || position < 0 || position >= len(ids) {
				continue
			}
			neighbor := vector(ids[position])
			absolute := distance
			if absolute < 0 {
				absolute = -absolute
			}
			addScaledSemantic(target, &neighbor, 1/float32(absolute))
		}
	}
}

func (corpus *semanticCorpus) Vector(token string) (semanticVector, bool) {
	if corpus == nil || !corpus.finalized {
		return semanticVector{}, false
	}
	id, ok := corpus.index[token]
	if !ok || id < 0 || id >= len(corpus.enriched) {
		return semanticVector{}, false
	}
	return corpus.enriched[id], true
}

func (corpus *semanticCorpus) IDF(token string) float32 {
	if corpus == nil || corpus.documents == 0 || corpus.frequency[token] == 0 {
		return 0
	}
	return float32(math.Log(float64(corpus.documents) / float64(corpus.frequency[token])))
}
