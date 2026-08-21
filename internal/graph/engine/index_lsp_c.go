package engine

import (
	"context"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// enrichCResolvedCalls fills FileResult.ResolvedCalls for C/C++/CUDA using an
// in-process type registry (file defs + unique methods). This is not clangd.
func enrichCResolvedCalls(ctx context.Context, files []ParsedSyntaxFile) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	registry := newCTypeRegistry(files)
	for index := range files {
		parsed := &files[index]
		if !isCFamilyLanguage(parsed.File.Language) {
			continue
		}
		resolveCFileCalls(parsed, registry)
	}
	return nil
}

type cTypeRegistry struct {
	types         map[string]string   // short name -> unique QN
	methods       map[string][]string // typeQN or type short name -> method QNs
	methodsByName map[string][]string
	fields        map[string]string // typeQN.field -> field type short name
}

func newCTypeRegistry(files []ParsedSyntaxFile) *cTypeRegistry {
	reg := &cTypeRegistry{
		types:         map[string]string{},
		methods:       map[string][]string{},
		methodsByName: map[string][]string{},
		fields:        map[string]string{},
	}
	typeCounts := map[string]int{}
	typeQN := map[string]string{}
	for _, parsed := range files {
		if !isCFamilyLanguage(parsed.File.Language) {
			continue
		}
		rel := parsed.File.Path
		lang := parsed.File.Language
		for _, def := range parsed.Extraction.Definitions {
			qn := syntaxDefinitionFactQN(lang, rel, def)
			switch def.Kind {
			case "class":
				typeCounts[def.Name]++
				typeQN[def.Name] = qn
			case "function":
				owner := cMethodOwner(def)
				if owner != "" {
					reg.methods[owner] = appendUnique(reg.methods[owner], qn)
					reg.methodsByName[def.Name] = appendUnique(reg.methodsByName[def.Name], qn)
				}
				if len(def.ParamTypes) > 0 && def.ParamTypes[0] != "" {
					recv := def.ParamTypes[0]
					reg.methods[recv] = appendUnique(reg.methods[recv], qn)
					reg.methodsByName[def.Name] = appendUnique(reg.methodsByName[def.Name], qn)
				}
			case "field":
				owner := def.ParentClass
				if owner == "" {
					owner = def.Scope
				}
				if owner != "" && def.ReturnType != "" {
					reg.fields[owner+"."+def.Name] = cleanCTypeName(def.ReturnType)
				}
			}
		}
	}
	for name, count := range typeCounts {
		if count == 1 {
			reg.types[name] = typeQN[name]
		}
	}
	return reg
}

func cMethodOwner(def SyntaxFact) string {
	if def.ParentClass != "" {
		return def.ParentClass
	}
	if def.Scope != "" {
		return def.Scope
	}
	return ""
}

func resolveCFileCalls(parsed *ParsedSyntaxFile, registry *cTypeRegistry) {
	locals := cFileLocals(parsed)
	rel := parsed.File.Path
	for _, call := range parsed.Extraction.Calls {
		recv, meth, form := splitCCallee(call.Name)
		if form == "direct" || meth == "" {
			continue
		}
		source := syntaxScopeOwner(parsed, call.StartLine)
		if source == "" {
			source = fileQualifiedName(rel)
		}
		target, strategy := registry.resolveMember(recv, meth, form, locals)
		if target == "" || target == source {
			continue
		}
		parsed.Extraction.ResolvedCalls = append(parsed.Extraction.ResolvedCalls, ResolvedRelationship{
			Source: source, Target: target, Type: "CALLS", Strategy: strategy, Confidence: .9,
			Location:   syntaxLocation(rel, call),
			Properties: api.Properties{"callee": call.Name, "source_origin": call.SourceOrigin},
		})
	}
}

func (reg *cTypeRegistry) resolveMember(recv, meth, form string, locals map[string]string) (string, string) {
	if form == "scoped" {
		if qn := reg.lookupMember(recv, meth); qn != "" {
			return qn, "lsp_scoped"
		}
		return "", ""
	}
	typ := reg.typeOfReceiver(recv, locals)
	if typ != "" {
		if qn := reg.lookupMember(typ, meth); qn != "" {
			return qn, "lsp_type_dispatch"
		}
	}
	if qn := reg.lookupMember(recv, meth); qn != "" {
		return qn, "lsp_type_dispatch"
	}
	if names := reg.methodsByName[meth]; len(names) == 1 {
		return names[0], "lsp_type_dispatch"
	}
	return "", ""
}

