package engine

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// enrichGoResolvedCalls fills FileResult.ResolvedCalls for Go sources using
// the pinned import-map and same-module strategies before assemble joins them.
func enrichGoResolvedCalls(ctx context.Context, root string, files []ParsedSyntaxFile) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fileSet := token.NewFileSet()
	for index := range files {
		parsed := &files[index]
		if parsed.File.Language != "go" || len(parsed.Body) == 0 {
			continue
		}
		abs := filepath.Join(root, filepath.FromSlash(parsed.File.Path))
		file, err := parser.ParseFile(fileSet, abs, parsed.Body, parser.AllErrors)
		if err != nil || file == nil {
			continue
		}
		module := syntaxDefinitionModuleQN("go", parsed.File.Path)
		imports := map[string]string{}
		for _, fact := range parsed.Extraction.Imports {
			if fact.LocalName != "" {
				imports[fact.LocalName] = fact.Name
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			callee := goCallName(call.Fun)
			if callee == "" || goBuiltinCall(callee) {
				return true
			}
			pos := fileSet.Position(call.Pos())
			source := fileQualifiedName(parsed.File.Path)
			if owner := syntaxScopeOwner(parsed, pos.Line); owner != "" {
				source = owner
			}
			target, strategy, confidence := resolveGoTextualCall(callee, module, imports)
			if target == "" {
				return true
			}
			parsed.Extraction.ResolvedCalls = append(parsed.Extraction.ResolvedCalls, ResolvedRelationship{
				Source: source, Target: target, Type: "CALLS", Strategy: strategy, Confidence: confidence,
				Location:   api.Location{File: parsed.File.Path, StartLine: pos.Line, StartColumn: pos.Column},
				Properties: api.Properties{"callee": callee},
			})
			return true
		})
	}
	return nil
}

func resolveGoTextualCall(callee, module string, imports map[string]string) (string, string, float64) {
	if dot := strings.IndexByte(callee, '.'); dot >= 0 {
		prefix := callee[:dot]
		if imported, ok := imports[prefix]; ok && imported != "" {
			return imported + callee[dot:], "import_map", .95
		}
	}
	if !strings.Contains(callee, ".") {
		return module + "." + callee, "same_module", .9
	}
	return "", "", 0
}

func syntaxScopeOwner(parsed *ParsedSyntaxFile, line int) string {
	owner := ""
	ownerSpan := ^uint32(0)
	for _, fact := range parsed.Extraction.Definitions {
		if fact.Kind != "function" || fact.StartLine > line || line > fact.EndLine {
			continue
		}
		if span := uint32(fact.EndLine - fact.StartLine); span < ownerSpan {
			owner = joinSyntaxScope(syntaxDefinitionModuleQN(parsed.File.Language, parsed.File.Path), joinSyntaxScope(fact.Scope, fact.Name))
			ownerSpan = span
		}
	}
	return owner
}

func joinResolvedCalls(graph *goGraph, files []ParsedSyntaxFile, registry symbolRegistry) {
	for _, parsed := range files {
		rel := filepath.ToSlash(parsed.File.Path)
		moduleQN := syntaxDefinitionModuleQN(parsed.File.Language, rel)
		imports := map[string]string{}
		for _, fact := range parsed.Extraction.Imports {
			if fact.LocalName != "" {
				imports[fact.LocalName] = fact.Name
			}
		}
		for _, relationship := range parsed.Extraction.ResolvedCalls {
			if relationship.Type != "CALLS" || relationship.Target == "" || relationship.Source == "" {
				continue
			}
			callee, _ := relationship.Properties["callee"].(string)
			if callee == "" {
				continue
			}
			graph.edges = append(graph.edges, pendingEdge{
				source: relationship.Source, target: relationship.Target, kind: "CALLS",
				properties: api.Properties{
					"callee": callee, "strategy": relationship.Strategy, "confidence": relationship.Confidence,
				},
				evidence: &api.Evidence{
					Strategy: relationship.Strategy, Confidence: relationship.Confidence,
					Location: locationPointer(relationship.Location),
				},
			})
			_ = moduleQN
			_ = imports
			_ = registry
		}
	}
	sortGraph(graph)
}
