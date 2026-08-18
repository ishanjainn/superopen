package engine

import "testing"

func TestPinnedLanguageKeywordExceptions(t *testing.T) {
	t.Parallel()
	if !isLanguageKeyword("go", "println") || !isLanguageKeyword("typescript", "Promise") || !isLanguageKeyword("scala", "Integer") {
		t.Fatal("language family keyword alias is missing")
	}
	if isLanguageKeyword("kotlin", "double") {
		t.Fatal("Kotlin primitive-like identifier must remain legal")
	}
	if isLanguageKeyword("puppet", "include") || isLanguageKeyword("puppet", "require") {
		t.Fatal("Puppet built-in calls must not be suppressed")
	}
	if !isResolvableBuiltin("python", "len") || isResolvableBuiltin("python", "enumerate") {
		t.Fatal("Python resolvable builtin inventory drifted")
	}
}
