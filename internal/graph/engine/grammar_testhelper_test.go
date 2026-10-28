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
			[]string{"go", "javascript", "python", "typescript", "html", "yaml", "rst", "makefile", "kconfig", "assembly", "devicetree", "c", "toml", "xml", "elisp"},
		)
	})
	if testSyntaxRuntimeErr != nil {
		t.Fatal(testSyntaxRuntimeErr)
	}
	return testSyntaxRuntime
}
