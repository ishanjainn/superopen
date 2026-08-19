package engine

import (
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// symbolRegistry is the Go representation of Superopen's observable registry
// chain: import map, same module, unique name, then proximity-ranked suffix.
// It intentionally resolves only published definition labels.
type symbolRegistry struct {
	exact  map[string]api.Node
	byName map[string][]api.Node
}

type symbolResolution struct {
	qn         string
	strategy   string
	confidence float64
	candidates int
}

// applyFieldTypeHint ports try_field_type_hint from the pinned parallel call
// pass. It is intentionally lexical rather than type-correct: the receiver
// variable name is capitalized and matched against candidate qualified names.
// This quirk is part of Superopen's observable resolution behavior.
func (registry symbolRegistry) applyFieldTypeHint(callee, source string, resolution symbolResolution) symbolResolution {
	if resolution.qn == "" || resolution.candidates <= 1 {
		return resolution
	}
	dot := strings.IndexByte(callee, '.')
	if dot < 0 {
		return resolution
	}
	typeHint := callee[:dot]
	typeHint = strings.TrimPrefix(typeHint, "_")
	typeHint = strings.TrimPrefix(typeHint, "m_")
	if typeHint == "" {
		return resolution
	}
	typeName := strings.ToUpper(typeHint[:1]) + typeHint[1:]
	interfaceName := "I" + typeName
	method := callee[dot+1:]
	for _, candidate := range registry.byName[method] {
		if candidate.QualifiedName != source &&
			(strings.Contains(candidate.QualifiedName, typeName) || strings.Contains(candidate.QualifiedName, interfaceName)) {
			resolution.qn = candidate.QualifiedName
			resolution.strategy = "field_type_hint"
			resolution.confidence = .85
			return resolution
		}
	}
	return resolution
}

func newSymbolRegistry(nodes []api.Node) symbolRegistry {
	registry := symbolRegistry{exact: map[string]api.Node{}, byName: map[string][]api.Node{}}
	for _, node := range nodes {
		if external, _ := node.Properties["external"].(bool); external || !registryDefinitionLabel(node.Label) {
			continue
		}
		registry.exact[node.QualifiedName] = node
		registry.byName[node.Name] = append(registry.byName[node.Name], node)
	}
	return registry
}

func registryDefinitionLabel(label string) bool {
	switch label {
	case "Function", "Method", "Class", "Struct", "Interface", "Enum", "Type", "Trait", "Variable", "Field":
		return true
	default:
		return false
	}
}

func (registry symbolRegistry) resolve(callee, module string, imports map[string]string) symbolResolution {
	prefix, suffix := splitRegistryCallee(callee)
	if resolved := imports[prefix]; resolved != "" {
		if suffix == "" {
			if _, ok := registry.exact[resolved]; ok {
				return symbolResolution{qn: resolved, strategy: "import_map", confidence: .95, candidates: 1}
			}
		}
		candidate := resolved + "." + prefix
		if suffix != "" {
			candidate = resolved + "." + suffix
		}
		if _, ok := registry.exact[candidate]; ok {
			return symbolResolution{qn: candidate, strategy: "import_map", confidence: .95, candidates: 1}
		}
		if suffix != "" {
			prefixDot, suffixDot := resolved+".", "."+suffix
			for _, node := range registry.byName[simpleRegistryName(suffix)] {
				if strings.HasPrefix(node.QualifiedName, prefixDot) && strings.HasSuffix(node.QualifiedName, suffixDot) {
					return symbolResolution{qn: node.QualifiedName, strategy: "import_map_suffix", confidence: .85, candidates: 1}
				}
			}
		}
	}
	for _, name := range []string{callee, suffix} {
		if name == "" {
			continue
		}
		candidate := module + "." + name
		if _, ok := registry.exact[candidate]; ok {
			return symbolResolution{qn: candidate, strategy: "same_module", confidence: .9, candidates: 1}
		}
	}
	candidates := registry.byName[simpleRegistryName(callee)]
	if len(candidates) == 0 || len(candidates) > 256 {
		return symbolResolution{}
	}
	if len(candidates) == 1 {
		confidence := .75
		if len(imports) > 0 && !registryImportReachable(candidates[0].QualifiedName, imports) {
			confidence *= .5
		}
		return symbolResolution{qn: candidates[0].QualifiedName, strategy: "unique_name", confidence: confidence, candidates: 1}
	}
	if qualified := registryQualifiedSuffix(callee, candidates); qualified != "" {
		return symbolResolution{qn: qualified, strategy: "qualified_suffix", confidence: .9, candidates: len(candidates)}
	}
	reachable := make([]api.Node, 0, len(candidates))
	if len(imports) > 0 {
		for _, candidate := range candidates {
			if registryImportReachable(candidate.QualifiedName, imports) {
				reachable = append(reachable, candidate)
			}
		}
	}
	pool, confidence, penaltyCount := candidates, .55, len(candidates)
	if len(reachable) > 0 {
		pool = reachable
		penaltyCount = len(reachable)
	} else if len(imports) > 0 {
		confidence *= .5
	}
	if len(reachable) == 1 {
		penaltyCount = len(candidates)
	}
	best := registryBestCandidate(pool, module)
	return symbolResolution{qn: best.QualifiedName, strategy: "suffix_match", confidence: registryCandidatePenalty(confidence, penaltyCount), candidates: penaltyCount}
}

func splitRegistryCallee(callee string) (string, string) {
	dot := strings.IndexByte(callee, '.')
	colon := strings.Index(callee, "::")
	index, width := dot, 1
	if colon >= 0 && (index < 0 || colon < index) {
		index, width = colon, 2
	}
	if index < 0 {
		return callee, ""
	}
	return callee[:index], callee[index+width:]
}

func simpleRegistryName(value string) string {
	value = strings.ReplaceAll(value, "::", ".")
	if index := strings.LastIndexByte(value, '.'); index >= 0 {
		return value[index+1:]
	}
	return value
}

func registryQualifiedSuffix(callee string, candidates []api.Node) string {
	dotted := strings.ReplaceAll(callee, "::", ".")
	if !strings.Contains(dotted, ".") {
		return ""
	}
	match := ""
	for _, candidate := range candidates {
		if candidate.QualifiedName == dotted || strings.HasSuffix(candidate.QualifiedName, "."+dotted) {
			if match != "" {
				return ""
			}
			match = candidate.QualifiedName
		}
	}
	return match
}

func registryImportReachable(candidate string, imports map[string]string) bool {
	module := candidate
	if index := strings.LastIndexByte(module, '.'); index >= 0 {
		module = module[:index]
	}
	for _, imported := range imports {
		if strings.Contains(module, imported) || strings.Contains(imported, module) {
			return true
		}
	}
	return false
}

func registryBestCandidate(candidates []api.Node, module string) api.Node {
	best, bestScore := api.Node{}, -1
	for _, candidate := range candidates {
		score := registryCommonPrefix(candidate.QualifiedName, module)
		if !isTestQualifiedName(candidate.QualifiedName) {
			score += 1000
		}
		if score > bestScore {
			best, bestScore = candidate, score
		}
	}
	return best
}

// isTestQualifiedName mirrors is_test_qn Superopen, whose substring list is
// case-sensitive and deliberately asymmetric: `Fixture` but not `fixture`,
// `spec` but not `Spec`.
func isTestQualifiedName(qn string) bool {
	for _, marker := range []string{"Test", "test", "Mock", "mock", "Stub", "stub",
		"Fake", "fake", "Fixture", "spec"} {
		if strings.Contains(qn, marker) {
			return true
		}
	}
	return false
}

func registryCommonPrefix(left, right string) int {
	a, b := strings.Split(left, "."), strings.Split(right, ".")
	count := 0
	for count < len(a) && count < len(b) && a[count] == b[count] {
		count++
	}
	return count
}

func registryCandidatePenalty(base float64, count int) float64 {
	if count <= 3 {
		return base
	}
	return base * (3 / float64(count))
}
