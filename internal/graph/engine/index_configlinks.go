package engine

import (
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func indexConfigLinks(graph *goGraph) {
	type entry struct {
		qn, name, normalized string
	}
	var configs, code []entry
	nodes := append([]api.Node(nil), graph.nodes...)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].QualifiedName < nodes[j].QualifiedName })
	for _, node := range nodes {
		normalized, tokens := normalizeConfigKey(node.Name)
		if normalized == "" {
			continue
		}
		if node.Label == "Variable" && hasConfigExtension(node.Location.File) {
			if len(tokens) < 2 || !allConfigTokensLong(tokens) {
				continue
			}
			configs = append(configs, entry{node.QualifiedName, node.Name, normalized})
			continue
		}
		if !hasConfigExtension(node.Location.File) {
			switch node.Label {
			case "Function", "Variable", "Class", "Struct":
				code = append(code, entry{qn: node.QualifiedName, normalized: normalized})
			}
		}
	}
	for _, config := range configs {
		for _, symbol := range code {
			confidence := 0.0
			if symbol.normalized == config.normalized {
				confidence = .85
			} else if strings.Contains(symbol.normalized, config.normalized) {
				confidence = .75
			}
			if confidence == 0 {
				continue
			}
			graph.edges = append(graph.edges, pendingEdge{source: symbol.qn, target: config.qn, kind: "CONFIGURES",
				properties: api.Properties{"strategy": "key_symbol", "confidence": confidence, "config_key": config.name},
				evidence:   &api.Evidence{Strategy: "key_symbol", Confidence: confidence}})
		}
	}
	sortGraph(graph)
}

func normalizeConfigKey(value string) (string, []string) {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == '_' || r == '-' || r == '.' || unicode.IsSpace(r) })
	var tokens []string
	for _, part := range parts {
		start := 0
		for index := 1; index < len(part); index++ {
			if part[index] >= 'A' && part[index] <= 'Z' && part[index-1] >= 'a' && part[index-1] <= 'z' {
				tokens = append(tokens, strings.ToLower(part[start:index]))
				start = index
			}
		}
		if start < len(part) {
			tokens = append(tokens, strings.ToLower(part[start:]))
		}
	}
	return strings.Join(tokens, "_"), tokens
}

func allConfigTokensLong(tokens []string) bool {
	for _, token := range tokens {
		if len(token) < 3 {
			return false
		}
	}
	return true
}

func hasConfigExtension(path string) bool {
	base := filepath.Base(path)
	if base == ".env" {
		return true
	}
	switch strings.ToLower(filepath.Ext(base)) {
	case ".toml", ".ini", ".yaml", ".yml", ".cfg", ".properties", ".json", ".xml", ".conf", ".env":
		return true
	default:
		return false
	}
}
