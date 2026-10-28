package engine

import (
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

type methodSet map[string]string

// indexSyntaxGoImplements implements Superopen on the generic
// syntax graph: struct method sets are matched against interface requirements.
func indexSyntaxGoImplements(graph *goGraph) {
	structMethods := map[string]methodSet{}
	interfaces := map[string]methodSet{}
	for _, node := range graph.nodes {
		if node.Label != "Struct" && node.Label != "Interface" {
			continue
		}
		if node.Label == "Struct" {
			structMethods[node.QualifiedName] = methodSet{}
		} else {
			interfaces[node.QualifiedName] = methodSet{}
		}
	}
	for _, edge := range graph.edges {
		if edge.kind != "DEFINES_METHOD" {
			continue
		}
		if methods, ok := structMethods[edge.source]; ok {
			methods[graphNodeName(edge.target)] = edge.target
		}
		if required, ok := interfaces[edge.source]; ok {
			required[graphNodeName(edge.target)] = edge.target
		}
	}
	for _, edge := range graph.edges {
		if edge.kind != "INHERITS" {
			continue
		}
		if required, ok := interfaces[edge.target]; ok && len(required) > 0 {
			interfaces[edge.source] = required
		}
	}
	structsByMethod := map[string][]string{}
	for concrete, methods := range structMethods {
		for name := range methods {
			structsByMethod[name] = append(structsByMethod[name], concrete)
		}
	}
	for iface, required := range interfaces {
		if len(required) == 0 {
			continue
		}
		var candidates []string
		first := true
		for name := range required {
			owners := structsByMethod[name]
			if first {
				candidates = append(candidates, owners...)
				first = false
				continue
			}
			allowed := make(map[string]bool, len(owners))
			for _, owner := range owners {
				allowed[owner] = true
			}
			filtered := candidates[:0]
			for _, concrete := range candidates {
				if allowed[concrete] {
					filtered = append(filtered, concrete)
				}
			}
			candidates = filtered
			if len(candidates) == 0 {
				break
			}
		}
		for _, concrete := range candidates {
			methods := structMethods[concrete]
			if concrete == iface || !containsMethods(methods, required) {
				continue
			}
			graph.edges = append(graph.edges, pendingEdge{
				source: concrete, target: iface, kind: "IMPLEMENTS",
				properties: api.Properties{"method_count": len(required)},
				evidence:   &api.Evidence{Strategy: "go_method_set", Confidence: 0.9},
			})
			for name, interfaceMethod := range required {
				if methodQN := methods[name]; methodQN != "" {
					graph.edges = append(graph.edges, pendingEdge{
						source: methodQN, target: interfaceMethod, kind: "OVERRIDE",
						evidence: &api.Evidence{Strategy: "go_method_set", Confidence: 0.9},
					})
				}
			}
		}
	}
}

func graphNodeName(qn string) string {
	if index := len(qn) - 1; index >= 0 {
		for index >= 0 && qn[index] != '.' {
			index--
		}
		if index >= 0 && index+1 < len(qn) {
			return qn[index+1:]
		}
	}
	return qn
}

// indexSyntaxInheritance matches Superopen resolve_def_inherits: base_classes
// resolve through the registry to a type-like target, then emit INHERITS or
// IMPLEMENTS based on the *target* label (Interface → IMPLEMENTS).
func indexSyntaxInheritance(_ string, files []ParsedSyntaxFile, graph *goGraph, registry symbolRegistry) {
	nodeQNs := make(map[string]bool, len(graph.nodes))
	for _, node := range graph.nodes {
		nodeQNs[node.QualifiedName] = true
	}
	importIndex := newImportTargetIndex(graph.nodes)
	for _, parsed := range files {
		rel := filepath.ToSlash(parsed.File.Path)
		moduleQN := syntaxDefinitionModuleQN(parsed.File.Language, rel)
		imports := map[string]string{}
		for _, fact := range parsed.Extraction.Imports {
			if fact.LocalName == "" {
				continue
			}
			qn := localSyntaxImportTargetForLanguage(parsed.File.Language, rel, fact.Name, "", importIndex)
			if qn == "" {
				continue
			}
			imports[fact.LocalName] = qn
		}
		for _, fact := range parsed.Extraction.Inheritance {
			if fact.Scope == "" {
				continue
			}
			source := joinSyntaxScope(moduleQN, fact.Scope)
			if !nodeQNs[source] {
				continue
			}
			resolution := registry.resolve(fact.Name, moduleQN, imports)
			if resolution.qn == "" || resolution.qn == source {
				continue
			}
			target, ok := registry.exact[resolution.qn]
			if !ok || !syntaxTypeLikeLabel(target.Label) {
				continue
			}
			kind := semanticBaseEdgeType(target.Label)
			graph.edges = append(graph.edges, pendingEdge{
				source: source, target: resolution.qn, kind: kind,
				evidence: syntaxEvidence(rel, fact, resolution.strategy, resolution.confidence),
			})
		}
	}
}

// indexSyntaxExplicitOverrides links each method to the nearest ancestor
// method with the same name. All declarations at the same depth are kept.
func indexSyntaxExplicitOverrides(graph *goGraph) {
	methodsByType := map[string]methodSet{}
	fileByQN := map[string]string{}
	for _, node := range graph.nodes {
		fileByQN[node.QualifiedName] = node.Location.File
		switch node.Label {
		case "Class", "Struct", "Interface", "Enum", "Type", "Trait":
			methodsByType[node.QualifiedName] = methodSet{}
		}
	}
	for _, edge := range graph.edges {
		if edge.kind != "DEFINES_METHOD" {
			continue
		}
		if methods, ok := methodsByType[edge.source]; ok {
			methods[graphNodeName(edge.target)] = edge.target
		}
	}
	parents := map[string][]string{}
	for _, edge := range graph.edges {
		if edge.kind != "INHERITS" && edge.kind != "IMPLEMENTS" {
			continue
		}
		if _, ok := methodsByType[edge.source]; !ok {
			continue
		}
		if _, ok := methodsByType[edge.target]; !ok {
			continue
		}
		parents[edge.source] = append(parents[edge.source], edge.target)
	}
	var added []pendingEdge
	for child, childMethods := range methodsByType {
		if len(childMethods) == 0 || len(parents[child]) == 0 || strings.HasSuffix(strings.ToLower(fileByQN[child]), ".go") {
			continue
		}
		bound := map[string]bool{}
		visited := map[string]bool{child: true}
		level := append([]string(nil), parents[child]...)
		for len(level) > 0 && len(bound) < len(childMethods) {
			var next []string
			boundNow := map[string]bool{}
			for _, ancestor := range level {
				if visited[ancestor] {
					continue
				}
				visited[ancestor] = true
				for name, childMethod := range childMethods {
					baseMethod := methodsByType[ancestor][name]
					if bound[name] || baseMethod == "" || baseMethod == childMethod {
						continue
					}
					added = append(added, pendingEdge{
						source: childMethod, target: baseMethod, kind: "OVERRIDE",
						evidence: &api.Evidence{Strategy: "explicit_override", Confidence: 0.9},
					})
					boundNow[name] = true
				}
				next = append(next, parents[ancestor]...)
			}
			for name := range boundNow {
				bound[name] = true
			}
			level = next
		}
	}
	graph.edges = append(graph.edges, added...)
}

func semanticBaseEdgeType(label string) string {
	if label == "Interface" {
		return "IMPLEMENTS"
	}
	return "INHERITS"
}

func syntaxTypeLikeLabel(label string) bool {
	switch label {
	case "Class", "Struct", "Interface", "Enum", "Type", "Trait":
		return true
	default:
		return false
	}
}
