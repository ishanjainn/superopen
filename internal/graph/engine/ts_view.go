//go:build tsnative && cgo

package engine

import tree_sitter "github.com/tree-sitter/go-tree-sitter"

// tsView walks a live Tree-sitter node. Children are visited with a TreeCursor
// and are not retained on the parent, so extract RSS stays O(depth) not O(nodes).
type tsView struct {
	node     tree_sitter.Node
	field    string
	kind     string
	kindID   uint16
	start    uint32
	end      uint32
	named    bool
	hasErr   bool
	gotMeta  bool
	gotErr   bool
	gotKindID bool
	nchild   int
	gotCount bool
}

func tsRootView(root *tree_sitter.Node) syntaxView {
	if root == nil {
		return SyntaxNode{}
	}
	return &tsView{node: *root}
}

func (v *tsView) loadMeta() {
	if v == nil || v.gotMeta {
		return
	}
	v.kind = v.node.Kind()
	v.named = v.node.IsNamed()
	v.start = uint32(v.node.StartByte())
	v.end = uint32(v.node.EndByte())
	v.gotMeta = true
}

func (v *tsView) Kind() string {
	v.loadMeta()
	return v.kind
}

func (v *tsView) KindId() uint16 {
	if v == nil {
		return 0
	}
	if !v.gotKindID {
		v.kindID = v.node.KindId()
		v.gotKindID = true
	}
	return v.kindID
}

func (v *tsView) FieldName() string { return v.field }

func (v *tsView) IsNamed() bool {
	v.loadMeta()
	return v.named
}

func (v *tsView) HasErr() bool {
	if v == nil {
		return false
	}
	if !v.gotErr {
		v.hasErr = v.node.HasError()
		v.gotErr = true
	}
	return v.hasErr
}

func (v *tsView) StartByte() uint32 {
	v.loadMeta()
	return v.start
}

func (v *tsView) EndByte() uint32 {
	v.loadMeta()
	return v.end
}

func (v *tsView) ChildCount() int {
	if v == nil {
		return 0
	}
	if !v.gotCount {
		v.nchild = int(v.node.ChildCount())
		v.gotCount = true
	}
	return v.nchild
}

func (v *tsView) ChildAt(i int) syntaxView {
	if v == nil || i < 0 || i >= v.ChildCount() {
		return SyntaxNode{}
	}
	child := v.node.Child(uint(i))
	if child == nil {
		return SyntaxNode{}
	}
	return &tsView{node: *child, field: v.node.FieldNameForChild(uint32(i))}
}

func (v *tsView) EachChild(fn func(syntaxView)) {
	if v == nil {
		return
	}
	cursor := v.node.Walk()
	defer cursor.Close()
	if !cursor.GotoFirstChild() {
		v.nchild = 0
		v.gotCount = true
		return
	}
	count := 0
	for {
		child := cursor.Node()
		if child != nil {
			fn(&tsView{node: *child, field: cursor.FieldName()})
			count++
		}
		if !cursor.GotoNextSibling() {
			break
		}
	}
	v.nchild = count
	v.gotCount = true
}

func (v *tsView) ChildByField(field string) (syntaxView, bool) {
	if v == nil || field == "" {
		return SyntaxNode{}, false
	}
	child := v.node.ChildByFieldName(field)
	if child == nil {
		return SyntaxNode{}, false
	}
	return &tsView{node: *child, field: field}, true
}
