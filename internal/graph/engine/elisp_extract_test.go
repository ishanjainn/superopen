package engine

import (
	"context"
	"testing"
)

func TestNestedElispDefinitionHasName(t *testing.T) {
	source := []byte("(when t\n  (defun flymake-pylint-init ()\n    (list 1)))\n")
	tree, err := testSyntaxGrammarRuntime(t).Parse(context.Background(), "elisp", source)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := ExtractSyntaxFacts("elisp", tree, source)
	if err != nil {
		t.Fatal(err)
	}
	for _, fact := range facts.Definitions {
		if fact.Kind == "function" && fact.Name == "flymake-pylint-init" {
			return
		}
	}
	t.Fatalf("nested function missing: %+v", facts.Definitions)
}
