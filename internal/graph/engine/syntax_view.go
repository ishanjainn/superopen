package engine

// syntaxView is a Tree-sitter node that extract can walk without copying the
// full AST into []SyntaxNode. Native extract uses live C nodes; WASM extract
// fetches children on demand; tests keep using SyntaxNode.
type syntaxView interface {
	Kind() string
	FieldName() string
	IsNamed() bool
	HasErr() bool
	StartByte() uint32
	EndByte() uint32
	ChildCount() int
	ChildAt(i int) syntaxView
}

type fieldLookupView interface {
	ChildByField(field string) (syntaxView, bool)
}

type childWalker interface {
	EachChild(fn func(syntaxView))
}

func (n SyntaxNode) Kind() string      { return n.Type }
func (n SyntaxNode) FieldName() string { return n.Field }
func (n SyntaxNode) IsNamed() bool     { return n.Named }
func (n SyntaxNode) HasErr() bool      { return n.HasError }
func (n SyntaxNode) StartByte() uint32 { return n.Start }
func (n SyntaxNode) EndByte() uint32   { return n.End }
func (n SyntaxNode) ChildCount() int   { return len(n.Children) }

func (n SyntaxNode) ChildAt(i int) syntaxView {
	if i < 0 || i >= len(n.Children) {
		return SyntaxNode{}
	}
	return n.Children[i]
}

func (n SyntaxNode) EachChild(fn func(syntaxView)) {
	for i := range n.Children {
		fn(n.Children[i])
	}
}

func viewMissing(node syntaxView) bool {
	return node == nil || node.Kind() == ""
}

func viewEachChild(node syntaxView, fn func(syntaxView)) {
	if node == nil {
		return
	}
	if walker, ok := node.(childWalker); ok {
		walker.EachChild(fn)
		return
	}
	count := node.ChildCount()
	for i := 0; i < count; i++ {
		fn(node.ChildAt(i))
	}
}

func viewPushChildrenReversed(node syntaxView, stack []syntaxView, limit int) []syntaxView {
	if node == nil {
		return stack
	}
	var kids []syntaxView
	viewEachChild(node, func(child syntaxView) {
		kids = append(kids, child)
	})
	for i := len(kids) - 1; i >= 0; i-- {
		if limit > 0 && len(stack) >= limit {
			break
		}
		stack = append(stack, kids[i])
	}
	return stack
}
