package engine

import "testing"

func TestASTProfilePinnedCountersAndVector(t *testing.T) {
	t.Parallel()
	source := []byte("if x {\n return 1\n}\n")
	tree := SyntaxNode{Type: "block", Named: true, End: uint32(len(source)), Children: []SyntaxNode{
		{Type: "if_statement", Named: true, End: uint32(len(source)), Children: []SyntaxNode{
			{Type: "identifier", Named: true, Start: 3, End: 4},
			{Type: "return_statement", Named: true, Start: 8, End: 16, Children: []SyntaxNode{
				{Type: "integer", Named: true, Start: 15, End: 16},
			}},
		}},
	}}
	profile, ok := ComputeASTProfile(tree, source, []string{"x"})
	if !ok || profile[profileIf] != 1 || profile[profileReturn] != 1 || profile[profileNumber] != 1 || profile[profileParameters] != 1 || profile[profileBodyLines] != 4 {
		t.Fatalf("profile = %#v, %v", profile, ok)
	}
	vector := profile.Vector()
	if vector[profileIf] != .01 || vector[profileParameters] != .05 {
		t.Fatalf("profile vector = %#v", vector)
	}
	if profile.String() == "" {
		t.Fatal("serialized profile is empty")
	}
}

func TestASTProfileSkipsAnonymousLeaves(t *testing.T) {
	t.Parallel()
	profile, ok := ComputeASTProfile(SyntaxNode{Type: ";", Named: false}, nil, nil)
	if ok || profile != (ASTProfile{}) {
		t.Fatalf("anonymous-only profile = %#v, %v", profile, ok)
	}
}
