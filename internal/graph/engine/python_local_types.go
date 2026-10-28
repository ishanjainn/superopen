package engine

import "strings"

// pythonLocalTypes tracks simple assignments within one function. A binding
// becomes usable only after the assignment finishes, and a later assignment
// without a constructor invalidates the earlier type.
type pythonLocalTypes struct {
	moduleQN     string
	session      *resolveSession
	byScope      map[string]map[string][]pythonLocalType
	fields       map[string]map[string]pythonLocalType
	globalFields map[string]map[string]pythonLocalType
}

// Fold instance fields from every class before resolving cross-file
// calls. Build the same class-field surface once for the repository.
func collectPythonClassFields(files []ParsedSyntaxFile, registry symbolRegistry, importsByFile map[string]map[string]string) map[string]map[string]pythonLocalType {
	all := make(map[string]map[string]pythonLocalType)
	for i := range files {
		parsed := &files[i]
		if parsed.File.Language != "python" {
			continue
		}
		session := newResolveSession(syntaxDefinitionModuleQN("python", parsed.File.Path), importsByFile[parsed.File.Path])
		for class, fields := range newPythonLocalTypes(parsed, registry, session).fields {
			if all[class] == nil {
				all[class] = make(map[string]pythonLocalType)
			}
			for name, binding := range fields {
				all[class][name] = binding
			}
		}
	}
	return all
}

type pythonLocalType struct {
	endByte    uint32
	classQN    string
	elementQN  string
	callableQN string
	bases      []string
}

func newPythonLocalTypes(parsed *ParsedSyntaxFile, registry symbolRegistry, session *resolveSession) pythonLocalTypes {
	result := pythonLocalTypes{
		moduleQN: session.module,
		session:  session,
		byScope:  make(map[string]map[string][]pythonLocalType),
		fields:   make(map[string]map[string]pythonLocalType),
	}
	if parsed.File.Language != "python" {
		return result
	}
	for _, fact := range parsed.Extraction.TypeAssignments {
		if fact.Name == "" {
			continue
		}
		binding := pythonLocalType{endByte: fact.EndByte}
		if fact.TypeKind == "list_literal" {
			binding.classQN = "builtins.list"
		} else if fact.TypeKind == "dict_literal" {
			binding.classQN = "builtins.dict"
		} else if fact.TypeKind == "set_literal" {
			binding.classQN = "builtins.set"
		} else if fact.TypeKind == "iter_element" || fact.TypeKind == "index_element" {
			iterable := result.iterableValue(fact.Scope, fact.ValueName, fact.StartByte)
			binding.classQN = iterable.elementQN
			binding.bases = pythonClassBases(registry, session, binding.classQN)
		} else if fact.ReturnType != "" {
			if fact.TypeKind == "annotation" {
				binding.classQN = pythonAnnotatedClass(registry, session, fact.Scope, fact.ReturnType)
				if strings.HasPrefix(strings.TrimSpace(fact.ReturnType), "list[") {
					binding.classQN = "builtins.list"
				}
				binding.elementQN = pythonAnnotatedElement(registry, session, fact.Scope, fact.ReturnType)
			} else {
				binding.classQN = pythonConstructorClass(registry, session, fact.Scope, fact.ReturnType)
				if binding.classQN == "" && fact.ReturnType == "cls" {
					binding.classQN = registry.enclosingClass(result.moduleQN + "." + fact.Scope)
				}
			}
			binding.bases = pythonClassBases(registry, session, binding.classQN)
		} else if fact.ValueName != "" {
			binding = result.typeValue(registry, fact.Scope, fact.ValueName, fact.StartByte)
			binding.endByte = fact.EndByte
		}
		if owner, member, dotted := strings.Cut(fact.Name, "."); dotted && (owner == "self" || owner == "cls") {
			lastDot := strings.LastIndexByte(fact.Scope, '.')
			if lastDot < 0 {
				continue
			}
			method := fact.Scope[lastDot+1:]
			if method != "__init__" && method != "setUp" && method != "setUpClass" {
				continue
			}
			classScope := fact.Scope[:lastDot]
			classQN := result.moduleQN + "." + classScope
			if node, ok := registry.exact[classQN]; !ok || node.Label != "Class" {
				continue
			}
			if result.fields[classQN] == nil {
				result.fields[classQN] = make(map[string]pythonLocalType)
			}
			previous, exists := result.fields[classQN][member]
			if exists && previous.classQN != "" && binding.classQN != "" && previous.classQN != binding.classQN {
				binding.classQN, binding.bases = "", nil
			}
			result.fields[classQN][member] = binding
			continue
		}
		if result.byScope[fact.Scope] == nil {
			result.byScope[fact.Scope] = make(map[string][]pythonLocalType)
		}
		result.byScope[fact.Scope][fact.Name] = append(result.byScope[fact.Scope][fact.Name], binding)
	}
	return result
}

