//go:build tsnative && cgo

// Package rstlang links the vendored tree-sitter-rst grammar (stsewd/tree-sitter-rst v0.2.0).
// The upstream Go binding omits -I for <tree_sitter/parser.h>, so Superopen compiles
// the C sources with the include path the scanner needs.
package rstlang

/*
#cgo CFLAGS: -std=c11 -fPIC -I${SRCDIR}/src
#include "src/parser.c"
#include "src/scanner.c"
*/
import "C"

import "unsafe"

func Language() unsafe.Pointer {
	return unsafe.Pointer(C.tree_sitter_rst())
}
