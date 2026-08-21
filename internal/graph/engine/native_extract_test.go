//go:build tsnative && cgo

package engine

import (
	"context"
	"reflect"
	"testing"
)

func TestNativeFileResultMatchesWASM(t *testing.T) {
	runtime := testSyntaxGrammarRuntime(t)
	native := nativeSyntaxParser()
	if native == nil {
		t.Fatal("native parser was not linked")
	}
	cases := []struct {
		language, grammar, path, source string
	}{
		{"go", "go", "main.go", "package fixture\n\nfunc Hello() {\n\tfmt.Println(os.Getenv(\"HOME\"))\n}\n"},
		{"javascript", "javascript", "app.js", "import { run } from './run.js'\nexport function start() { run() }\n"},
		{"python", "python", "app.py", "import os\n\ndef load():\n    return os.getenv('HOME')\n"},
		{"typescript", "typescript", "app.ts", "export function start(name: string): void { console.log(name) }\n"},
		{"yaml", "yaml", "app.yaml", "service:\n  url: https://example.com/api\n"},
		{"rst", "rst", "doc.rst", "Title\n=====\n\nSee :func:`hello`.\n"},
		{"makefile", "makefile", "Makefile", "all:\n\techo hello\n"},
		{"kconfig", "kconfig", "Kconfig", "config FOO\n\tbool \"Foo\"\n"},
		{"assembly", "assembly", "boot.S", "start:\n\tmov r0, #1\n"},
		{"devicetree", "devicetree", "board.dts", "/ {\n\tcompatible = \"test,board\";\n};\n"},
		{"c", "c", "log.c", "void report(void) { printk(\"x\"); }\n"},
	}
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.language, func(t *testing.T) {
			wasmTree, err := runtime.Parse(ctx, tc.grammar, []byte(tc.source))
			if err != nil {
				t.Fatal(err)
			}
			wasm, err := ExtractSyntaxFacts(tc.language, wasmTree, []byte(tc.source))
			if err != nil {
				t.Fatal(err)
			}
			nativeResult, err := native.(factExtractor).ExtractFacts(ctx, tc.language, tc.grammar, []byte(tc.source))
			if err != nil {
				t.Fatal(err)
			}
			assertFileResultNames(t, "definitions", namesOf(wasm.Definitions), namesOf(nativeResult.Definitions))
			assertFileResultNames(t, "calls", namesOf(wasm.Calls), namesOf(nativeResult.Calls))
			assertFileResultNames(t, "imports", namesOf(wasm.Imports), namesOf(nativeResult.Imports))
			assertFileResultNames(t, "usages", namesOfOccurrences(wasm.Usages), namesOfOccurrences(nativeResult.Usages))
		})
	}
}

func namesOf(facts []SyntaxFact) []string {
	out := make([]string, 0, len(facts))
	for _, fact := range facts {
		out = append(out, fact.Kind+"\x00"+fact.Name+"\x00"+fact.LocalName)
	}
	return out
}

func assertFileResultNames(t *testing.T, kind string, wasm, native []string) {
	t.Helper()
	if !reflect.DeepEqual(wasm, native) {
		t.Fatalf("%s mismatch\nwasm=%v\nnative=%v", kind, wasm, native)
	}
}