func pythonAnnotatedClass(registry symbolRegistry, session *resolveSession, scope, annotation string) string {
	annotation = strings.TrimSpace(strings.Trim(annotation, "'\""))
	for _, wrapper := range []string{"Optional[", "typing.Optional["} {
		if strings.HasPrefix(annotation, wrapper) && strings.HasSuffix(annotation, "]") {
			annotation = strings.TrimSpace(annotation[len(wrapper) : len(annotation)-1])
			break
		}
	}
	if cut := strings.IndexByte(annotation, '|'); cut >= 0 {
		annotation = strings.TrimSpace(annotation[:cut])
	}
	if cut := strings.IndexByte(annotation, '['); cut >= 0 {
		annotation = strings.TrimSpace(annotation[:cut])
	}
	return pythonConstructorClass(registry, session, scope, annotation)
}

func pythonAnnotatedElement(registry symbolRegistry, session *resolveSession, scope, annotation string) string {
	annotation = strings.TrimSpace(annotation)
	open := strings.IndexByte(annotation, '[')
	if open < 0 || !strings.HasSuffix(annotation, "]") {
		return ""
	}
	container := strings.TrimSpace(annotation[:open])
	switch container {
	case "list", "List", "tuple", "Tuple", "Sequence", "Iterable", "Iterator", "Collection":
	default:
		return ""
	}
	element := strings.TrimSpace(strings.SplitN(annotation[open+1:len(annotation)-1], ",", 2)[0])
	return pythonAnnotatedClass(registry, session, scope, element)
}

func (types pythonLocalTypes) iterableValue(scope, name string, before uint32) pythonLocalType {
	if cut := strings.IndexByte(name, '['); cut >= 0 && strings.HasSuffix(name, "]") {
		name = name[:cut]
	}
	if owner, member, dotted := strings.Cut(name, "."); dotted && (owner == "self" || owner == "cls") {
		if cut := strings.LastIndexByte(scope, '.'); cut >= 0 {
			return types.fields[types.moduleQN+"."+scope[:cut]][member]
		}
	}
	for current := scope; current != ""; {
		for i := len(types.byScope[current][name]) - 1; i >= 0; i-- {
			binding := types.byScope[current][name][i]
			if binding.endByte <= before {
				return binding
			}
		}
		cut := strings.LastIndexByte(current, '.')
		if cut < 0 {
			break
		}
		current = current[:cut]
	}
	return pythonLocalType{}
}

func (types pythonLocalTypes) typeValue(registry symbolRegistry, scope, name string, before uint32) pythonLocalType {
	for current := scope; current != ""; {
		for i := len(types.byScope[current][name]) - 1; i >= 0; i-- {
			binding := types.byScope[current][name][i]
			if binding.endByte <= before {
				return binding
			}
		}
		cut := strings.LastIndexByte(current, '.')
		if cut < 0 {
			break
		}
		current = current[:cut]
	}
	for i := len(types.byScope[""][name]) - 1; i >= 0; i-- {
		binding := types.byScope[""][name][i]
		if binding.endByte <= before {
			return binding
		}
	}
	return pythonLocalType{callableQN: types.callableValue(registry, scope, name, before)}
}

// Field lookup walks base classes as well as the defining class.
func (types pythonLocalTypes) field(registry symbolRegistry, classQN, member string) (pythonLocalType, bool) {
	classes := []string{classQN}
	if order, ok := pythonC3Order(registry, types.session, classQN, map[string]bool{}); ok {
		classes = order
	}
	for _, class := range classes {
		if field, ok := types.fields[class][member]; ok {
			return field, true
		}
		if field, ok := types.globalFields[class][member]; ok {
			return field, true
		}
	}
	return pythonLocalType{}, false
}

