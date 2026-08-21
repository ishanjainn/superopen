package engine

import (
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

const (
	configLinkConfigCap = 4096
	configLinkCodeCap   = 8192
	configLinkDepCap    = 2048
)

type configLinkEntry struct {
	qn, name, normalized string
	file                 string
	startLine            int
}

func indexConfigLinks(graph *goGraph) {
	if graph == nil {
		return
	}
	hasConfig := false
	for _, node := range graph.nodes {
		if node.Label == "Variable" && hasConfigExtension(node.Location.File) ||
			node.Label == "Module" && hasConfigExtension(node.Location.File) {
			hasConfig = true
			break
		}
	}
	if !hasConfig {
		return
	}
	indexConfigKeySymbols(graph)
	indexConfigDepImports(graph)
}

func indexConfigKeySymbols(graph *goGraph) {
	configs := collectConfigEntries(graph.nodes, configLinkConfigCap)
	if len(configs) == 0 {
		return
	}
	code := collectCodeEntries(graph.nodes, configLinkCodeCap)
	if len(code) == 0 {
		return
	}
	exact := make(map[string][]configLinkEntry, len(code))
	for _, symbol := range code {
		exact[symbol.normalized] = append(exact[symbol.normalized], symbol)
	}
	seen := map[string]struct{}{}
	for _, config := range configs {
		for _, symbol := range exact[config.normalized] {
			emitConfigKeyEdge(graph, symbol, config, .85, seen)
		}
		for _, symbol := range code {
			if symbol.normalized == config.normalized {
				continue
			}
			if !strings.Contains(symbol.normalized, config.normalized) {
				continue
			}
			emitConfigKeyEdge(graph, symbol, config, .75, seen)
		}
	}
}

func emitConfigKeyEdge(graph *goGraph, symbol, config configLinkEntry, confidence float64, seen map[string]struct{}) {
	key := symbol.qn + "\x00" + config.qn
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	graph.edges = append(graph.edges, pendingEdge{source: symbol.qn, target: config.qn, kind: "CONFIGURES",
		properties: api.Properties{"strategy": "key_symbol", "confidence": confidence, "config_key": config.name},
		evidence:   &api.Evidence{Strategy: "key_symbol", Confidence: confidence}})
}

func collectConfigEntries(nodes []api.Node, cap int) []configLinkEntry {
	vars := make([]api.Node, 0, 64)
	for _, node := range nodes {
		if node.Label == "Variable" && hasConfigExtension(node.Location.File) {
			vars = append(vars, node)
		}
	}
	sortConfigLinkNodes(vars)
	out := make([]configLinkEntry, 0, minInt(len(vars), cap))
	for _, node := range vars {
		if len(out) >= cap {
			break
		}
		normalized, tokens := normalizeConfigKey(node.Name)
		if len(tokens) < 2 || !allConfigTokensLong(tokens) {
			continue
		}
		out = append(out, configLinkEntry{
			qn: node.QualifiedName, name: node.Name, normalized: normalized,
			file: node.Location.File, startLine: node.Location.StartLine,
		})
	}
	return out
}

func collectCodeEntries(nodes []api.Node, cap int) []configLinkEntry {
	out := make([]configLinkEntry, 0, minInt(len(nodes), cap))
	for _, label := range []string{"Function", "Variable", "Class", "Struct"} {
		group := make([]api.Node, 0)
		for _, node := range nodes {
			if node.Label != label || hasConfigExtension(node.Location.File) {
				continue
			}
			group = append(group, node)
		}
		sortConfigLinkNodes(group)
		for _, node := range group {
			if len(out) >= cap {
				return out
			}
			normalized, tokens := normalizeConfigKey(node.Name)
			if normalized == "" || len(tokens) == 0 {
				continue
			}
			out = append(out, configLinkEntry{qn: node.QualifiedName, normalized: normalized})
		}
	}
	return out
}

func sortConfigLinkNodes(nodes []api.Node) {
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].QualifiedName != nodes[j].QualifiedName {
			return nodes[i].QualifiedName < nodes[j].QualifiedName
		}
		if nodes[i].Location.File != nodes[j].Location.File {
			return nodes[i].Location.File < nodes[j].Location.File
		}
		if nodes[i].Location.StartLine != nodes[j].Location.StartLine {
			return nodes[i].Location.StartLine < nodes[j].Location.StartLine
		}
		return nodes[i].Name < nodes[j].Name
	})
}

func indexConfigDepImports(graph *goGraph) {
	deps := collectManifestDeps(graph.nodes, configLinkDepCap)
	if len(deps) == 0 {
		return
	}
	nodesByQN := make(map[string]api.Node, len(graph.nodes))
	for _, node := range graph.nodes {
		nodesByQN[node.QualifiedName] = node
	}
	type importEdge struct {
		source, target string
	}
	var imports []importEdge
	for _, edge := range graph.edges {
		if edge.kind == "IMPORTS" {
			imports = append(imports, importEdge{source: edge.source, target: edge.target})
		}
	}
	if len(imports) == 0 {
		return
	}
	for _, dep := range deps {
		depLower := strings.ToLower(dep.name)
		for _, im := range imports {
			target, ok := nodesByQN[im.target]
			if !ok {
				continue
			}
			if _, ok := nodesByQN[im.source]; !ok {
				continue
			}
			confidence := matchDepToImport(target, depLower)
			if confidence == 0 {
				continue
			}
			graph.edges = append(graph.edges, pendingEdge{source: im.source, target: dep.qn, kind: "CONFIGURES",
				properties: api.Properties{"strategy": "dependency_import", "confidence": confidence, "dep_name": dep.name},
				evidence:   &api.Evidence{Strategy: "dependency_import", Confidence: confidence}})
		}
	}
}

func collectManifestDeps(nodes []api.Node, cap int) []configLinkEntry {
	out := make([]configLinkEntry, 0)
	for _, node := range nodes {
		if len(out) >= cap {
			break
		}
		if node.Label != "Variable" {
			continue
		}
		base := filepath.Base(node.Location.File)
		if !isManifestFile(base) {
			continue
		}
		if !isDepSection(node.QualifiedName) && !(base == "Cargo.toml" && isCargoDepSection(node.QualifiedName)) {
			continue
		}
		out = append(out, configLinkEntry{qn: node.QualifiedName, name: node.Name})
	}
	return out
}

func isManifestFile(base string) bool {
	switch base {
	case "Cargo.toml", "package.json", "go.mod", "requirements.txt", "Gemfile", "build.gradle", "pom.xml", "composer.json":
		return true
	default:
		return false
	}
}

func isDepSection(qn string) bool {
	lower := strings.ToLower(qn)
	for _, section := range []string{"dependencies", "devdependencies", "peerdependencies", "dev-dependencies", "build-dependencies"} {
		if strings.Contains(lower, section) {
			return true
		}
	}
	return false
}

func isCargoDepSection(qn string) bool {
	for _, part := range strings.Split(qn, ".") {
		switch strings.ToLower(part) {
		case "dependencies", "devdependencies", "peerdependencies", "dev-dependencies", "build-dependencies":
			return true
		}
	}
	return false
}

func matchDepToImport(target api.Node, depLower string) float64 {
	if strings.ToLower(target.Name) == depLower {
		return .95
	}
	if target.QualifiedName != "" && strings.Contains(strings.ToLower(target.QualifiedName), depLower) {
		return .80
	}
	return 0
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
