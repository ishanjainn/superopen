//go:build cgo

// Package cpreproc expands C/C++/CUDA macros with vendored simplecpp.
// Fail closed: no work or errors return nil so callers keep the raw extract.
package cpreproc

/*
#cgo CXXFLAGS: -std=c++17 -O2 -fPIC -I${SRCDIR}
#cgo linux LDFLAGS: -lstdc++
#include "preprocessor.h"
#include <stdlib.h>
*/
import "C"

import (
	"unsafe"
)

// Result is expanded source plus a 1-based expanded-line map back to the main file.
type Result struct {
	Source        string
	OriginalLine  []uint32
	BelongsToMain []bool
}

// WithMap preprocesses source. filename is the simplecpp main-file identity
// (usually the repository-relative path). cppMode selects c++17 vs c11.
func WithMap(source []byte, filename string, cppMode bool) *Result {
	if len(source) == 0 {
		return nil
	}
	if filename == "" {
		filename = "<input>"
	}
	cSrc := C.CBytes(source)
	defer C.free(cSrc)
	cFile := C.CString(filename)
	defer C.free(unsafe.Pointer(cFile))
	mode := C.int(0)
	if cppMode {
		mode = 1
	}
	pp := C.so_preprocess_with_map((*C.char)(cSrc), C.int(len(source)), cFile, nil, nil, mode)
	if pp == nil {
		return nil
	}
	defer C.so_preprocessed_source_free(pp)
	if pp.source == nil || pp.expanded_line_count <= 0 {
		return nil
	}
	n := int(pp.expanded_line_count) + 1
	out := &Result{
		Source:        C.GoString(pp.source),
		OriginalLine:  make([]uint32, n),
		BelongsToMain: make([]bool, n),
	}
	copy(out.OriginalLine, unsafe.Slice((*uint32)(unsafe.Pointer(pp.original_line_by_expanded_line)), n))
	for i, flag := range unsafe.Slice((*byte)(unsafe.Pointer(pp.belongs_to_main_file)), n) {
		out.BelongsToMain[i] = flag != 0
	}
	return out
}
