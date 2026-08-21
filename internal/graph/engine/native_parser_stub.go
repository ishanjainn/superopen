//go:build !tsnative || !cgo

package engine

func nativeSyntaxParser() SyntaxParser { return nil }
