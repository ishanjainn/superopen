package engine

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"strings"

	"github.com/zeebo/xxh3"
)

const (
	minHashSize              = 64
	minHashMinLeaves         = 30
	minHashMinUniqueTrigrams = 32
	minHashWalkStack         = 2048
	minHashMaxTokens         = 4096
	minHashTrigramBytes      = 160
)

type minHashFingerprint [minHashSize]uint32

type lshEntry struct {
	NodeID        int64
	Fingerprint   minHashFingerprint
	FilePath      string
	FileExtension string
	QualifiedName string
}

type lshIndex struct {
	entries []lshEntry
	bands   [32]map[uint16][]int
}

var identifierSyntax = map[string]bool{
	"identifier": true, "field_identifier": true, "property_identifier": true,
	"type_identifier": true, "shorthand_property_identifier": true,
	"shorthand_field_identifier": true, "variable_name": true, "name": true,
}

var stringSyntax = map[string]bool{
	"string": true, "string_literal": true, "interpreted_string_literal": true,
	"raw_string_literal": true, "template_string": true, "string_content": true,
	"escape_sequence": true,
}

var numberSyntax = map[string]bool{
	"number": true, "integer": true, "float": true, "integer_literal": true,
	"float_literal": true, "int_literal": true, "number_literal": true,
}

var typeSyntax = map[string]bool{
	"type_identifier": true, "predefined_type": true, "primitive_type": true,
	"builtin_type": true, "type_annotation": true, "simple_type": true,
}

func syntaxMinHash(root syntaxView) (minHashFingerprint, bool) {
	return minHashFromTokens(collectMinHashTokens(root))
}

func minHashFromTokens(tokens []string) (minHashFingerprint, bool) {
	var fingerprint minHashFingerprint
	for index := range fingerprint {
		fingerprint[index] = math.MaxUint32
	}
	if len(tokens) < minHashMinLeaves {
		return fingerprint, false
	}
	unique := map[uint64]struct{}{}
	for index := 0; index+2 < len(tokens); index++ {
		first, second, third := tokens[index], tokens[index+1], tokens[index+2]
		weight := structuralTokenWeight(first) + structuralTokenWeight(second) + structuralTokenWeight(third)
		if weight == 0 {
			continue
		}
		trigram := first + "|" + second + "|" + third
		if len(trigram) >= minHashTrigramBytes {
			continue
		}
		bytes := []byte(trigram)
		unique[xxh3.Hash(bytes)] = struct{}{}
		for slot := 0; slot < minHashSize; slot++ {
			for repetition := 0; repetition < weight; repetition++ {
				seed := uint64(slot*3 + repetition)
				value := uint32(xxh3.HashSeed(bytes, seed))
				if value < fingerprint[slot] {
					fingerprint[slot] = value
				}
			}
		}
	}
	return fingerprint, len(unique) >= minHashMinUniqueTrigrams
}

func collectMinHashTokens(root syntaxView) []string {
	tokens := make([]string, 0, 128)
	stack := make([]syntaxView, 1, minHashWalkStack)
	stack[0] = root
	for len(stack) > 0 && len(tokens) < minHashMaxTokens {
		last := len(stack) - 1
		node := stack[last]
		stack = stack[:last]
		if node == nil {
			continue
		}
		count := node.ChildCount()
		if count == 0 {
			if node.Kind() != "" {
				tokens = append(tokens, normalizeSyntaxType(node.Kind()))
			}
			continue
		}
		stack = viewPushChildrenReversed(node, stack, minHashWalkStack)
	}
	return tokens
}

func normalizeSyntaxType(kind string) string {
	// Ordering intentionally matches Superopen: type_identifier normalizes to I,
	// because identifier detection precedes type-annotation detection.
	switch {
	case identifierSyntax[kind]:
		return "I"
	case stringSyntax[kind]:
		return "S"
	case numberSyntax[kind]:
		return "N"
	case typeSyntax[kind]:
		return "T"
	default:
		return kind
	}
}

func structuralTokenWeight(token string) int {
	if len(token) == 1 && strings.Contains("ISNT", token) {
		return 0
	}
	return 1
}

func minHashJaccard(left, right minHashFingerprint) float64 {
	matches := 0
	for index := range left {
		if left[index] == right[index] {
			matches++
		}
	}
	return float64(matches) / minHashSize
}

func minHashHex(fingerprint minHashFingerprint) string {
	result := make([]byte, minHashSize*8)
	for index, value := range fingerprint {
		hex.Encode(result[index*8:index*8+8], []byte{
			byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value),
		})
	}
	return string(result)
}

func parseMinHashHex(value string) (minHashFingerprint, error) {
	var result minHashFingerprint
	if len(value) != minHashSize*8 {
		return result, fmt.Errorf("minhash fingerprint must contain %d hex characters", minHashSize*8)
	}
	for index := range result {
		var raw [4]byte
		if _, err := hex.Decode(raw[:], []byte(value[index*8:index*8+8])); err != nil {
			return minHashFingerprint{}, fmt.Errorf("decode minhash slot %d: %w", index, err)
		}
		result[index] = uint32(raw[0])<<24 | uint32(raw[1])<<16 | uint32(raw[2])<<8 | uint32(raw[3])
	}
	return result, nil
}

func newLSHIndex() *lshIndex {
	index := &lshIndex{}
	for band := range index.bands {
		index.bands[band] = map[uint16][]int{}
	}
	return index
}

func (index *lshIndex) Insert(entry lshEntry) {
	if index == nil || entry.NodeID <= 0 {
		return
	}
	entryIndex := len(index.entries)
	index.entries = append(index.entries, entry)
	for band := range index.bands {
		hash := minHashBand(entry.Fingerprint, band)
		index.bands[band][hash] = append(index.bands[band][hash], entryIndex)
	}
}

func (index *lshIndex) Candidates(fingerprint minHashFingerprint, limit int) []lshEntry {
	if index == nil || limit <= 0 {
		return nil
	}
	result := make([]lshEntry, 0, minInt(limit, 64))
	seen := make(map[int64]bool)
	for band := range index.bands {
		bucket := index.bands[band][minHashBand(fingerprint, band)]
		if len(bucket) > 200 {
			continue
		}
		for _, entryIndex := range bucket {
			entry := index.entries[entryIndex]
			if seen[entry.NodeID] {
				continue
			}
			seen[entry.NodeID] = true
			result = append(result, entry)
			if len(result) == limit {
				return result
			}
		}
	}
	return result
}

func minHashBand(fingerprint minHashFingerprint, band int) uint16 {
	var values [8]byte
	binary.LittleEndian.PutUint32(values[:4], fingerprint[band*2])
	binary.LittleEndian.PutUint32(values[4:], fingerprint[band*2+1])
	return uint16(xxh3.Hash(values[:]))
}
