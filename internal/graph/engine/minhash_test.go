package engine

import (
	"fmt"
	"testing"
)

func TestMinHashNormalizationAndBounds(t *testing.T) {
	if normalizeSyntaxType("type_identifier") != "I" || normalizeSyntaxType("primitive_type") != "T" ||
		normalizeSyntaxType("string_literal") != "S" || normalizeSyntaxType("integer") != "N" {
		t.Fatal("syntax normalization drifted")
	}
	short := SyntaxNode{Type: "body", Children: []SyntaxNode{{Type: "identifier"}}}
	if _, ok := syntaxMinHash(short); ok {
		t.Fatal("short syntax tree produced a fingerprint")
	}
}

func TestMinHashStableRenamesAndRoundTrip(t *testing.T) {
	makeTree := func(identifier string) SyntaxNode {
		children := make([]SyntaxNode, 0, 80)
		for index := 0; index < 40; index++ {
			children = append(children,
				SyntaxNode{Type: fmt.Sprintf("branch_%d", index)},
				SyntaxNode{Type: identifier},
				SyntaxNode{Type: "return"},
			)
		}
		return SyntaxNode{Type: "body", Children: children}
	}
	first, ok := syntaxMinHash(makeTree("identifier"))
	if !ok {
		t.Fatal("representative tree did not produce fingerprint")
	}
	renamed, ok := syntaxMinHash(makeTree("field_identifier"))
	if !ok || first != renamed || minHashJaccard(first, renamed) != 1 {
		t.Fatal("identifier renaming changed structural fingerprint")
	}
	encoded := minHashHex(first)
	decoded, err := parseMinHashHex(encoded)
	if err != nil || decoded != first || len(encoded) != 512 {
		t.Fatalf("roundtrip failed: len=%d err=%v", len(encoded), err)
	}
	if _, err := parseMinHashHex(encoded[:511] + "z"); err == nil {
		t.Fatal("invalid fingerprint accepted")
	}
}

func TestMinHashDifferentStructureDiffers(t *testing.T) {
	childrenA := make([]SyntaxNode, 0, 120)
	childrenB := make([]SyntaxNode, 0, 120)
	for index := 0; index < 40; index++ {
		childrenA = append(childrenA, SyntaxNode{Type: fmt.Sprintf("branch_%d", index)}, SyntaxNode{Type: "identifier"}, SyntaxNode{Type: "return"})
		childrenB = append(childrenB, SyntaxNode{Type: fmt.Sprintf("loop_%d", index)}, SyntaxNode{Type: "identifier"}, SyntaxNode{Type: "call"})
	}
	first, okA := syntaxMinHash(SyntaxNode{Type: "body", Children: childrenA})
	second, okB := syntaxMinHash(SyntaxNode{Type: "body", Children: childrenB})
	if !okA || !okB || minHashJaccard(first, second) >= 0.95 {
		t.Fatalf("different structures were near clones: ok=%t/%t score=%f", okA, okB, minHashJaccard(first, second))
	}
}

func TestLSHCandidatesDeduplicateAndBound(t *testing.T) {
	var fingerprint minHashFingerprint
	for index := range fingerprint {
		fingerprint[index] = uint32(index*17 + 3)
	}
	index := newLSHIndex()
	index.Insert(lshEntry{NodeID: 1, Fingerprint: fingerprint, QualifiedName: "one"})
	index.Insert(lshEntry{NodeID: 2, Fingerprint: fingerprint, QualifiedName: "two"})
	candidates := index.Candidates(fingerprint, 10)
	if len(candidates) != 2 || candidates[0].NodeID != 1 || candidates[1].NodeID != 2 {
		t.Fatalf("candidates = %#v", candidates)
	}
	if bounded := index.Candidates(fingerprint, 1); len(bounded) != 1 {
		t.Fatalf("bounded candidates = %#v", bounded)
	}
}

func TestLSHSkipsNoisyBuckets(t *testing.T) {
	index := newLSHIndex()
	var fingerprint minHashFingerprint
	for nodeID := int64(1); nodeID <= 201; nodeID++ {
		index.Insert(lshEntry{NodeID: nodeID, Fingerprint: fingerprint})
	}
	if candidates := index.Candidates(fingerprint, 300); len(candidates) != 0 {
		t.Fatalf("noisy bucket returned %d candidates", len(candidates))
	}
}
