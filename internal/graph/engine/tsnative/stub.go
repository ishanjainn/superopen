//go:build !tsnative || !cgo

package tsnative

// Native Tree-sitter extract is compiled with CGO and -tags tsnative,sqlite_fts5.
const Enabled = false