func (types pythonLocalTypes) callableValue(registry symbolRegistry, scope, name string, before uint32) string {
	for current := scope; current != ""; {
		for i := len(types.byScope[current][name]) - 1; i >= 0; i-- {
			binding := types.byScope[current][name][i]
			if binding.endByte <= before {
				if binding.callableQN != "" {
					return binding.callableQN
				}
				return ""
			}
		}
		if node, ok := registry.exact[types.moduleQN+"."+current+"."+name]; ok && jsxCallableLabel(node.Label) {
			return node.QualifiedName
		}
		cut := strings.LastIndexByte(current, '.')
		if cut < 0 {
			break
		}
		current = current[:cut]
	}
	for i := len(types.byScope[""][name]) - 1; i >= 0; i-- {
		binding := types.byScope[""][name][i]
		if binding.endByte <= before {
			return binding.callableQN
		}
	}
	resolved := registry.resolveWith(types.session, name)
	if resolved.strategy == "same_module" || resolved.strategy == "import_map" || resolved.strategy == "import_map_suffix" {
		if node, ok := registry.exact[resolved.qn]; ok && jsxCallableLabel(node.Label) {
			return node.QualifiedName
		}
	}
	return ""
}

func pythonConstructorClass(registry symbolRegistry, session *resolveSession, scope, callee string) string {
	if !strings.Contains(callee, ".") {
		if node, ok := registry.exact[session.module+"."+scope+"."+callee]; ok && node.Label == "Class" {
			return node.QualifiedName
		}
	}
	resolved := registry.resolveWith(session, callee)
	switch resolved.strategy {
	case "import_map", "import_map_suffix", "same_module":
		if node, ok := registry.exact[resolved.qn]; ok && node.Label == "Class" {
			return resolved.qn
		}
	}
	switch callee {
	case "list", "dict", "set":
		return "builtins." + callee
	}
	return ""
}

func pythonClassBases(registry symbolRegistry, session *resolveSession, classQN string) []string {
	node, ok := registry.exact[classQN]
	if !ok {
		return nil
	}
	names, ok := node.Properties["base_classes"].([]string)
	if !ok {
		return nil
	}
	baseSession := session
	if owner := syntaxDefinitionModuleQN("python", node.Location.File); owner != session.module {
		baseSession = newResolveSession(owner, nil)
	}
	var bases []string
	for _, name := range names {
		resolved := registry.resolveWith(baseSession, name)
		if base, ok := registry.exact[resolved.qn]; ok && base.Label == "Class" &&
			(resolved.strategy == "import_map" || resolved.strategy == "import_map_suffix" || resolved.strategy == "same_module") {
			bases = append(bases, base.QualifiedName)
			continue
		}
		if candidates := registry.byName[syntaxCallBase(name)]; len(candidates) == 1 && candidates[0].Label == "Class" {
			bases = append(bases, candidates[0].QualifiedName)
		}
	}
	return bases
}

