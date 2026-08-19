package engine

import (
	"math"
	"testing"
)

func TestSemanticVectorPrimitives(t *testing.T) {
	var left, right semanticVector
	left[0], right[0] = 1, 1
	if semanticCosine(left, right) != 1 {
		t.Fatalf("identical cosine = %f", semanticCosine(left, right))
	}
	right[0], right[1] = 0, 1
	if semanticCosine(left, right) != 0 {
		t.Fatalf("orthogonal cosine = %f", semanticCosine(left, right))
	}
	normalizeSemantic(&left)
	addScaledSemantic(&left, &right, 0.5)
	if left[1] != 0.5 {
		t.Fatalf("scaled add = %f", left[1])
	}
}

func TestSparseSemanticIndexDeterministic(t *testing.T) {
	first, second := sparseSemanticIndex("unlisted-token"), sparseSemanticIndex("unlisted-token")
	other := sparseSemanticIndex("different-token")
	if first != second || semanticCosine(first, second) < 0.999 {
		t.Fatal("fallback index is not deterministic")
	}
	if first == other {
		t.Fatal("different fallback tokens have identical vectors")
	}
}

func TestSemanticProximityAndDiffusion(t *testing.T) {
	if semanticProximity("src/main.go", "src/main.go") != 1.1 ||
		semanticProximity("src/foo/a.go", "tests/bar/b.go") != 1 {
		t.Fatal("proximity drifted")
	}
	var combined, neighbor semanticVector
	combined[0], combined[1], neighbor[0] = 0.5, 0.5, 1
	normalizeSemantic(&combined)
	diffuseSemantic(&combined, []semanticVector{neighbor}, 0.3)
	if magnitude := math.Sqrt(float64(semanticCosine(combined, combined))); math.Abs(magnitude-1) > 0.01 {
		t.Fatalf("diffused vector is not normalized: %f", magnitude)
	}
}

func TestSemanticCorpusIDF(t *testing.T) {
	corpus := newSemanticCorpus()
	corpus.Add(nil)
	corpus.Add([]string{"a", "a", "b"})
	corpus.Add([]string{"a", "c"})
	if corpus.documents != 2 || corpus.IDF("a") >= 0.01 || corpus.IDF("b") <= 0 || corpus.IDF("missing") != 0 {
		t.Fatalf("corpus documents=%d idf(a)=%f idf(b)=%f", corpus.documents, corpus.IDF("a"), corpus.IDF("b"))
	}
}

func TestSemanticCorpusReflectiveEnrichment(t *testing.T) {
	corpus := newSemanticCorpus()
	corpus.Add([]string{"request", "handler", "response"})
	corpus.Add([]string{"request", "controller", "response"})
	corpus.Finalize(nil)
	handler, ok := corpus.Vector("handler")
	if !ok {
		t.Fatal("handler vector missing after finalize")
	}
	controller, ok := corpus.Vector("controller")
	if !ok || semanticCosine(handler, controller) <= 0 {
		t.Fatalf("co-occurrence did not bridge synonyms: cosine=%f", semanticCosine(handler, controller))
	}
	first := handler
	corpus.Finalize(nil)
	again, _ := corpus.Vector("handler")
	if first != again {
		t.Fatal("semantic corpus finalize is not idempotent")
	}
}
