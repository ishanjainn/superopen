package engine

import (
	"context"
	"errors"
)

type factExtractor interface {
	ExtractFacts(ctx context.Context, language, grammar string, source []byte) (FileResult, error)
}

type fallbackSyntaxParser struct {
	native SyntaxParser
	wasm   *GrammarRuntime
}

func (p *fallbackSyntaxParser) Parse(ctx context.Context, language string, source []byte) (SyntaxNode, error) {
	if p == nil {
		return SyntaxNode{}, errors.New("syntax parser is required")
	}
	if p.native != nil {
		if tree, err := p.native.Parse(ctx, language, source); err == nil {
			return tree, nil
		}
	}
	if p.wasm == nil {
		return SyntaxNode{}, errors.New("grammar runtime is closed")
	}
	return p.wasm.Parse(ctx, language, source)
}

func (p *fallbackSyntaxParser) ExtractFacts(ctx context.Context, language, grammar string, source []byte) (FileResult, error) {
	if extractor, ok := p.native.(factExtractor); ok {
		if result, err := extractor.ExtractFacts(ctx, language, grammar, source); err == nil {
			return result, nil
		}
	}
	if p.wasm != nil {
		return p.wasm.ExtractFacts(ctx, language, grammar, source)
	}
	tree, err := p.Parse(ctx, grammar, source)
	if err != nil {
		return FileResult{}, err
	}
	return ExtractSyntaxFacts(language, tree, source)
}

func (p *fallbackSyntaxParser) NewParseSession(ctx context.Context) ParseSession {
	var native SyntaxParser
	if factory, ok := p.native.(parseSessionFactory); ok {
		native = factory.NewParseSession(ctx)
	} else {
		native = p.native
	}
	var wasm ParseSession
	if p.wasm != nil {
		wasm = p.wasm.NewParseSession(ctx)
	}
	return &fallbackParseSession{native: native, wasm: wasm}
}

type fallbackParseSession struct {
	native SyntaxParser
	wasm   ParseSession
}

func (s *fallbackParseSession) Parse(ctx context.Context, language string, source []byte) (SyntaxNode, error) {
	if s.native != nil {
		if tree, err := s.native.Parse(ctx, language, source); err == nil {
			return tree, nil
		}
	}
	if s.wasm == nil {
		return SyntaxNode{}, errors.New("grammar runtime is closed")
	}
	return s.wasm.Parse(ctx, language, source)
}

func (s *fallbackParseSession) ExtractFacts(ctx context.Context, language, grammar string, source []byte) (FileResult, error) {
	if extractor, ok := s.native.(factExtractor); ok {
		if result, err := extractor.ExtractFacts(ctx, language, grammar, source); err == nil {
			return result, nil
		}
	}
	if extractor, ok := s.wasm.(factExtractor); ok {
		return extractor.ExtractFacts(ctx, language, grammar, source)
	}
	tree, err := s.Parse(ctx, grammar, source)
	if err != nil {
		return FileResult{}, err
	}
	return ExtractSyntaxFacts(language, tree, source)
}

func (s *fallbackParseSession) Close(ctx context.Context) error {
	var first error
	if closer, ok := s.native.(ParseSession); ok {
		first = closer.Close(ctx)
	}
	if s.wasm != nil {
		if err := s.wasm.Close(ctx); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func parserIndexMode(parser SyntaxParser) string {
	switch parser.(type) {
	case *fallbackSyntaxParser, *fallbackParseSession:
		return "tree-sitter-native"
	default:
		return "tree-sitter-wasm"
	}
}

var _ SyntaxParser = (*fallbackSyntaxParser)(nil)
var _ parseSessionFactory = (*fallbackSyntaxParser)(nil)
var _ ParseSession = (*fallbackParseSession)(nil)
var _ factExtractor = (*fallbackSyntaxParser)(nil)
var _ factExtractor = (*fallbackParseSession)(nil)