func (types pythonLocalTypes) resolve(registry symbolRegistry, fact SyntaxFact) (symbolResolution, bool) {
	if !strings.Contains(fact.Name, ".") {
		for scope := fact.Scope; scope != ""; {
			if bindings := types.byScope[scope][fact.Name]; len(bindings) > 0 {
				for i := len(bindings) - 1; i >= 0; i-- {
					if bindings[i].endByte <= fact.StartByte {
						if bindings[i].callableQN != "" {
							return symbolResolution{qn: bindings[i].callableQN, strategy: "lsp_callable_alias", confidence: .97, candidates: 1}, true
						}
						return symbolResolution{}, true
					}
				}
			}
			cut := strings.LastIndexByte(scope, '.')
			if cut < 0 {
				break
			}
			scope = scope[:cut]
		}
		for i := len(types.byScope[""][fact.Name]) - 1; i >= 0; i-- {
			if types.byScope[""][fact.Name][i].endByte <= fact.StartByte {
				binding := types.byScope[""][fact.Name][i]
				if binding.callableQN != "" {
					return symbolResolution{qn: binding.callableQN, strategy: "lsp_callable_alias", confidence: .97, candidates: 1}, true
				}
				return symbolResolution{}, true
			}
		}
	}
	if callEnd := strings.LastIndex(fact.Name, ")."); callEnd > 0 {
		if open := strings.IndexByte(fact.Name, '('); open > 0 && open < callEnd {
			constructor := fact.Name[:open]
			member := fact.Name[callEnd+2:]
			if member != "" && !strings.ContainsAny(member, ".:()") {
				classQN := pythonConstructorClass(registry, types.session, fact.Scope, constructor)
				if classQN != "" {
					binding := pythonLocalType{classQN: classQN, bases: pythonClassBases(registry, types.session, classQN)}
					return pythonBoundMember(registry, types.session, binding, member), true
				}
				// Evaluate the inner call's return annotation before
				// resolving a chained method such as token().match().
				inner := fact
				inner.Name = constructor
				resolved, _ := types.resolve(registry, inner)
				if target, ok := registry.exact[resolved.qn]; ok {
					if annotation, ok := target.Properties["return_type"].(string); ok {
						classQN = pythonAnnotatedClass(registry, types.session, fact.Scope, annotation)
						if classQN != "" {
							return pythonBoundMember(registry, types.session, pythonLocalType{classQN: classQN}, member), true
						}
					}
				}
			}
		}
	}
	if strings.HasPrefix(fact.Name, "super().") && strings.Count(fact.Name, ".") == 1 {
		lastDot := strings.LastIndexByte(fact.Scope, '.')
		if lastDot >= 0 {
			classQN := types.moduleQN + "." + fact.Scope[:lastDot]
			member := strings.TrimPrefix(fact.Name, "super().")
			if order, ok := pythonC3Order(registry, types.session, classQN, map[string]bool{}); ok {
				for _, base := range order[1:] {
					if node, exists := registry.exact[base+"."+member]; exists && node.Label == "Method" {
						strategy, confidence := "lsp_super", .88
						if member == "__init__" {
							strategy, confidence = "lsp_super_init", .90
						}
						return symbolResolution{qn: node.QualifiedName, strategy: strategy, confidence: confidence, candidates: 1}, true
					}
				}
			}
			seen := map[string]bool{}
			var search func(string) string
			search = func(qn string) string {
				for _, base := range pythonClassBases(registry, types.session, qn) {
					if seen[base] {
						continue
					}
					seen[base] = true
					if node, ok := registry.exact[base+"."+member]; ok && node.Label == "Method" {
						return node.QualifiedName
					}
					if inherited := search(base); inherited != "" {
						return inherited
					}
				}
				return ""
			}
			if qn := search(classQN); qn != "" {
				strategy, confidence := "lsp_super", .88
				if member == "__init__" {
					strategy, confidence = "lsp_super_init", .90
				}
				return symbolResolution{qn: qn, strategy: strategy, confidence: confidence, candidates: 1}, true
			}
		}
	}
	parts := strings.Split(fact.Name, ".")
	if len(parts) > 3 && len(parts) <= 6 {
		binding := pythonLocalType{}
		if parts[0] == "self" || parts[0] == "cls" {
			binding.classQN = registry.enclosingClass(types.moduleQN + "." + fact.Scope)
		} else {
			binding = types.typeValue(registry, fact.Scope, parts[0], fact.StartByte)
		}
		for _, fieldName := range parts[1 : len(parts)-1] {
			if binding.classQN == "" {
				return symbolResolution{}, false
			}
			name, indexed := pythonIndexedSegment(fieldName)
			field, ok := types.field(registry, binding.classQN, name)
			if !ok {
				return symbolResolution{}, false
			}
			binding = field
			if indexed {
				binding.classQN = binding.elementQN
			}
		}
		return pythonBoundMember(registry, types.session, binding, parts[len(parts)-1]), true
	}
	if len(parts) != 2 && len(parts) != 3 {
		return symbolResolution{}, false
	}
	if len(parts) == 2 && (parts[0] == "self" || parts[0] == "cls") {
		if classQN := registry.enclosingClass(types.moduleQN + "." + fact.Scope); classQN != "" {
			if resolved := pythonBoundMember(registry, types.session, pythonLocalType{classQN: classQN}, parts[1]); resolved.qn != "" {
				resolved.strategy = "own_member"
				return resolved, true
			}
		}
	}
	if len(parts) == 3 {
		if parts[0] != "self" && parts[0] != "cls" {
			receiver := types.typeValue(registry, fact.Scope, parts[0], fact.StartByte)
			if receiver.classQN == "" {
				return symbolResolution{}, false
			}
			name, indexed := pythonIndexedSegment(parts[1])
			field, ok := types.field(registry, receiver.classQN, name)
			if !ok {
				return symbolResolution{}, false
			}
			if indexed {
				field.classQN = field.elementQN
			}
			return pythonBoundMember(registry, types.session, field, parts[2]), true
		}
		classQN := registry.enclosingClass(types.moduleQN + "." + fact.Scope)
		if classQN == "" {
			return symbolResolution{}, false
		}
		name, indexed := pythonIndexedSegment(parts[1])
		field, ok := types.field(registry, classQN, name)
		if !ok {
			return symbolResolution{}, false
		}
		if indexed {
			field.classQN = field.elementQN
		}
		return pythonBoundMember(registry, types.session, field, parts[2]), true
	}
	if name, indexed := pythonIndexedSegment(parts[0]); indexed {
		binding := types.typeValue(registry, fact.Scope, name, fact.StartByte)
		if binding.elementQN != "" {
			binding.classQN = binding.elementQN
			return pythonBoundMember(registry, types.session, binding, parts[1]), true
		}
	}
	if len(parts) == 2 {
		if classQN := pythonConstructorClass(registry, types.session, fact.Scope, parts[0]); classQN != "" {
			if resolved := pythonBoundMember(registry, types.session, pythonLocalType{classQN: classQN}, parts[1]); resolved.qn != "" {
				resolved.strategy = "lsp_class_method"
				return resolved, true
			}
		}
	}
	for scope := fact.Scope; scope != ""; {
		bindings := types.byScope[scope][parts[0]]
		for i := len(bindings) - 1; i >= 0; i-- {
			binding := bindings[i]
			if binding.endByte > fact.StartByte {
				continue
			}
			return pythonBoundMember(registry, types.session, binding, parts[1]), true
		}
		cut := strings.LastIndexByte(scope, '.')
		if cut < 0 {
			break
		}
		scope = scope[:cut]
	}
	for i := len(types.byScope[""][parts[0]]) - 1; i >= 0; i-- {
		binding := types.byScope[""][parts[0]][i]
		if binding.endByte <= fact.StartByte {
			return pythonBoundMember(registry, types.session, binding, parts[1]), true
		}
	}
	return symbolResolution{}, false
}

