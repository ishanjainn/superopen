package engine

import (
	"context"

	"github.com/tetratelabs/wazero/api"
)

// wasmView walks WASM parser handles on demand. It does not marshal the full
// tree into []SyntaxNode.
type wasmView struct {
	ctx     context.Context
	module  api.Module
	handle  uint32
	field   string
	kind    string
	start   uint32
	end     uint32
	named   bool
	hasErr  bool
	nchild  int
	gotMeta bool
	gotKids bool
}

func (v *wasmView) Kind() string {
	v.loadMeta()
	return v.kind
}

func (v *wasmView) FieldName() string { return v.field }

func (v *wasmView) IsNamed() bool {
	v.loadMeta()
	return v.named
}

func (v *wasmView) HasErr() bool {
	v.loadMeta()
	return v.hasErr
}

func (v *wasmView) StartByte() uint32 {
	v.loadMeta()
	return v.start
}

func (v *wasmView) EndByte() uint32 {
	v.loadMeta()
	return v.end
}

func (v *wasmView) ChildCount() int {
	if v == nil {
		return 0
	}
	if !v.gotKids {
		count, err := callOne(v.ctx, v.module, "so_node_child_count", uint64(v.handle))
		if err != nil || count > 1_000_000 {
			return 0
		}
		v.nchild = int(count)
		v.gotKids = true
	}
	return v.nchild
}

func (v *wasmView) ChildAt(i int) syntaxView {
	if v == nil || i < 0 || i >= v.ChildCount() {
		return SyntaxNode{}
	}
	fieldPointer, err := callOne(v.ctx, v.module, "so_node_child_field_name", uint64(v.handle), uint64(i))
	if err != nil {
		return SyntaxNode{}
	}
	field := ""
	if fieldPointer != 0 {
		var ok bool
		field, ok = readCString(v.module.Memory(), uint32(fieldPointer), 1024)
		if !ok {
			return SyntaxNode{}
		}
	}
	child, err := callOne(v.ctx, v.module, "so_node_child", uint64(v.handle), uint64(i))
	if err != nil || child == 0 {
		return SyntaxNode{}
	}
	return &wasmView{ctx: v.ctx, module: v.module, handle: uint32(child), field: field}
}

func (v *wasmView) EachChild(fn func(syntaxView)) {
	if v == nil {
		return
	}
	count := v.ChildCount()
	for i := 0; i < count; i++ {
		fn(v.ChildAt(i))
	}
}

func (v *wasmView) loadMeta() {
	if v == nil || v.gotMeta {
		return
	}
	typePointer, err := callOne(v.ctx, v.module, "so_node_type", uint64(v.handle))
	if err != nil {
		return
	}
	kind, ok := readCString(v.module.Memory(), uint32(typePointer), 1024)
	if !ok {
		return
	}
	start, err := callOne(v.ctx, v.module, "so_node_start_byte", uint64(v.handle))
	if err != nil {
		return
	}
	end, err := callOne(v.ctx, v.module, "so_node_end_byte", uint64(v.handle))
	if err != nil {
		return
	}
	errorFlag, err := callOne(v.ctx, v.module, "so_node_has_error", uint64(v.handle))
	if err != nil {
		return
	}
	named, err := callOne(v.ctx, v.module, "so_node_is_named", uint64(v.handle))
	if err != nil {
		return
	}
	v.kind = kind
	v.start = uint32(start)
	v.end = uint32(end)
	v.hasErr = errorFlag != 0
	v.named = named != 0
	v.gotMeta = true
}
