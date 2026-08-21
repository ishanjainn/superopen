//go:build tsnative && cgo

package tsnative

import (
	"context"
	"fmt"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type Session struct {
	parsers map[string]*tree_sitter.Parser
}

func NewSession() *Session {
	return &Session{parsers: map[string]*tree_sitter.Parser{}}
}

func (s *Session) Close() {
	if s == nil {
		return
	}
	for grammar, parser := range s.parsers {
		parser.Close()
		delete(s.parsers, grammar)
	}
}

func (s *Session) Parse(ctx context.Context, grammar string, source []byte) (Node, error) {
	if s == nil {
		return Node{}, fmt.Errorf("native parser session is closed")
	}
	lang, err := language(grammar)
	if err != nil {
		return Node{}, err
	}
	parser := s.parsers[grammar]
	if parser == nil {
		parser = tree_sitter.NewParser()
		if err := parser.SetLanguage(lang); err != nil {
			parser.Close()
			return Node{}, err
		}
		s.parsers[grammar] = parser
	}
	tree := parser.ParseCtx(ctx, source, nil)
	if tree == nil {
		if err := ctx.Err(); err != nil {
			return Node{}, err
		}
		return Node{}, fmt.Errorf("native parse %s failed", grammar)
	}
	defer tree.Close()
	return Convert(tree.RootNode()), nil
}

func (s *Session) ParseTree(ctx context.Context, grammar string, source []byte) (*tree_sitter.Tree, error) {
	if s == nil {
		return nil, fmt.Errorf("native parser session is closed")
	}
	lang, err := language(grammar)
	if err != nil {
		return nil, err
	}
	parser := s.parsers[grammar]
	if parser == nil {
		parser = tree_sitter.NewParser()
		if err := parser.SetLanguage(lang); err != nil {
			parser.Close()
			return nil, err
		}
		s.parsers[grammar] = parser
	}
	tree := parser.ParseCtx(ctx, source, nil)
	if tree == nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("native parse %s failed", grammar)
	}
	return tree, nil
}