func pythonIndexedSegment(segment string) (string, bool) {
	if open := strings.IndexByte(segment, '['); open > 0 && strings.HasSuffix(segment, "]") {
		return segment[:open], true
	}
	return segment, false
}

func pythonC3Order(registry symbolRegistry, session *resolveSession, classQN string, active map[string]bool) ([]string, bool) {
	if active[classQN] || len(active) >= 32 {
		return nil, false
	}
	active[classQN] = true
	defer delete(active, classQN)
	bases := pythonClassBases(registry, session, classQN)
	sequences := make([][]string, 0, len(bases)+1)
	for _, base := range bases {
		order, ok := pythonC3Order(registry, session, base, active)
		if !ok {
			return nil, false
		}
		sequences = append(sequences, order)
	}
	sequences = append(sequences, append([]string(nil), bases...))
	result := []string{classQN}
	for {
		remaining := false
		candidate := ""
		for _, sequence := range sequences {
			if len(sequence) == 0 {
				continue
			}
			remaining = true
			head := sequence[0]
			inTail := false
			for _, other := range sequences {
				if len(other) == 0 {
					continue
				}
				for _, tail := range other[1:] {
					if tail == head {
						inTail = true
						break
					}
				}
				if inTail {
					break
				}
			}
			if !inTail {
				candidate = head
				break
			}
		}
		if !remaining {
			return result, true
		}
		if candidate == "" {
			return nil, false
		}
		result = append(result, candidate)
		for i := range sequences {
			if len(sequences[i]) > 0 && sequences[i][0] == candidate {
				sequences[i] = sequences[i][1:]
			}
		}
	}
}

func pythonBoundMember(registry symbolRegistry, session *resolveSession, binding pythonLocalType, method string) symbolResolution {
	classes := append([]string{binding.classQN}, binding.bases...)
	if binding.classQN != "" {
		if order, ok := pythonC3Order(registry, session, binding.classQN, map[string]bool{}); ok {
			classes = order
		}
	}
	for _, classQN := range classes {
		if classQN == "" {
			continue
		}
		qn := classQN + "." + method
		if node, ok := registry.exact[qn]; ok && node.Label == "Method" {
			return symbolResolution{qn: qn, strategy: "local_constructor", confidence: .98, candidates: 1}
		}
	}
	return symbolResolution{}
}
