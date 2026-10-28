package engine

import "strings"

// Resolve Rust UFCS/associated calls against the named type before its
// short-name fallback. Abstract trait signatures are not concrete targets.
func rustResolveQualifiedCall(registry symbolRegistry, module, callee string, imports map[string]string) symbolResolution {
	if !strings.Contains(callee, "::") {
		return symbolResolution{}
	}
	path := strings.ReplaceAll(rustStripTurbofish(callee), "::", ".")
	for _, candidate := range []string{path, joinSyntaxScope(module, path)} {
		if target, ok := registry.exact[candidate]; ok && (target.Label == "Function" || target.Label == "Method") {
			parent := packagePrefix(candidate)
			if owner, ok := registry.exact[parent]; ok && owner.Label == "Interface" {
				return symbolResolution{}
			}
			strategy := "lsp_direct"
			if target.Label == "Method" {
				strategy = "lsp_ufcs"
				if strings.HasSuffix(candidate, ".new") {
					strategy = "lsp_constructor"
				}
			}
			return symbolResolution{qn: target.QualifiedName, strategy: strategy, confidence: .95, candidates: 1}
		}
	}
	if head, tail, ok := strings.Cut(path, "."); ok {
		if imported := imports[head]; imported != "" {
			candidate := imported + "." + tail
			if target, ok := registry.exact[candidate]; ok && (target.Label == "Function" || target.Label == "Method") {
				return symbolResolution{qn: target.QualifiedName, strategy: "lsp_direct", confidence: .95, candidates: 1}
			}
		}
	}
	return symbolResolution{}
}

func rustStripTurbofish(path string) string {
	var out strings.Builder
	for i := 0; i < len(path); {
		if strings.HasPrefix(path[i:], "::<") {
			depth := 1
			j := i + 3
			for ; j < len(path) && depth > 0; j++ {
				switch path[j] {
				case '<':
					depth++
				case '>':
					depth--
				}
			}
			if depth == 0 {
				i = j
				continue
			}
		}
		out.WriteByte(path[i])
		i++
	}
	return out.String()
}
