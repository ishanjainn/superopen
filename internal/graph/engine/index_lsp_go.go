package engine

import (
	"github.com/ishanjainn/superopen/internal/graph/api"
)

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

func joinResolvedCalls(graph *goGraph, files []ParsedSyntaxFile) {
	for _, parsed := range files {
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
		}
	}
}
