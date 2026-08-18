package engine

import "github.com/ishanjainn/superopen/internal/graph/langspec"

// LanguageSpec is the Go representation of Superopen language extraction rules.
// Node kinds are Tree-sitter grammar names, not graph labels.
type LanguageSpec = langspec.Spec

// PinnedLanguageSpec returns a defensive copy of an extraction specification.
func PinnedLanguageSpec(language string) (LanguageSpec, bool) {
	return langspec.Lookup(language)
}
