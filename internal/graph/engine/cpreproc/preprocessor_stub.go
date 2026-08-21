//go:build !cgo

package cpreproc

// Result is expanded source plus a 1-based expanded-line map back to the main file.
type Result struct {
	Source        string
	OriginalLine  []uint32
	BelongsToMain []bool
}

// WithMap is a no-op without cgo; callers keep the raw Tree-sitter extract.
func WithMap([]byte, string, bool) *Result { return nil }
