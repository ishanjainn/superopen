package engine

import (
	"context"
	"sync"
	"testing"
)

var (
	testSyntaxRuntime     *GrammarRuntime
	testSyntaxRuntimeErr  error
	testSyntaxRuntimeOnce sync.Once
)

func testSyntaxGrammarRuntime(t *testing.T) *GrammarRuntime {
	t.Helper()
	testSyntaxRuntimeOnce.Do(func() {
		testSyntaxRuntime, _, testSyntaxRuntimeErr = loadSelectedGrammarAssets(
			context.Background(),
			EngineAssets,
			"assets/grammars/manifest.json",
			false,
			[]string{"go", "javascript", "python", "typescript", "yaml"},
		)
	})
	if testSyntaxRuntimeErr != nil {
		t.Fatal(testSyntaxRuntimeErr)
	}
	return testSyntaxRuntime
}
