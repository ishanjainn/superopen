package engine

import (
	"sort"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// OccurrenceFact is the in-RAM row for identifier reference edges.
// Assemble still emits one USAGE/WRITES/CALL_REFERENCE edge per row; unused
// SyntaxFact fields (body tokens, minhash, argument slices) are not stored.
type OccurrenceFact struct {
	Name               string
	Scope              string
	StartByte          uint32
	EndByte            uint32
	StartLine          int32
	StartColumn        int32
	EndLine            int32
	EndColumn          int32
	Confidence         float32
	MayBeCallReference bool
	SourceOrigin       string
}

func occurrenceFact(name, scope string, node syntaxView, lines []uint32, confidence float64) OccurrenceFact {
	startLine, startColumn := bytePosition(lines, node.StartByte())
	endLine, endColumn := bytePosition(lines, node.EndByte())
	return OccurrenceFact{
		Name:        internString(name),
		Scope:       internString(scope),
		StartByte:   node.StartByte(),
		EndByte:     node.EndByte(),
		StartLine:   int32(startLine),
		StartColumn: int32(startColumn),
		EndLine:     int32(endLine),
		EndColumn:   int32(endColumn),
		Confidence:  float32(confidence),
	}
}

func sortOccurrenceFacts(values []OccurrenceFact) {
	sort.SliceStable(values, func(i, j int) bool {
		if values[i].StartByte != values[j].StartByte {
			return values[i].StartByte < values[j].StartByte
		}
		return values[i].Name < values[j].Name
	})
}

func occurrenceLocation(file string, fact OccurrenceFact) api.Location {
	return api.Location{
		File: file, StartLine: int(fact.StartLine), StartColumn: int(fact.StartColumn),
		EndLine: int(fact.EndLine), EndColumn: int(fact.EndColumn),
	}
}

func occurrenceEvidence(file string, fact OccurrenceFact, strategy string, confidence ...float64) *api.Evidence {
	value := float64(fact.Confidence)
	if len(confidence) > 0 {
		value = confidence[0]
	}
	return &api.Evidence{Strategy: internString(strategy), Confidence: value, Location: locationPointer(occurrenceLocation(file, fact))}
}

func occurrenceLocallyBound(file string, fact OccurrenceFact, bindings map[string]map[string]bool) bool {
	if fact.Scope == "" || strings.ContainsAny(fact.Name, ".:") {
		return false
	}
	scope := fact.Scope
	for {
		if bindings[file+"\x00"+scope][fact.Name] {
			return true
		}
		index := strings.LastIndexByte(scope, '.')
		if index < 0 {
			return false
		}
		scope = scope[:index]
	}
}

func namesOfOccurrences(facts []OccurrenceFact) []string {
	out := make([]string, 0, len(facts))
	for _, fact := range facts {
		out = append(out, "usage\x00"+fact.Name+"\x00")
	}
	return out
}
