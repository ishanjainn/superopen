//go:build tsnative && cgo

package tsnative

import (
	"context"
	"testing"
)

func TestParseGoFile(t *testing.T) {
	if !Supported("go") {
		t.Fatal("go grammar should be linked")
	}
	session := NewSession()
	defer session.Close()
	node, err := session.Parse(context.Background(), "go", []byte("package fixture\n\nfunc Hello() {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if node.Type != "source_file" || !node.Named {
		t.Fatalf("root=%+v", node)
	}
	found := false
	var walk func(Node)
	walk = func(n Node) {
		if n.Type == "function_declaration" {
			found = true
		}
		for _, child := range n.Children {
			walk(child)
		}
	}
	walk(node)
	if !found {
		t.Fatal("expected function_declaration")
	}
}
