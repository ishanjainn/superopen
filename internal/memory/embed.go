package memory

import (
	"encoding/binary"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/zeebo/xxh3"
)

// EmbedderID stamps hashed 384-d int8 vectors used when the loopback
// worker is unavailable. Neural workers use distinct ids so geometries
// are never mixed.
const EmbedderID = "so-prose-384-v1"
const bgeEmbedderID = "so-bge-small-en-v1.5-384"

var activeEmbedderID = EmbedderID

func CurrentEmbedder() string {
	if activeEmbedderID == "" {
		return EmbedderID
	}
	return activeEmbedderID
}

const (
	embedDimensions = 384
	embedHashes     = 8
	maxEmbedRunes   = 1500
)

// Vector is a unit-quantized 384-d int8 embedding.
type Vector [embedDimensions]int8

func (v Vector) Bytes() []byte {
	out := make([]byte, embedDimensions)
	for i, value := range v {
		out[i] = byte(value)
	}
	return out
}

func vectorFromBytes(raw []byte) (Vector, bool) {
	var out Vector
	if len(raw) != embedDimensions {
		return out, false
	}
	for i, value := range raw {
		out[i] = int8(value)
	}
	return out, true
}

// EmbedText encodes prose for memory ingest and recall.
// A healthy BGE worker is preferred. If one is configured but fails,
// a zero vector (embedding_pending) is returned so hash geometry is
// never mixed into a neural store. If no worker is configured at all,
// hashed embeddings are used and EnsureEmbedWorker prints a one-time
// stderr notice — that path is weaker semantic recall, not a silent
// success.
func EmbedText(text string) Vector {
	EnsureEmbedWorker()
	if v, ok := embedViaWorker(text); ok {
		return v
	}
	if workerConfigured() {
		return Vector{}
	}
	return EmbedSentence(text)
}

// EmbedQuery encodes a search query. The BGE worker gets input_type=query
// (instruction prefix). The hash fallback is unprefixed so it stays symmetric
// with stored document vectors on machines without the Python worker.
func EmbedQuery(text string) Vector {
	EnsureEmbedWorker()
	if v, ok := embedQueryViaWorker(text); ok {
		return v
	}
	if workerConfigured() {
		return Vector{}
	}
	return EmbedSentence(text)
}

// EmbedSentence hashes word and character n-grams into a 384-d int8 unit vector.
func EmbedSentence(text string) Vector {
	text = normalizeEmbedText(text)
	if text == "" {
		return Vector{}
	}
	var acc [embedDimensions]float64
	for _, feature := range embedFeatures(text) {
		seed := xxh3.Hash([]byte(feature))
		for index := uint32(0); index < embedHashes; index++ {
			var encoded [4]byte
			binary.LittleEndian.PutUint32(encoded[:], index)
			hash := xxh3.HashSeed(encoded[:], seed+0x50524f53) // "PROS"
			position := int(hash % embedDimensions)
			sign := -1.0
			if hash&1 != 0 {
				sign = 1
			}
			acc[position] += sign
		}
	}
	return quantizeUnit(acc)
}

func Cosine(left Vector, right []byte) float64 {
	if len(right) != embedDimensions {
		return 0
	}
	var dot, leftMag, rightMag float64
	for i, lv := range left {
		rv := float64(int8(right[i]))
		lf := float64(lv)
		dot += lf * rv
		leftMag += lf * lf
		rightMag += rv * rv
	}
	den := math.Sqrt(leftMag) * math.Sqrt(rightMag)
	if den < 1e-10 {
		return 0
	}
	return dot / den
}

func EstimateTokens(s string) int {
	n := utf8.RuneCountInString(s)
	if n <= 0 {
		return 0
	}
	return (n + 3) / 4
}

func normalizeEmbedText(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(strings.ToLower(s))
	if len(runes) > maxEmbedRunes {
		runes = runes[:maxEmbedRunes]
	}
	var b strings.Builder
	b.Grow(len(runes))
	prevSpace := true
	for _, r := range runes {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevSpace = false
			continue
		}
		if !prevSpace {
			b.WriteByte(' ')
			prevSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func embedFeatures(text string) []string {
	words := strings.Fields(text)
	out := make([]string, 0, len(words)*2+16)
	var prev string
	for _, word := range words {
		out = append(out, "w:"+word)
		if prev != "" {
			out = append(out, "b:"+prev+"_"+word)
		}
		prev = word
		if n := utf8.RuneCountInString(word); n >= 3 && n <= 24 {
			runes := []rune(word)
			for size := 3; size <= 5 && size <= len(runes); size++ {
				for i := 0; i+size <= len(runes); i++ {
					out = append(out, "c:"+string(runes[i:i+size]))
				}
			}
		}
	}
	return out
}

func quantizeUnit(acc [embedDimensions]float64) Vector {
	var mag float64
	for _, v := range acc {
		mag += v * v
	}
	mag = math.Sqrt(mag)
	var out Vector
	if mag < 1e-10 {
		return out
	}
	scale := 127.0 / mag
	for i, v := range acc {
		scaled := v * scale
		if scaled > 127 {
			scaled = 127
		} else if scaled < -127 {
			scaled = -127
		}
		if scaled >= 0 {
			out[i] = int8(scaled + 0.5)
		} else {
			out[i] = int8(scaled - 0.5)
		}
	}
	return out
}