func (reg *cTypeRegistry) typeOfReceiver(recv string, locals map[string]string) string {
	parts := splitCReceiverParts(recv)
	if len(parts) == 0 {
		return ""
	}
	typ := locals[parts[0]]
	for _, field := range parts[1:] {
		if typ == "" {
			return ""
		}
		next := reg.fields[typ+"."+field]
		if next == "" {
			if qn := reg.types[typ]; qn != "" {
				next = reg.fields[qn+"."+field]
			}
		}
		typ = next
	}
	return typ
}

func splitCReceiverParts(recv string) []string {
	recv = strings.TrimSpace(recv)
	if recv == "" {
		return nil
	}
	var parts []string
	start := 0
	for i := 0; i < len(recv); {
		if i+1 < len(recv) && recv[i] == '-' && recv[i+1] == '>' {
			if i > start {
				parts = append(parts, recv[start:i])
			}
			i += 2
			start = i
			continue
		}
		if recv[i] == '.' {
			if i > start {
				parts = append(parts, recv[start:i])
			}
			i++
			start = i
			continue
		}
		i++
	}
	if start < len(recv) {
		parts = append(parts, recv[start:])
	}
	return parts
}

func (reg *cTypeRegistry) lookupMember(typeName, method string) string {
	if typeName == "" || method == "" {
		return ""
	}
	short := typeName
	if index := strings.LastIndexAny(short, ".:"); index >= 0 {
		short = short[index+1:]
	}
	for _, key := range []string{typeName, short} {
		matches := []string{}
		for _, qn := range reg.methods[key] {
			if cQualifiedNameLeaf(qn) == method {
				matches = appendUnique(matches, qn)
			}
		}
		if len(matches) == 1 {
			return matches[0]
		}
	}
	if typeQN := reg.types[short]; typeQN != "" {
		matches := []string{}
		for _, qn := range reg.methods[typeQN] {
			if strings.HasSuffix(qn, "."+method) {
				matches = appendUnique(matches, qn)
			}
		}
		if len(matches) == 1 {
			return matches[0]
		}
	}
	return ""
}

func cFileLocals(parsed *ParsedSyntaxFile) map[string]string {
	locals := map[string]string{}
	for _, def := range parsed.Extraction.Definitions {
		if def.Kind != "function" {
			continue
		}
		n := len(def.ParamNames)
		if len(def.ParamTypes) < n {
			n = len(def.ParamTypes)
		}
		for i := 0; i < n; i++ {
			if def.ParamNames[i] != "" && def.ParamTypes[i] != "" {
				locals[def.ParamNames[i]] = cleanCTypeName(def.ParamTypes[i])
			}
		}
	}
	return locals
}

func splitCCallee(name string) (recv, meth, form string) {
	name = strings.TrimSpace(strings.Trim(name, "()"))
	if name == "" {
		return "", "", "direct"
	}
	if index := strings.LastIndex(name, "::"); index >= 0 {
		return name[:index], name[index+2:], "scoped"
	}
	if index := strings.LastIndex(name, "->"); index >= 0 {
		return name[:index], name[index+2:], "arrow"
	}
	if index := strings.LastIndexByte(name, '.'); index >= 0 {
		return name[:index], name[index+1:], "dot"
	}
	return "", name, "direct"
}

func cQualifiedNameLeaf(qn string) string {
	if index := strings.LastIndexByte(qn, '.'); index >= 0 {
		return qn[index+1:]
	}
	return qn
}

func coveredByResolvedCall(parsed ParsedSyntaxFile, fact SyntaxFact) bool {
	for _, relationship := range parsed.Extraction.ResolvedCalls {
		if relationship.Type != "CALLS" || relationship.Target == "" {
			continue
		}
		if relationship.Location.StartLine != fact.StartLine {
			continue
		}
		callee, _ := relationship.Properties["callee"].(string)
		if callee == fact.Name || callee == "" {
			return true
		}
	}
	return false
}

func appendUnique(values []string, next string) []string {
	for _, value := range values {
		if value == next {
			return values
		}
	}
	return append(values, next)
}
