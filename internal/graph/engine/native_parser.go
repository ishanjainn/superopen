//go:build tsnative && cgo

package engine

import (
	"context"
	"fmt"

	"github.com/ishanjainn/superopen/internal/graph/engine/tsnative"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func nativeSyntaxParser() SyntaxParser {
	return newNativeParser()
}

type nativeParser struct{}

func newNativeParser() *nativeParser { return &nativeParser{} }

func (p *nativeParser) Parse(ctx context.Context, language string, source []byte) (SyntaxNode, error) {
	session := tsnative.NewSession()
	defer session.Close()
	return parseNativeTree(ctx, session, language, source)
}

func (p *nativeParser) ExtractFacts(ctx context.Context, language, grammar string, source []byte) (FileResult, error) {
	session := tsnative.NewSession()
	defer session.Close()
	return extractNativeFacts(ctx, session, language, grammar, source)
}

func (p *nativeParser) NewParseSession(context.Context) ParseSession {
	return &nativeParseSession{session: tsnative.NewSession()}
}

type nativeParseSession struct {
	session *tsnative.Session
}

func (s *nativeParseSession) Parse(ctx context.Context, language string, source []byte) (SyntaxNode, error) {
	if s == nil || s.session == nil {
		return SyntaxNode{}, fmt.Errorf("native parser session is closed")
	}
	return parseNativeTree(ctx, s.session, language, source)
}

func (s *nativeParseSession) ExtractFacts(ctx context.Context, language, grammar string, source []byte) (FileResult, error) {
	if s == nil || s.session == nil {
		return FileResult{}, fmt.Errorf("native parser session is closed")
	}
	return extractNativeFacts(ctx, s.session, language, grammar, source)
}

func (s *nativeParseSession) Close(context.Context) error {
	if s != nil && s.session != nil {
		s.session.Close()
		s.session = nil
	}
	return nil
}

func extractNativeFacts(ctx context.Context, session *tsnative.Session, language, grammar string, source []byte) (FileResult, error) {
	if !tsnative.Supported(grammar) {
		return FileResult{}, fmt.Errorf("native grammar %s is not linked", grammar)
	}
	tree, err := session.ParseTree(ctx, grammar, source)
	if err != nil {
		return FileResult{}, err
	}
	defer tree.Close()
	root := tree.RootNode()
	if root == nil {
		return FileResult{}, fmt.Errorf("native parse %s returned no tree", grammar)
	}
	syntax := tsRootView(root)
	spec, ok := PinnedLanguageSpec(language)
	if !ok {
		return ExtractSyntaxFacts(language, syntax, source)
	}
	sets := syntaxSpecSets(spec)
	if lang, err := tsnative.Language(grammar); err == nil {
		attachSpecKindIDs(&sets, lang)
	}
	return extractSyntaxFactsWithSets(language, syntax, source, sets)
}

func attachSpecKindIDs(sets *specSets, lang *tree_sitter.Language) {
	if sets == nil || lang == nil {
		return
	}
	kindIDs := func(names map[string]bool) map[uint16]bool {
		ids := make(map[uint16]bool, len(names)*2)
		for name := range names {
			if id := lang.IdForNodeKind(name, true); id != 0 {
				ids[id] = true
			}
			if id := lang.IdForNodeKind(name, false); id != 0 {
				ids[id] = true
			}
		}
		return ids
	}
	sets.functionIDs = kindIDs(sets.functions)
	sets.classIDs = kindIDs(sets.classes)
	sets.fieldIDs = kindIDs(sets.fields)
	sets.moduleIDs = kindIDs(sets.modules)
	sets.callIDs = kindIDs(sets.calls)
	sets.variableIDs = kindIDs(sets.variables)
}

func parseNativeTree(ctx context.Context, session *tsnative.Session, grammar string, source []byte) (SyntaxNode, error) {
	if !tsnative.Supported(grammar) {
		return SyntaxNode{}, fmt.Errorf("native grammar %s is not linked", grammar)
	}
	tree, err := session.ParseTree(ctx, grammar, source)
	if err != nil {
		return SyntaxNode{}, err
	}
	defer tree.Close()
	root := tree.RootNode()
	if root == nil {
		return SyntaxNode{}, fmt.Errorf("native parse %s returned no tree", grammar)
	}
	return syntaxNodeFromTSCursor(root), nil
}

func syntaxNodeFromTSCursor(root *tree_sitter.Node) SyntaxNode {
	if root == nil {
		return SyntaxNode{}
	}
	cursor := root.Walk()
	defer cursor.Close()
	return syntaxNodeFromCursor(cursor)
}

func syntaxNodeFromCursor(cursor *tree_sitter.TreeCursor) SyntaxNode {
	n := cursor.Node()
	out := SyntaxNode{
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
		out.Children = append(out.Children, syntaxNodeFromCursor(cursor))
		if !cursor.GotoNextSibling() {
			break
		}
	}
	cursor.GotoParent()
	return out
}

var _ SyntaxParser = (*nativeParser)(nil)
var _ parseSessionFactory = (*nativeParser)(nil)
var _ ParseSession = (*nativeParseSession)(nil)
var _ factExtractor = (*nativeParser)(nil)
var _ factExtractor = (*nativeParseSession)(nil)
