package engine

import (
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/langspec"
)

func TestPinnedLanguageSpecsCoverEveryGrammar(t *testing.T) {
	t.Parallel()
	all := langspec.All()
	for _, language := range Languages {
		if _, ok := PinnedLanguageSpec(language); !ok {
			t.Errorf("missing pinned extraction spec for %s", language)
		}
	}
	for _, virtual := range []string{"k8s", "kustomize"} {
		if _, ok := PinnedLanguageSpec(virtual); !ok {
			t.Errorf("missing virtual extraction spec for %s", virtual)
		}
	}
	if len(all) < len(Languages) {
		t.Fatalf("langspec count %d < Languages %d", len(all), len(Languages))
	}
}

func TestPinnedLanguageSpecIsDefensiveAndExact(t *testing.T) {
	t.Parallel()
	spec, ok := PinnedLanguageSpec("go")
	if !ok || len(spec.Functions) != 4 || spec.Functions[0] != "function_declaration" || spec.Calls[0] != "call_expression" {
		t.Fatalf("unexpected pinned Go spec: %#v, %v", spec, ok)
	}
	spec.Functions[0] = "mutated"
	again, _ := PinnedLanguageSpec("go")
	if again.Functions[0] != "function_declaration" {
		t.Fatal("caller mutated the pinned language table")
	}
}
