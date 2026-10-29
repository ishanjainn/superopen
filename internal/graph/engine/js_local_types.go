package engine

import "strings"

// A local constructor binding gives member calls an exact receiver type.
type jsLocalTypes map[string]map[string][]SyntaxFact

func newJSLocalTypes(parsed *ParsedSyntaxFile) jsLocalTypes {
	out := jsLocalTypes{}
	if !isJSLanguage(parsed.File.Language) {
		return out
	}
	for _, fact := range parsed.Extraction.TypeAssignments {
		if fact.Name == "" || fact.ReturnType == "" {
			continue
		}
		if out[fact.Scope] == nil {
			out[fact.Scope] = map[string][]SyntaxFact{}
		}
		out[fact.Scope][fact.Name] = append(out[fact.Scope][fact.Name], fact)
	}
	return out
}

func (types jsLocalTypes) resolve(registry symbolRegistry, session *resolveSession, fact SyntaxFact) (symbolResolution, bool) {
	receiver, member, dotted := strings.Cut(fact.Name, ".")
	if !dotted || receiver == "" || member == "" || strings.Contains(member, ".") {
		return symbolResolution{}, false
	}
	for scope := fact.Scope; ; {
		bindings := types[scope][receiver]
		for i := len(bindings) - 1; i >= 0; i-- {
			binding := bindings[i]
			if binding.EndByte > fact.StartByte {
				continue
			}
			constructor := registry.resolveWith(session, binding.ReturnType)
			class, ok := registry.exact[constructor.qn]
			if !ok || class.Label != "Class" {
				return symbolResolution{}, true
			}
			if target, ok := registry.exact[class.QualifiedName+"."+member]; ok && target.Label == "Method" {
				return symbolResolution{qn: target.QualifiedName, strategy: "lsp_ts_method", confidence: .9, candidates: 1}, true
			}
			return symbolResolution{}, true
		}
		cut := strings.LastIndexByte(scope, '.')
		if cut < 0 {
			if scope == "" {
				break
			}
			scope = ""
		} else {
			scope = scope[:cut]
		}
	}
	return symbolResolution{}, false
}
