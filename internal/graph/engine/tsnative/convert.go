//go:build tsnative && cgo

package tsnative

import tree_sitter "github.com/tree-sitter/go-tree-sitter"

// Node is the in-process Tree-sitter tree used to fill FileResult. It is not
// the WASM SyntaxNode marshal path; production discards it after extract.
type Node struct {
	Type     string
	Field    string
	Named    bool
	Start    uint32
	End      uint32
	HasError bool
	Children []Node
}

func Convert(root *tree_sitter.Node) Node {
	if root == nil {
		return Node{}
	}
	cursor := root.Walk()
	defer cursor.Close()
	return convertCursor(cursor)
}

func convertCursor(cursor *tree_sitter.TreeCursor) Node {
	n := cursor.Node()
	out := Node{
		Type:     n.Kind(),
		Field:    cursor.FieldName(),
		Named:    n.IsNamed(),
		Start:    uint32(n.StartByte()),
		End:      uint32(n.EndByte()),
		HasError: n.HasError(),
	}
	if !cursor.GotoFirstChild() {
		return out
	}
	for {
		out.Children = append(out.Children, convertCursor(cursor))
		if !cursor.GotoNextSibling() {
			break
		}
	}
	cursor.GotoParent()
	return out
}
