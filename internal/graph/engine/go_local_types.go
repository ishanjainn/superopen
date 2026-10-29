package engine

import "strings"

// Go selector calls need the declared receiver type. The Go call pass uses
// typed receiver evidence before its weak-name suppression.
type goLocalTypes struct {
	byFunction map[string]map[string][]goTypedBinding
	module     string
}

type goTypedBinding struct {
	endByte  uint32
	typeName string
}

func newGoLocalTypes(parsed *ParsedSyntaxFile, registry symbolRegistry, imports map[string]string) goLocalTypes {
	result := goLocalTypes{byFunction: map[string]map[string][]goTypedBinding{}, module: syntaxDefinitionModuleQN("go", parsed.File.Path)}
	if parsed.File.Language != "go" {
		return result
	}
	for _, binding := range parsed.Extraction.TypeAssignments {
		if binding.Name == "" || binding.ReturnType == "" {
			continue
		}
		owner, bestSpan := "", ^uint32(0)
		for _, definition := range parsed.Extraction.Definitions {
			if definition.Kind != "function" || definition.StartByte > binding.StartByte || binding.EndByte > definition.EndByte {
				continue
			}
			if span := definition.EndByte - definition.StartByte; span < bestSpan {
				owner, bestSpan = syntaxDefinitionFactQN("go", parsed.File.Path, definition), span
			}
		}
		if owner == "" {
			continue
		}
		typeName := binding.ReturnType
		if binding.TypeKind == "call_return" {
			resolved := registry.resolveWith(newResolveSession(result.module, imports), binding.ReturnType)
			if target, ok := registry.exact[resolved.qn]; ok {
				if returned, ok := target.Properties["return_type"].(string); ok {
					typeName = cleanSyntaxType(returned)
				}
			}
			if typeName == binding.ReturnType {
				continue
			}
		}
		if result.byFunction[owner] == nil {
			result.byFunction[owner] = map[string][]goTypedBinding{}
		}
		result.byFunction[owner][binding.Name] = append(result.byFunction[owner][binding.Name], goTypedBinding{endByte: binding.EndByte, typeName: typeName})
	}
	return result
}

func (types goLocalTypes) resolve(registry symbolRegistry, source, callee string, imports map[string]string, before uint32) symbolResolution {
	parts := strings.Split(callee, ".")
	if len(parts) < 2 || len(parts) > 5 {
		return symbolResolution{}
	}
	receiver, method := parts[0], parts[len(parts)-1]
	typeName := ""
	for _, binding := range types.byFunction[source][receiver] {
		if binding.endByte <= before {
			typeName = binding.typeName
		}
	}
	if typeName == "" && strings.HasSuffix(receiver, "{}") {
		typeName = strings.TrimSuffix(receiver, "{}")
	}
	if typeName == "" {
		if candidate := goTypeOwner(types.module, receiver, imports); goRegisteredType(registry.exact[candidate].Label) {
			typeName = receiver
		} else if fn, ok := registry.exact[candidate]; ok && fn.Label == "Function" {
			if returned, ok := fn.Properties["return_type"].(string); ok {
				typeName = cleanSyntaxType(returned)
			}
		}
	}
	if typeName == "" {
		return symbolResolution{}
	}
	owner := goTypeOwner(types.module, typeName, imports)
	for _, fieldName := range parts[1 : len(parts)-1] {
		field, ok := registry.exact[owner+"."+fieldName]
		if !ok || field.Label != "Field" {
			return symbolResolution{}
		}
		fieldType, _ := field.Properties["return_type"].(string)
		if fieldType == "" {
			return symbolResolution{}
		}
		owner = goTypeOwner(types.module, fieldType, imports)
	}
	if target, ok := registry.exact[owner+"."+method]; ok && target.Label == "Method" {
		return symbolResolution{qn: target.QualifiedName, strategy: "lsp_go_method", confidence: .95, candidates: 1}
	}
	return symbolResolution{}
}

func goTypeOwner(module, typeName string, imports map[string]string) string {
	typeName = cleanSyntaxType(typeName)
	if prefix, member, qualified := strings.Cut(typeName, "."); qualified && imports[prefix] != "" {
		return imports[prefix] + "." + member
	}
	return joinSyntaxScope(module, typeName)
}

func goRegisteredType(label string) bool {
	switch label {
	case "Struct", "Interface", "Class", "Type":
		return true
	}
	return false
}
