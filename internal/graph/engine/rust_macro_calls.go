package engine

import (
	"context"
	"strings"
)

// Reparse expression-macro arguments because the Rust grammar leaves
// their calls inside token_tree nodes. Keep the list to macros whose arguments
// are evaluated as ordinary expressions.
func rustExpressionMacro(name string) bool {
	switch name {
	case "println", "eprintln", "print", "eprint", "format", "write", "writeln",
		"panic", "assert", "assert_eq", "assert_ne", "debug_assert",
		"debug_assert_eq", "debug_assert_ne", "dbg":
		return true
	}
	return false
}

func enrichRustMacroCalls(ctx context.Context, parser SyntaxParser, body []byte, extraction FileResult) FileResult {
	if len(extraction.MacroArguments) == 0 {
		return extraction
	}
	const prefix = "fn __so_macro_args() { let _ = ("
	lines := sourceLineIndex(body)
	for _, macro := range extraction.MacroArguments {
		if macro.EndByte > uint32(len(body)) || macro.StartByte >= macro.EndByte || macro.EndByte-macro.StartByte > 16*1024 {
			continue
		}
		inner := body[macro.StartByte:macro.EndByte]
		wrapped := []byte(prefix + string(inner) + "); }\n")
		tree, err := parser.Parse(ctx, "rust", wrapped)
		if err != nil || tree.HasErr() {
			continue
		}
		facts, err := ExtractSyntaxFacts("rust", tree, wrapped)
		if err != nil || facts.Partial {
			continue
		}
		for _, call := range facts.Calls {
			if call.StartByte < uint32(len(prefix)) || call.EndByte > uint32(len(prefix))+uint32(len(inner)) {
				continue
			}
			call.StartByte = macro.StartByte + call.StartByte - uint32(len(prefix))
			call.EndByte = macro.StartByte + call.EndByte - uint32(len(prefix))
			call.StartLine, call.StartColumn = bytePosition(lines, call.StartByte)
			call.EndLine, call.EndColumn = bytePosition(lines, call.EndByte)
			call.Scope = strings.TrimSpace(macro.Scope)
			call.SourceOrigin = "rust_macro_argument"
			extraction.Calls = append(extraction.Calls, call)
		}
	}
	return extraction
}
