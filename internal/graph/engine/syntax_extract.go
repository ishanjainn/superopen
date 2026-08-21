package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// SyntaxFact is an intermediate, source-grounded extraction result. Semantic
// resolvers consume these facts before graph IDs exist, which keeps parsing
// independent from storage and allows golden comparison pass by pass.
type SyntaxFact struct {
	Kind               string
	Name               string
	LocalName          string
	Scope              string
	NodeType           string
	StartByte          uint32
	EndByte            uint32
	StartLine          int
	StartColumn        int
	EndLine            int
	EndColumn          int
	Confidence         float64
	IsEntryPoint       bool
	IsExported         bool
	IsTest             bool
	Local              bool
	EnclosedByVariable bool
	VariableDepth      int
	StructuralProfile  string
	MinHash            string
	FirstStringArg     string
	Arguments          []SyntaxArgument
	Signature          string
	ReturnType         string
	ParentClass        string
	Docstring          string
	BodyTokens         string
	ParamNames         []string
	ParamTypes         []string
	BaseClasses        []string
	TypeKind           string
	Complexity         int
	Cognitive          int
	LoopCount          int
	LoopDepth          int
	MaxAccessDepth     int
	Lines              int
	MayBeCallReference bool
	SourceOrigin       string
}

// SyntaxArgument preserves the observable call-site information consumed by
// Superopen's service, configuration, data-flow, and argument-propagation
// passes. Values are unquoted string literals; Expr remains exact source text.
type SyntaxArgument struct {
	Expr    string
	Value   string
	Keyword string
	Index   int
	Literal bool
}

// FileResult is the single extraction boundary used by every language. It
// mirrors the pinned pipeline's per-file result rather than leaking parser or
// go/types objects into later passes. Relationship slices retain source order;
// resolvers add ResolvedCalls without mutating the grounded occurrences.
type FileResult struct {
	Definitions         []SyntaxFact
	Imports             []SyntaxFact
	Calls               []SyntaxFact
	Usages              []OccurrenceFact
	CallableReferences  []SyntaxFact
	Bindings            []OccurrenceFact
	Writes              []OccurrenceFact
	Throws              []SyntaxFact
	Decorators          []SyntaxFact
	Inheritance         []SyntaxFact
	TypeReferences      []SyntaxFact
	TypeAssignments     []SyntaxFact
	Implementations     []SyntaxFact
	Routes              []SyntaxFact
	Channels            []SyntaxFact
	EnvironmentAccesses []SyntaxFact
	ConfigurationRefs   []SyntaxFact
	StringReferences    []SyntaxFact
	KubernetesFacts     []SyntaxFact
	Sections            []SyntaxFact
	Branches            []SyntaxFact
	ResolvedCalls       []ResolvedRelationship
	ParseStatus         ParseStatus
	LSPSurface          LSPSurface
	RootModule          bool
	Partial             bool
}

type ResolvedRelationship struct {
	Source, Target, Type, Strategy, UnresolvedReason string
	Confidence                                       float64
	Ambiguous                                        bool
	Alternatives                                     []string
	Location                                         api.Location
	Properties                                       api.Properties
}

type ParseStatus struct {
	Parsed, Partial bool
	ErrorRanges     []api.Location
	Failure         string
}

type LSPSurface struct {
	Definitions []string
	References  []string
	Imports     map[string]string
	Config      string
}

// SyntaxExtraction is retained as a source-compatible alias while callers
// move to the unified name. It is not a second extraction representation.
type SyntaxExtraction = FileResult

// ExtractSyntaxFacts performs the language-neutral portion of Superopen's
// extraction pipeline using its generated language specification. Family
// resolvers enrich and resolve these grounded facts in later passes.
func ExtractSyntaxFacts(language string, root syntaxView, source []byte) (SyntaxExtraction, error) {
	spec, ok := PinnedLanguageSpec(language)
	if !ok {
		return SyntaxExtraction{}, fmt.Errorf("no pinned extraction spec for %s", language)
	}
	return extractSyntaxFactsWithSets(language, root, source, syntaxSpecSets(spec))
}

func extractSyntaxFactsWithSets(language string, root syntaxView, source []byte, sets specSets) (SyntaxExtraction, error) {
	lines := sourceLineIndex(source)
	result := FileResult{Partial: root.HasErr(), RootModule: sets.modules[root.Kind()], ParseStatus: ParseStatus{Parsed: true, Partial: root.HasErr()}}
	var walk func(syntaxView, syntaxView, []syntaxView, []string, bool, int, bool, bool, int, bool, bool)
	walk = func(node, parent syntaxView, ancestors []syntaxView, scope []string, local bool, variableDepth int, enclosedByVariable bool, exactCallee bool, importDepth int, inClass bool, inClassMethod bool) {
		if node.Kind() == "ERROR" {
			startLine, startColumn := bytePosition(lines, node.StartByte())
			endLine, endColumn := bytePosition(lines, node.EndByte())
			result.ParseStatus.ErrorRanges = append(result.ParseStatus.ErrorRanges, api.Location{
				StartLine: startLine, StartColumn: startColumn, EndLine: endLine, EndColumn: endColumn,
			})
		}
		currentScope := strings.Join(scope, ".")
		definitionKind := ""
		if language == "go" && (node.Kind() == "var_declaration" || node.Kind() == "const_declaration") {
			for _, fact := range syntaxGoDeclarationFacts(node, source, currentScope, lines, local) {
				result.Definitions = append(result.Definitions, fact)
			}
		}
		switch {
		case specHit(sets.functions, sets.functionIDs, node):
			definitionKind = "function"
		case specHit(sets.classes, sets.classIDs, node):
			definitionKind = "class"
		case specHit(sets.fields, sets.fieldIDs, node):
			definitionKind = "field"
		case specHit(sets.variables, sets.variableIDs, node):
			if language != "go" || (node.Kind() != "var_declaration" && node.Kind() != "const_declaration") {
				definitionKind = "variable"
			}
		case specHit(sets.modules, sets.moduleIDs, node) && node.StartByte() != root.StartByte():
			definitionKind = "module"
		}
		promotedName := ""
		if definitionKind == "variable" && isJSLanguage(language) {
			promotedName = functionVariableName(node, source)
			if promotedName != "" {
				definitionKind = "function"
			}
		}
		childScope := scope
		definitionEstablished := false
		// Superopen extract_variables only walks module-level JS/TS declarators
		// (direct program children, optionally wrapped by export_statement).
		// Nested bindings inside declare module / functions stay unpublished.
		if definitionKind == "variable" && isJSLanguage(language) &&
			(node.Kind() == "lexical_declaration" || node.Kind() == "variable_declaration") {
			if isJSModuleLevelVariable(ancestors) {
				for _, fact := range extractJSLexicalVariables(node, source, currentScope, lines, local, variableDepth, enclosedByVariable) {
					result.Definitions = append(result.Definitions, fact)
				}
			}
		} else if definitionKind != "" {
			name := promotedName
			if name == "" {
				name = definitionName(language, node, source, definitionKind)
			}
			if name == "" && definitionKind == "function" && node.Kind() == "arrow_function" && parent.Kind() == "pair" {
				if key, ok := findField(parent, "key"); ok {
					name = strings.TrimSpace(nodeText(key, source))
				}
			}
			if name == "" && definitionKind == "function" && isJSLanguage(language) &&
				(parent.Kind() == "public_field_definition" || parent.Kind() == "field_definition") {
				if key, ok := findField(parent, "name"); ok {
					name = strings.TrimSpace(nodeText(key, source))
				} else if key, ok := findField(parent, "property"); ok {
					name = strings.TrimSpace(nodeText(key, source))
				}
			}
			// Superopen extract_class_methods publishes class members only and does
			// not descend into method bodies, so nested const/function arrows
			// inside constructors/methods must not become Function nodes.
			// Non-member callables inside a class body (e.g. object-literal
			// arrows in a field initializer) are also unpublished: walk_defs
			// routes class bodies through extract_class_methods only.
			if name != "" && isJSLanguage(language) && definitionKind == "function" && inClass &&
				(inClassMethod || !jsClassMethodNode(node, parent)) {
				name = ""
			}
			if name != "" {
				definitionEstablished = true
				// JS/TS callables are module-flat unless they are class members.
				// Superopen compute_func_qn only qualifies under enclosing_class_qn.
				publishScope := currentScope
				scopeForChildren := scope
				if isJSLanguage(language) && definitionKind == "function" && !inClass {
					publishScope = ""
					scopeForChildren = nil
				}
				fact := syntaxFact(definitionKind, name, publishScope, node, lines, 1)
				fact.Local = local || (language == "bash" && definitionKind == "variable" && parent.Kind() != root.Kind())
				fact.EnclosedByVariable = enclosedByVariable
				fact.VariableDepth = variableDepth
				// Superopen derives export visibility from the name for every
				// definition kind, not only functions.
				fact.IsExported = pinnedIsExported(name, language)
				if definitionKind == "function" {
					fact.IsEntryPoint = name == "main"
					body := node
					if value, ok := findField(node, "body"); ok {
						body = value
					}
					parameters := []string{}
					if value, ok := findField(node, "parameters"); ok {
						fact.Signature = strings.TrimSpace(nodeText(value, source))
						parameters = syntaxParameterNames(value, source)
						fact.ParamTypes = syntaxParameterTypes(value, source)
					}
					fact.ParamNames = parameters
					fillCFamilyFunctionParams(language, node, source, &fact)
					for _, field := range []string{"result", "return_type", "type"} {
						if value, ok := findField(node, field); ok {
							fact.ReturnType = strings.TrimSpace(nodeText(value, source))
							break
						}
					}
					if language == "go" && node.Kind() == "method_declaration" {
						if receiver, ok := findField(node, "receiver"); ok {
							fact.ParentClass = syntaxGoReceiverType(receiver, source)
						}
					}
					metrics := syntaxComplexity(node, sets.branches)
					fact.Complexity = metrics.cyclomatic
					fact.Cognitive = metrics.cognitive
					fact.LoopCount = metrics.loopCount
					fact.LoopDepth = metrics.loopDepth
					fact.MaxAccessDepth = metrics.maxAccessDepth
					fillFunctionBodyFacts(&fact, body, source, parameters)
				}
				if definitionKind == "field" {
					if value, ok := findField(node, "type"); ok {
						fact.ReturnType = strings.TrimSpace(nodeText(value, source))
					}
				}
				if definitionKind == "class" {
					fact.BaseClasses = syntaxBaseClasses(language, node, source)
					// Inheritance edges are owned by the class/interface itself,
					// matching Superopen's use of def->qualified_name as source.
					classScope := joinSyntaxScope(currentScope, name)
					for _, base := range fact.BaseClasses {
						baseFact := syntaxFact("inheritance", base, classScope, node, lines, 1)
						baseFact.TypeKind = fact.TypeKind
						result.Inheritance = append(result.Inheritance, baseFact)
					}
					if language == "go" && node.Kind() == "type_spec" {
						if value, ok := findField(node, "type"); ok {
							switch value.Kind() {
							case "interface_type":
								fact.TypeKind = "interface"
							case "struct_type":
								fact.TypeKind = "struct"
							}
						}
					}
				}
				result.Definitions = append(result.Definitions, fact)
				if definitionKind == "class" && isEnumDeclarationNode(node.Kind()) {
					memberScope := joinSyntaxScope(currentScope, name)
					for _, member := range extractEnumMemberFacts(node, source, memberScope, lines) {
						result.Definitions = append(result.Definitions, member)
					}
				}
				if language == "yaml" && definitionKind == "variable" {
					if route := yamlInfraURLRoute(node, source, name, lines); route.Name != "" {
						result.Routes = append(result.Routes, route)
					}
				}
				// INI sections are Class nodes but Superopen does not push a
				// lexical scope for them; settings stay module-flat.
				if definitionKind == "function" || definitionKind == "module" ||
					(definitionKind == "class" && !(language == "ini" && node.Kind() == "section")) {
					childScope = append(append([]string(nil), scopeForChildren...), name)
				}
			}
		}
		if specHit(sets.calls, sets.callIDs, node) {
			if name := relationshipName(node, source, []string{"function", "callee", "name", "method"}); name != "" {
				base := name
				if index := strings.LastIndexAny(base, ".:"); index >= 0 {
					base = base[index+1:]
				}
				if !isLanguageKeyword(language, base) || isResolvableBuiltin(language, base) {
					fact := syntaxFact("call", name, currentScope, node, lines, .8)
					fact.Arguments, fact.FirstStringArg = syntaxCallArguments(node, source)
					result.Calls = append(result.Calls, fact)
				}
			}
		}
		// Superopen extract_jsx_component_ref: uppercase JSX tags are CALL sites
		// that require import/same-module resolution (LSP strategies).
		if isJSLanguage(language) &&
			(node.Kind() == "jsx_self_closing_element" || node.Kind() == "jsx_opening_element") {
			if nameNode, ok := findField(node, "name"); ok {
				name := strings.TrimSpace(nodeText(nameNode, source))
				if name != "" && name[0] >= 'A' && name[0] <= 'Z' {
					result.Calls = append(result.Calls, syntaxFact("call", name, currentScope, node, lines, .95))
				}
			}
		}
		if isSyntaxReferenceNode(language, node, parent, ancestors) && !exactCallee && importDepth == 0 &&
			!syntaxForwardSiblingCallee(language, node, parent) &&
			!syntaxCallArgumentLabel(node, ancestors) {
			if name := syntaxReferenceName(language, node, source); name != "" &&
				(!isLanguageKeyword(language, syntaxCallBase(name)) || isResolvableBuiltin(language, syntaxCallBase(name))) {
				switch {
				// Declarations, parameters, and assignment targets introduce a
				// lexical binding. Superopen records the binding and emits no
				// reference edge for the occurrence itself; later references to a
				// bound name are then suppressed as locally shadowed.
				case syntaxBindingOccurrence(language, node, ancestors, sets),
					syntaxWriteOccurrence(language, node, ancestors, sets):
					result.Bindings = append(result.Bindings, occurrenceFact(name, currentScope, node, lines, .9))
				default:
					fact := occurrenceFact(name, currentScope, node, lines, .7)
					// Superopen stamps may_be_call_reference for direct argument
					// values so the usages pass can emit CALL_REFERENCE when the
					// target is a proven callable.
					fact.MayBeCallReference = syntaxDirectArgumentValue(node, ancestors)
					result.Usages = append(result.Usages, fact)
				}
			}
		}
		if (sets.imports[node.Kind()] || sets.importsFrom[node.Kind()]) && (language != "bash" || parent.Kind() == root.Kind()) &&
			syntaxEmitsImportFacts(language, node) {
			imports := syntaxImportFacts(language, node, source, currentScope, lines)
			if len(imports) > 0 {
				result.Imports = append(result.Imports, imports...)
			} else if name := relationshipName(node, source, []string{"source", "path", "module", "name"}); name != "" {
				fact := syntaxFact("import", trimSyntaxLiteral(name), currentScope, node, lines, 1)
				fact.LocalName = importPathLast(fact.Name)
				result.Imports = append(result.Imports, fact)
			}
		}
		// CommonJS require("...") is a call_expression, not an import_statement.
		if isJSLanguage(language) && node.Kind() == "call_expression" {
			if fact, ok := syntaxCommonJSRequireFact(node, parent, source, currentScope, lines); ok {
				result.Imports = append(result.Imports, fact)
			}
		}
		if sets.assignments[node.Kind()] {
			if name := syntaxAssignmentWriteName(language, node, source); name != "" && !isLanguageKeyword(language, name) {
				result.Writes = append(result.Writes, occurrenceFact(name, currentScope, node, lines, 1))
			}
		}
		if sets.branches[node.Kind()] {
			result.Branches = append(result.Branches, syntaxFact("branch", node.Kind(), currentScope, node, lines, 1))
		}
		if sets.throws[node.Kind()] {
			if name := syntaxExceptionName(node, source); name != "" {
				result.Throws = append(result.Throws, syntaxFact("throw", name, currentScope, node, lines, 1))
			}
		}
		if language == "markdown" && node.Kind() == "atx_heading" {
			name := strings.TrimSpace(strings.TrimLeft(nodeText(node, source), "#"))
			if name != "" {
				result.Sections = append(result.Sections, syntaxFact("section", name, "", node, lines, 1))
			}
		}
		// Anonymous functions still introduce a local binding boundary. Class
		// wrappers such as Go's type_declaration do not: only a class with an
		// extracted name creates a nested definition scope.
		iniSection := language == "ini" && node.Kind() == "section"
		childLocal := local || definitionKind == "function" || definitionEstablished &&
			(definitionKind == "class" && !iniSection || isJSLanguage(language) && definitionKind == "variable")
		childVariableDepth := variableDepth
		if definitionKind == "variable" {
			childVariableDepth++
		}
		childImportDepth := importDepth
		if syntaxOpensImportContext(language, node, sets) {
			childImportDepth++
		}
		childInClass := inClass
		childInClassMethod := inClassMethod
		if isJSLanguage(language) {
			if definitionEstablished && definitionKind == "class" {
				childInClass = true
				childInClassMethod = false
			}
			if definitionEstablished && definitionKind == "function" && childInClass && !inClassMethod &&
				jsClassMethodNode(node, parent) {
				childInClassMethod = true
			}
		}
		childAncestors := append(ancestors, node)
		for i := 0; i < node.ChildCount(); i++ {
			child := node.ChildAt(i)
			childExactCallee := syntaxExactCalleeChild(node, child, exactCallee, specHit(sets.calls, sets.callIDs, node))
			walk(child, node, childAncestors, childScope, childLocal, childVariableDepth,
				enclosedByVariable || (isJSLanguage(language) && definitionKind == "variable"), childExactCallee, childImportDepth,
				childInClass, childInClassMethod)
		}
	}
	walk(root, SyntaxNode{}, nil, nil, false, 0, false, false, 0, false, false)
	attachSyntaxDocstrings(language, root, source, result.Definitions)
	collectSyntaxDecorators(root, source, sets.decorators, lines, result.Definitions, &result.Decorators)
	sortSyntaxFacts(&result)
	return result, nil
}

func attachSyntaxDocstrings(language string, root syntaxView, source []byte, definitions []SyntaxFact) {
	byStart := make(map[uint32][]int, len(definitions))
	for index := range definitions {
		byStart[definitions[index].StartByte] = append(byStart[definitions[index].StartByte], index)
	}
	var walk func(syntaxView)
	walk = func(parent syntaxView) {
		var previous syntaxView
		for i := 0; i < parent.ChildCount(); i++ {
			child := parent.ChildAt(i)
			if !viewMissing(previous) && isSyntaxComment(previous.Kind()) {
				if indexes := byStart[child.StartByte()]; len(indexes) > 0 {
					for _, index := range indexes {
						definitions[index].Docstring = truncateSyntaxComment(nodeText(previous, source), 500)
					}
				}
				if language == "go" && child.Kind() == "type_declaration" {
					attachGoTypeDocstring(child, truncateSyntaxComment(nodeText(previous, source), 500), byStart, definitions)
				}
			}
			walk(child)
			previous = child
		}
	}
	walk(root)
}

func attachGoTypeDocstring(node syntaxView, value string, byStart map[uint32][]int, definitions []SyntaxFact) {
	for i := 0; i < node.ChildCount(); i++ {
		child := node.ChildAt(i)
		if child.Kind() == "type_spec" || child.Kind() == "type_alias" {
			for _, index := range byStart[child.StartByte()] {
				definitions[index].Docstring = value
			}
		}
	}
}

func isSyntaxComment(kind string) bool {
	switch kind {
	case "comment", "block_comment", "line_comment", "multiline_comment":
		return true
	default:
		return false
	}
}

func truncateSyntaxComment(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	end := limit
	for end > 0 && value[end]&0xc0 == 0x80 {
		end--
	}
	return value[:end]
}

func syntaxGoDeclarationFacts(node syntaxView, source []byte, scope string, lines []uint32, local bool) []SyntaxFact {
	var result []SyntaxFact
	for i := 0; i < node.ChildCount(); i++ {
		child := node.ChildAt(i)
		if !child.IsNamed() || (child.Kind() != "var_spec" && child.Kind() != "const_spec") {
			continue
		}
		name, ok := findField(child, "name")
		if !ok {
			continue
		}
		value := strings.TrimSpace(nodeText(name, source))
		if value == "" || value == "_" {
			continue
		}
		fact := syntaxFact("variable", value, scope, child, lines, 1)
		fact.Local = local
		fact.IsExported = pinnedIsExported(value, "go")
		result = append(result, fact)
	}
	return result
}

func syntaxExceptionName(node syntaxView, source []byte) string {
	for i := 0; i < node.ChildCount(); i++ {
		child := node.ChildAt(i)
		if child.Kind() == "raise" || child.Kind() == "throw" || child.Kind() == ";" || child.Kind() == "(" || child.Kind() == ")" {
			continue
		}
		switch child.Kind() {
		case "call", "call_expression", "new_expression", "object_creation_expression", "instance_expression":
			for _, field := range []string{"function", "constructor", "type"} {
				if target, ok := findField(child, field); ok {
					return strings.TrimSpace(nodeText(target, source))
				}
			}
			for i := 0; i < child.ChildCount(); i++ {
				target := child.ChildAt(i)
				if target.IsNamed() {
					return strings.TrimSpace(nodeText(target, source))
				}
			}
		default:
			if child.IsNamed() {
				return strings.TrimSpace(nodeText(child, source))
			}
		}
		break
	}
	return ""
}

// syntaxExactCalleeChild matches Superopen's occurrence rule: a call consumes
// only its callable expression (and the terminal callable leaf). Receivers,
// qualifiers, computed keys, arguments, and nested bodies remain value usages.
func syntaxExactCalleeChild(parent, child syntaxView, parentIsCallee, parentIsCall bool) bool {
	if parentIsCall {
		switch child.FieldName() {
		case "function", "callee", "method", "name":
			return true
		}
		return false
	}
	if !parentIsCallee {
		return false
	}
	switch child.FieldName() {
	case "field", "property", "method", "name":
		return true
	case "object", "receiver", "namespace", "scope":
		return false
	}
	named := 0
	for i := 0; i < parent.ChildCount(); i++ {
		candidate := parent.ChildAt(i)
		if candidate.IsNamed() {
			named++
		}
	}
	return named == 1 && child.IsNamed()
}

func syntaxImportFacts(language string, node syntaxView, source []byte, scope string, lines []uint32) []SyntaxFact {
	if isJSLanguage(language) || language == "javascript" {
		return syntaxESImportFacts(node, source, scope, lines)
	}
	if language == "python" {
		return syntaxPythonImportFacts(node, source, scope, lines)
	}
	if language != "go" {
		return nil
	}
	var result []SyntaxFact
	var visit func(syntaxView)
	visit = func(current syntaxView) {
		if current.Kind() == "import_spec" {
			pathNode, ok := findField(current, "path")
			if !ok {
				return
			}
			importPath := trimSyntaxLiteral(strings.TrimSpace(nodeText(pathNode, source)))
			if importPath == "" {
				return
			}
			localName := importPathLast(importPath)
			if nameNode, found := findField(current, "name"); found {
				localName = strings.TrimSpace(nodeText(nameNode, source))
			}
			fact := syntaxFact("import", importPath, scope, current, lines, 1)
			fact.LocalName = localName
			result = append(result, fact)
			return
		}
		for i := 0; i < current.ChildCount(); i++ {
			child := current.ChildAt(i)
			visit(child)
		}
	}
	visit(node)
	return result
}

func syntaxPythonImportFacts(node syntaxView, source []byte, scope string, lines []uint32) []SyntaxFact {
	if node.Kind() == "future_import_statement" {
		fact := syntaxFact("import", "__future__", scope, node, lines, 1)
		fact.LocalName = "__future__"
		return []SyntaxFact{fact}
	}
	if node.Kind() == "import_statement" {
		if name, found := findField(node, "name"); found {
			if name.Kind() == "aliased_import" {
				if fact, ok := syntaxPythonAliasedImport(name, "", source, scope, lines); ok {
					return []SyntaxFact{fact}
				}
				return nil
			}
			modulePath := strings.TrimSpace(nodeText(name, source))
			if modulePath != "" {
				fact := syntaxFact("import", modulePath, scope, name, lines, 1)
				fact.LocalName = pythonImportRoot(modulePath)
				return []SyntaxFact{fact}
			}
		}
		var result []SyntaxFact
		for i := 0; i < node.ChildCount(); i++ {
			child := node.ChildAt(i)
			switch child.Kind() {
			case "dotted_name", "identifier":
				modulePath := strings.TrimSpace(nodeText(child, source))
				if modulePath != "" {
					fact := syntaxFact("import", modulePath, scope, child, lines, 1)
					fact.LocalName = pythonImportRoot(modulePath)
					result = append(result, fact)
				}
			case "aliased_import":
				if fact, ok := syntaxPythonAliasedImport(child, "", source, scope, lines); ok {
					result = append(result, fact)
				}
			}
		}
		return result
	}

	var moduleNode syntaxView
	if candidate, found := findField(node, "module_name"); found {
		moduleNode = candidate
	} else {
		for i := 0; i < node.ChildCount(); i++ {
			child := node.ChildAt(i)
			if child.Kind() == "dotted_name" || child.Kind() == "relative_import" {
				moduleNode = child
				break
			}
		}
	}
	modulePath := strings.TrimSpace(nodeText(moduleNode, source))
	var result []SyntaxFact
	for i := 0; i < node.ChildCount(); i++ {
		child := node.ChildAt(i)
		if child.StartByte() == moduleNode.StartByte() && child.EndByte() == moduleNode.EndByte() && moduleNode.EndByte() > moduleNode.StartByte() {
			continue
		}
		switch child.Kind() {
		case "identifier", "dotted_name":
			name := strings.TrimSpace(nodeText(child, source))
			if name == "" {
				continue
			}
			full := name
			if modulePath != "" {
				full = modulePath + "." + name
			}
			fact := syntaxFact("import", full, scope, child, lines, 1)
			fact.LocalName = name
			result = append(result, fact)
		case "aliased_import":
			if fact, ok := syntaxPythonAliasedImport(child, modulePath, source, scope, lines); ok {
				result = append(result, fact)
			}
		case "wildcard_import":
			if modulePath != "" {
				fact := syntaxFact("import", modulePath, scope, child, lines, 1)
				fact.LocalName = importPathLast(modulePath)
				result = append(result, fact)
			}
		}
	}
	if len(result) == 0 && modulePath != "" {
		fact := syntaxFact("import", modulePath, scope, node, lines, 1)
		fact.LocalName = importPathLast(modulePath)
		result = append(result, fact)
	}
	return result
}

func syntaxPythonAliasedImport(node syntaxView, prefix string, source []byte, scope string, lines []uint32) (SyntaxFact, bool) {
	nameNode, found := findField(node, "name")
	if !found {
		return SyntaxFact{}, false
	}
	name := strings.TrimSpace(nodeText(nameNode, source))
	if name == "" {
		return SyntaxFact{}, false
	}
	localName := importPathLast(name)
	if alias, ok := findField(node, "alias"); ok {
		localName = strings.TrimSpace(nodeText(alias, source))
	}
	modulePath := name
	if prefix != "" {
		modulePath = prefix + "." + name
	}
	fact := syntaxFact("import", modulePath, scope, node, lines, 1)
	fact.LocalName = localName
	return fact, true
}

func pythonImportRoot(modulePath string) string {
	if dot := strings.IndexByte(modulePath, '.'); dot >= 0 {
		return modulePath[:dot]
	}
	return modulePath
}

func syntaxESImportFacts(node syntaxView, source []byte, scope string, lines []uint32) []SyntaxFact {
	sourceNode, ok := findField(node, "source")
	if !ok {
		return nil
	}
	modulePath := trimSyntaxLiteral(strings.TrimSpace(nodeText(sourceNode, source)))
	if modulePath == "" {
		return nil
	}
	var names []string
	var visit func(syntaxView)
	visit = func(current syntaxView) {
		switch current.Kind() {
		case "import_specifier":
			if alias, found := findField(current, "alias"); found {
				names = append(names, strings.TrimSpace(nodeText(alias, source)))
			} else if name, found := findField(current, "name"); found {
				names = append(names, strings.TrimSpace(nodeText(name, source)))
			}
			return
		case "namespace_import":
			if name, found := findField(current, "name"); found {
				names = append(names, strings.TrimSpace(nodeText(name, source)))
				return
			}
		}
		for i := 0; i < current.ChildCount(); i++ {
			child := current.ChildAt(i)
			if child.StartByte() == sourceNode.StartByte() && child.EndByte() == sourceNode.EndByte() {
				continue
			}
			visit(child)
		}
	}
	visit(node)
	names = append(names, esImportLocalNames(nodeText(node, source))...)
	// Default imports are commonly a direct identifier in import_clause.
	for i := 0; i < node.ChildCount(); i++ {
		child := node.ChildAt(i)
		if child.Kind() != "import_clause" {
			continue
		}
		for i := 0; i < child.ChildCount(); i++ {
			item := child.ChildAt(i)
			if isIdentifierNode(item.Kind()) {
				if name := identifierText(item, source); name != "" && name != "type" {
					names = append(names, name)
				}
				break
			}
		}
	}
	if len(names) == 0 {
		names = append(names, importPathLast(modulePath))
	}
	result := make([]SyntaxFact, 0, len(names))
	seen := map[string]bool{}
	for _, localName := range names {
		if localName == "" || seen[localName] {
			continue
		}
		seen[localName] = true
		fact := syntaxFact("import", modulePath, scope, node, lines, 1)
		fact.LocalName = localName
		result = append(result, fact)
	}
	return result
}

func esImportLocalNames(statement string) []string {
	statement = strings.TrimSpace(statement)
	if !strings.HasPrefix(statement, "import") {
		return nil
	}
	var result []string
	if open, close := strings.IndexByte(statement, '{'), strings.IndexByte(statement, '}'); open >= 0 && close > open {
		for _, item := range strings.Split(statement[open+1:close], ",") {
			fields := strings.Fields(strings.TrimSpace(item))
			if len(fields) == 0 {
				continue
			}
			name := fields[len(fields)-1]
			if len(fields) >= 3 && fields[len(fields)-2] != "as" {
				name = fields[0]
			}
			result = append(result, name)
		}
	}
	if star := strings.Index(statement, "* as "); star >= 0 {
		rest := statement[star+5:]
		if fields := strings.Fields(rest); len(fields) > 0 {
			result = append(result, strings.Trim(fields[0], ",;"))
		}
	}
	prefix := strings.TrimSpace(strings.TrimPrefix(statement, "import"))
	if index := strings.Index(prefix, "from"); index >= 0 {
		prefix = strings.TrimSpace(prefix[:index])
	}
	if index := strings.IndexAny(prefix, "{*"); index >= 0 {
		prefix = strings.TrimSpace(strings.TrimSuffix(prefix[:index], ","))
	}
	prefix = strings.TrimSpace(strings.TrimPrefix(prefix, "type "))
	if prefix == "type" {
		prefix = ""
	}
	if prefix != "" && !strings.HasPrefix(prefix, "\"") && !strings.HasPrefix(prefix, "'") {
		if comma := strings.IndexByte(prefix, ','); comma >= 0 {
			prefix = prefix[:comma]
		}
		if fields := strings.Fields(prefix); len(fields) > 0 {
			result = append(result, fields[0])
		}
	}
	return result
}

func importPathLast(value string) string {
	last := 0
	for index, char := range value {
		if char == '/' || char == '.' || char == ':' || char == '\\' {
			last = index + 1
		}
	}
	return value[last:]
}

func syntaxCallArguments(call syntaxView, source []byte) ([]SyntaxArgument, string) {
	arguments, ok := findField(call, "arguments")
	if !ok {
		for i := 0; i < call.ChildCount(); i++ {
			child := call.ChildAt(i)
			if child.Kind() == "argument_list" || child.Kind() == "arguments" || child.Kind() == "method_args" {
				arguments, ok = child, true
				break
			}
		}
	}
	if !ok {
		return nil, ""
	}
	result := make([]SyntaxArgument, 0, 8)
	firstString := ""
	position := 0
	for i := 0; i < arguments.ChildCount(); i++ {
		child := arguments.ChildAt(i)
		if !child.IsNamed() || len(result) == 8 {
			continue
		}
		argument := SyntaxArgument{Index: position}
		valueNode := child
		if child.Kind() == "keyword_argument" || child.Kind() == "pair" {
			if key, found := findField(child, "name"); found {
				argument.Keyword = strings.TrimSpace(nodeText(key, source))
			} else if key, found := findField(child, "key"); found {
				argument.Keyword = strings.TrimSpace(nodeText(key, source))
			}
			if value, found := findField(child, "value"); found {
				valueNode = value
			}
		}
		argument.Expr = strings.TrimSpace(nodeText(valueNode, source))
		if isStringLikeSyntaxNode(valueNode) {
			argument.Literal = true
			argument.Value = trimSyntaxLiteral(argument.Expr)
			if firstString == "" {
				firstString = argument.Value
			}
		}
		result = append(result, argument)
		position++
	}
	return result, firstString
}

func isStringLikeSyntaxNode(node syntaxView) bool {
	if isStringNode(node.Kind()) {
		return true
	}
	switch node.Kind() {
	case "template_string", "template_literal", "raw_string_literal", "interpreted_string_literal",
		"string_content", "heredoc_body", "concatenated_string":
		return true
	default:
		return false
	}
}

func isJSLanguage(language string) bool {
	return language == "javascript" || language == "typescript" || language == "tsx"
}

// isJSModuleLevelVariable matches Superopen extract_variables for JS/TS: only
// direct program children, optionally wrapped by export_statement /
// expression_statement / statement, become Variable nodes.
func isJSModuleLevelVariable(ancestors []syntaxView) bool {
	switch len(ancestors) {
	case 1:
		return true
	case 2:
		switch ancestors[1].Kind() {
		case "export_statement", "expression_statement", "statement":
			return true
		}
	}
	return false
}

// syntaxOpensImportContext matches Superopen is_import_context_kind: re-export
// export_statement forms suppress nested usages, but `export const/function/...`
// declarations must still emit ordinary USAGE edges in their bodies.
func syntaxOpensImportContext(language string, node syntaxView, sets specSets) bool {
	if !sets.imports[node.Kind()] && !sets.importsFrom[node.Kind()] {
		return false
	}
	if isJSLanguage(language) && node.Kind() == "export_statement" {
		if _, ok := findField(node, "source"); ok {
			return true
		}
		hasExportClause := false
		hasDeclaration := false
		if _, ok := findField(node, "declaration"); ok {
			hasDeclaration = true
		}
		for i := 0; i < node.ChildCount(); i++ {
			child := node.ChildAt(i)
			if !child.IsNamed() {
				continue
			}
			if child.Kind() == "export_clause" {
				hasExportClause = true
			}
			if child.FieldName() == "declaration" {
				hasDeclaration = true
			}
		}
		return hasExportClause && !hasDeclaration
	}
	return true
}

// syntaxEmitsImportFacts rejects JS/TS export statements that carry no module
// source. `export default App` and `export { App }` bind local symbols, so
// Superopen never records them as imports.
func syntaxEmitsImportFacts(language string, node syntaxView) bool {
	if !isJSLanguage(language) || node.Kind() != "export_statement" {
		return true
	}
	_, ok := findField(node, "source")
	return ok
}

// jsClassMethodNode reports whether a published function is a class member
// (method_definition or class-field arrow), matching extract_class_methods.
func jsClassMethodNode(node, parent syntaxView) bool {
	switch node.Kind() {
	case "method_definition":
		return true
	case "arrow_function", "function_expression", "generator_function":
		switch parent.Kind() {
		case "public_field_definition", "field_definition":
			return true
		}
	}
	return false
}

func functionVariableName(node syntaxView, source []byte) string {
	var declarator syntaxView
	if node.Kind() == "variable_declarator" {
		declarator = node
	} else {
		for i := 0; i < node.ChildCount(); i++ {
			child := node.ChildAt(i)
			if child.Kind() == "variable_declarator" {
				declarator = child
				break
			}
		}
	}
	if viewMissing(declarator) {
		return ""
	}
	value, ok := findField(declarator, "value")
	if !ok || value.Kind() != "arrow_function" {
		return ""
	}
	name, ok := findField(declarator, "name")
	if !ok {
		return ""
	}
	return identifierText(name, source)
}

func pinnedIsExported(name, language string) bool {
	if name == "" {
		return false
	}
	first := name[0]
	switch language {
	case "go", "java", "c_sharp", "kotlin":
		return first >= 'A' && first <= 'Z'
	case "python":
		return first != '_'
	default:
		return true
	}
}

func collectParameterNames(node syntaxView, source []byte, result *[]string) {
	if isIdentifierNode(node.Kind()) {
		if value := trimSyntaxLiteral(nodeText(node, source)); value != "" {
			*result = append(*result, value)
		}
		return
	}
	for i := 0; i < node.ChildCount(); i++ {
		child := node.ChildAt(i)
		if child.FieldName() == "type" || strings.Contains(child.Kind(), "type") {
			continue
		}
		collectParameterNames(child, source, result)
	}
}

func syntaxParameterNames(parameters syntaxView, source []byte) []string {
	result := make([]string, 0, 8)
	for i := 0; i < parameters.ChildCount(); i++ {
		parameter := parameters.ChildAt(i)
		if !parameter.IsNamed() {
			continue
		}
		if name, ok := findField(parameter, "name"); ok {
			if value := strings.TrimSpace(nodeText(name, source)); value != "" {
				result = append(result, value)
			}
			continue
		}
		if parameter.Kind() == "identifier" {
			if value := strings.TrimSpace(nodeText(parameter, source)); value != "" {
				result = append(result, value)
			}
		}
	}
	return result
}

func syntaxParameterTypes(parameters syntaxView, source []byte) []string {
	seen := map[string]bool{}
	result := make([]string, 0, 8)
	for i := 0; i < parameters.ChildCount(); i++ {
		parameter := parameters.ChildAt(i)
		if !parameter.IsNamed() {
			continue
		}
		typeNode, ok := findField(parameter, "type")
		if !ok {
			continue
		}
		name := cleanSyntaxType(nodeText(typeNode, source))
		if name == "" || syntaxBuiltinType(name) || seen[name] {
			continue
		}
		seen[name] = true
		result = append(result, name)
	}
	return result
}

func cleanSyntaxType(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimLeft(value, ":*&[].")
	if end := strings.IndexAny(value, "<[ \t\r\n"); end >= 0 {
		value = value[:end]
	}
	return value
}

func syntaxBuiltinType(value string) bool {
	switch value {
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64",
		"float", "float32", "float64", "double", "string", "str", "bool", "boolean", "byte", "rune",
		"void", "None", "any", "interface", "object", "Object", "error", "uintptr", "complex64", "complex128",
		"number", "bigint", "symbol", "undefined", "null", "char", "short", "long", "i8", "i16", "i32",
		"i64", "u8", "u16", "u32", "u64", "f32", "f64", "usize", "isize", "self", "Self", "cls",
		"type", "Int", "Int8", "Int16", "Int32", "Int64", "UInt", "UInt8", "UInt16", "UInt32", "UInt64",
		"Float", "Double", "String", "Bool", "Boolean", "Byte", "Short", "Long", "Char", "Unit", "Void",
		"Any", "Nothing", "Dynamic":
		return true
	default:
		return false
	}
}

func syntaxGoReceiverType(receiver syntaxView, source []byte) string {
	var find func(syntaxView, int) string
	find = func(node syntaxView, depth int) string {
		if depth > 4 {
			return ""
		}
		if node.Kind() == "type_identifier" {
			return strings.TrimSpace(nodeText(node, source))
		}
		if node.Kind() == "parameter_declaration" {
			if value, ok := findField(node, "type"); ok {
				return find(value, depth+1)
			}
		}
		if node.Kind() == "pointer_type" || node.Kind() == "generic_type" {
			if value, ok := findField(node, "type"); ok {
				return find(value, depth+1)
			}
		}
		for i := 0; i < node.ChildCount(); i++ {
			child := node.ChildAt(i)
			if value := find(child, depth+1); value != "" {
				return value
			}
		}
		return ""
	}
	return find(receiver, 0)
}

// syntaxBaseTypeNode reports whether a node kind names a base type directly.
func syntaxBaseTypeNode(kind string) bool {
	switch kind {
	case "type_identifier", "generic_type", "qualified_name", "scoped_type_identifier", "user_type":
		return true
	}
	return false
}

// syntaxBaseName trims a generic argument list off a captured base type, since
// Superopen stores `Base` for `Base<T>`.
func syntaxBaseName(node syntaxView, source []byte) string {
	text := strings.TrimSpace(nodeText(node, source))
	if index := strings.IndexByte(text, '<'); index >= 0 {
		text = text[:index]
	}
	return strings.TrimSpace(text)
}

// collectSyntaxBases matches Superopen collect_bases_from_field: it reads base
// types out of a heritage field, falling back to the field's raw text when no
// recognized child kind is present.
func collectSyntaxBases(field syntaxView, source []byte) []string {
	if syntaxBaseTypeNode(field.Kind()) {
		if name := syntaxBaseName(field, source); name != "" {
			return []string{name}
		}
		return nil
	}
	var result []string
	for i := 0; i < field.ChildCount(); i++ {
		child := field.ChildAt(i)
		if !child.IsNamed() {
			continue
		}
		switch {
		// Python carries a bare `identifier` for `class C(Base)` and an
		// `attribute` for a dotted base such as `mod.Base`.
		case syntaxBaseTypeNode(child.Kind()), child.Kind() == "identifier", child.Kind() == "attribute":
			if name := syntaxBaseName(child, source); name != "" {
				result = append(result, name)
			}
		// A parameterized Python base such as `Generic[T]` stores only the
		// subscripted value; the bracketed arguments must not leak in.
		case child.Kind() == "subscript":
			value, ok := findField(child, "value")
			if !ok {
				if named := firstNamedChild(child); named != nil {
					value, ok = named, true
				}
			}
			if ok {
				if name := strings.TrimSpace(nodeText(value, source)); name != "" {
					result = append(result, name)
				}
			}
		case child.Kind() == "type_list", child.Kind() == "interface_type_list":
			for i := 0; i < child.ChildCount(); i++ {
				entry := child.ChildAt(i)
				if entry.IsNamed() && syntaxBaseTypeNode(entry.Kind()) {
					if name := syntaxBaseName(entry, source); name != "" {
						result = append(result, name)
					}
				}
			}
		}
	}
	if len(result) == 0 {
		if name := strings.TrimSpace(nodeText(field, source)); name != "" {
			return []string{name}
		}
	}
	return result
}

func firstNamedChild(node syntaxView) syntaxView {
	for index := 0; index < node.ChildCount(); index++ {
		child := node.ChildAt(index)
		if child.IsNamed() {
			return child
		}
	}
	return nil
}

func syntaxBaseClasses(language string, node syntaxView, source []byte) []string {
	// TypeScript/TSX heritage lives under class_heritage / extends_type_clause;
	// the generic field walker would otherwise capture the "extends" keyword
	// text (e.g. "extends Guard") instead of the type name.
	if isJSLanguage(language) {
		if bases := syntaxTSBaseClasses(node, source); len(bases) > 0 {
			return bases
		}
	}
	var result []string
	for _, field := range []string{"superclass", "superclasses", "superinterfaces", "interfaces",
		"bases", "type_inheritance_clause", "delegation_specifiers"} {
		if value, ok := findField(node, field); ok {
			result = append(result, collectSyntaxBases(value, source)...)
		}
	}
	// Some grammars expose heritage as a named child rather than a field, such
	// as Java `interface X extends A, B` producing `extends_interfaces`.
	for i := 0; i < node.ChildCount(); i++ {
		child := node.ChildAt(i)
		if child.Kind() == "extends_interfaces" || child.Kind() == "super_interfaces" {
			result = append(result, collectSyntaxBases(child, source)...)
		}
	}
	if len(result) > 0 {
		return result
	}
	for i := 0; i < node.ChildCount(); i++ {
		child := node.ChildAt(i)
		switch child.Kind() {
		case "superclass", "superinterfaces", "type_inheritance_clause", "class_heritage",
			"delegation_specifiers", "super_interfaces", "extends_clause", "implements_clause",
			"extends_type_clause", "argument_list", "inheritance_specifier", "base_class_clause",
			"base_list":
			if bases := collectSyntaxBases(child, source); len(bases) > 0 {
				return bases
			}
		}
	}
	_ = language
	return result
}

// syntaxTSBaseClasses matches Superopen extract_ts_bases: walk class_heritage
// (extends_clause + implements_clause) or a bare interface extends_type_clause,
// reading type names rather than keyword text.
func syntaxTSBaseClasses(node syntaxView, source []byte) []string {
	var result []string
	for i := 0; i < node.ChildCount(); i++ {
		child := node.ChildAt(i)
		switch child.Kind() {
		case "class_heritage":
			for i := 0; i < child.ChildCount(); i++ {
				clause := child.ChildAt(i)
				result = append(result, collectTSBases(clause, source)...)
			}
		case "extends_type_clause":
			result = append(result, collectTSBases(child, source)...)
		}
	}
	return result
}

func collectTSBases(clause syntaxView, source []byte) []string {
	switch clause.Kind() {
	case "extends_clause":
		if value, ok := findField(clause, "value"); ok {
			if name := syntaxBaseName(value, source); name != "" {
				return []string{name}
			}
		}
		return nil
	case "implements_clause", "extends_type_clause":
		var result []string
		for i := 0; i < clause.ChildCount(); i++ {
			child := clause.ChildAt(i)
			if !child.IsNamed() || child.Kind() == "type_arguments" {
				continue
			}
			if child.Kind() == "generic_type" {
				if nameNode, ok := findField(child, "name"); ok {
					if name := syntaxBaseName(nameNode, source); name != "" {
						result = append(result, name)
					}
					continue
				}
			}
			if name := syntaxBaseName(child, source); name != "" {
				result = append(result, name)
			}
		}
		return result
	default:
		return nil
	}
}

type syntaxComplexityMetrics struct {
	cyclomatic, cognitive, loopCount, loopDepth, maxAccessDepth int
}

func syntaxComplexity(root syntaxView, branches map[string]bool) syntaxComplexityMetrics {
	type frame struct {
		node                   syntaxView
		branchDepth, loopDepth int
		accessDepth            int
	}
	stack := []frame{{node: root}}
	var result syntaxComplexityMetrics
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		childBranch, childLoop, childAccess := current.branchDepth, current.loopDepth, 0
		if branches[current.node.Kind()] {
			result.cyclomatic++
			result.cognitive += 1 + current.branchDepth
			childBranch++
		}
		if current.node.IsNamed() && syntaxLoopNode(current.node.Kind()) {
			result.loopCount++
			childLoop++
			if childLoop > result.loopDepth {
				result.loopDepth = childLoop
			}
		}
		if current.node.IsNamed() && syntaxAccessNode(current.node.Kind()) {
			childAccess = current.accessDepth + 1
			if childAccess > result.maxAccessDepth {
				result.maxAccessDepth = childAccess
			}
		}
		kids := viewPushChildrenReversed(current.node, nil, 0)
		for _, child := range kids {
			stack = append(stack, frame{node: child, branchDepth: childBranch,
				loopDepth: childLoop, accessDepth: childAccess})
		}
	}
	return result
}

func syntaxLoopNode(kind string) bool {
	switch kind {
	case "for_statement", "while_statement", "do_statement", "do_while_statement", "for_in_statement",
		"for_of_statement", "for_each_statement", "foreach_statement", "enhanced_for_statement", "for_range_loop",
		"c_style_for_statement", "for_expression", "while_expression", "loop_expression", "while_let_expression",
		"repeat_statement", "repeat_while_statement", "until", "while_modifier", "until_modifier", "for", "while":
		return true
	default:
		return false
	}
}

func syntaxAccessNode(kind string) bool {
	switch kind {
	case "member_expression", "field_expression", "selector_expression", "field_access", "member_access_expression",
		"navigation_expression", "attribute", "subscript_expression", "subscript", "index_expression",
		"element_access_expression", "scoped_identifier":
		return true
	default:
		return false
	}
}

func fillFunctionBodyFacts(fact *SyntaxFact, body syntaxView, source []byte, parameters []string) {
	tokens, profile, fingerprint, profileOK, hashOK := syntaxFusedBodyWalk(body, source, parameters)
	fact.BodyTokens = tokens
	if profileOK {
		fact.StructuralProfile = profile.String()
	}
	if hashOK {
		fact.MinHash = minHashHex(fingerprint)
	}
}

func syntaxBodyTokens(root syntaxView, source []byte) string {
	tokens, _, _, _, _ := syntaxFusedBodyWalk(root, source, nil)
	return tokens
}

func syntaxFusedBodyWalk(root syntaxView, source []byte, parameterNames []string) (string, ASTProfile, minHashFingerprint, bool, bool) {
	var profile ASTProfile
	profile[profileParameters] = uint16(len(parameterNames))
	type frame struct {
		node  syntaxView
		depth int
	}
	stack := []frame{{node: root}}
	seen := map[string]bool{}
	values := make([]string, 0, 128)
	minhashTokens := make([]string, 0, 128)
	operatorSet, operandSet := map[uint32]bool{}, map[uint32]bool{}
	totalDepth, nodeCount := 0, 0
	for len(stack) > 0 {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		node := current.node
		if node == nil {
			continue
		}
		kind := node.Kind()
		count := node.ChildCount()
		if len(minhashTokens) < minHashMaxTokens && count == 0 && kind != "" {
			minhashTokens = append(minhashTokens, normalizeSyntaxType(kind))
		}
		if len(values) < 128 && count == 0 && syntaxBodyIdentifier(kind) {
			value := nodeText(node, source)
			if len(value) > 0 && len(value) < 64 && !seen[value] {
				seen[value] = true
				values = append(values, value)
			}
		}
		if nodeCount < 2048 && (node.IsNamed() || count > 0) {
			nodeCount++
			totalDepth += current.depth
			if current.depth > int(profile[profileMaxDepth]) {
				profile[profileMaxDepth] = uint16(current.depth)
			}
			accumulateProfileKind(&profile, kind)
			if profileOperator(kind) {
				profile[profileTotalOperators]++
				hash := profileHash(kind)
				if !operatorSet[hash] && len(operatorSet) < 512 {
					operatorSet[hash] = true
					profile[profileUniqueOperators]++
				}
			}
			if count == 0 && profileOperand(kind) {
				profile[profileTotalOperands]++
				profile[profileBodyTokens]++
				hash := profileHash(kind)
				if !operandSet[hash] && len(operandSet) < 512 {
					operandSet[hash] = true
					profile[profileUniqueOperands]++
				}
			}
		}
		if len(minhashTokens) >= minHashMaxTokens && len(values) >= 128 && nodeCount >= 2048 {
			continue
		}
		limit := minHashWalkStack
		if len(values) < 128 {
			limit = 0
		}
		var kids []syntaxView
		viewEachChild(node, func(child syntaxView) { kids = append(kids, child) })
		for index := len(kids) - 1; index >= 0; index-- {
			if limit > 0 && len(stack) >= limit {
				break
			}
			stack = append(stack, frame{node: kids[index], depth: current.depth + 1})
		}
	}
	bodyTokens := strings.Join(values, " ")
	var fingerprint minHashFingerprint
	hashOK := false
	if len(minhashTokens) >= minHashMinLeaves {
		fingerprint, hashOK = minHashFromTokens(minhashTokens)
	}
	profileOK := nodeCount > 0
	if profileOK {
		profile[profileAverageDepthX10] = uint16(totalDepth * 10 / nodeCount)
		lines := sourceLineIndex(source)
		startLine, _ := bytePosition(lines, root.StartByte())
		endLine, _ := bytePosition(lines, root.EndByte())
		if endLine >= startLine {
			profile[profileBodyLines] = uint16(endLine - startLine + 1)
		}
	}
	return bodyTokens, profile, fingerprint, profileOK, hashOK
}

func syntaxBodyIdentifier(kind string) bool {
	switch kind {
	case "identifier", "field_identifier", "property_identifier", "type_identifier", "objectscript_identifier",
		"objectscript_identifier_special", "identifier_segment_immediate", "identifier_segment_immediate_special",
		"class_name", "method_name", "routine_name", "quote_permitting_identifier":
		return true
	default:
		return false
	}
}

type specSets struct {
	functions, classes, fields, modules, calls, imports, importsFrom, variables, assignments, branches, throws, decorators map[string]bool
	functionIDs, classIDs, fieldIDs, moduleIDs, callIDs, variableIDs                                                       map[uint16]bool
}

func specHit(names map[string]bool, ids map[uint16]bool, node syntaxView) bool {
	if len(ids) > 0 {
		if keyed, ok := node.(interface{ KindId() uint16 }); ok {
			return ids[keyed.KindId()]
		}
	}
	return names[node.Kind()]
}

func syntaxSpecSets(spec LanguageSpec) specSets {
	return specSets{
		functions: stringSet(spec.Functions), classes: stringSet(spec.Classes), fields: stringSet(spec.Fields),
		modules: stringSet(spec.Modules), calls: stringSet(spec.Calls), imports: stringSet(spec.Imports),
		importsFrom: stringSet(spec.ImportsFrom), variables: stringSet(spec.Variables),
		assignments: stringSet(spec.Assignments),
		branches:    stringSet(spec.Branches), throws: stringSet(spec.Throws), decorators: stringSet(spec.Decorators),
	}
}

func isSyntaxReferenceNode(language string, node, parent syntaxView, ancestors []syntaxView) bool {
	kind := node.Kind()
	if language == "python" && kind == "attribute" && syntaxDirectArgumentValue(node, ancestors) {
		return false
	}
	if language == "rust" && (kind == "identifier" || kind == "scoped_identifier") && parent.Kind() == "scoped_identifier" {
		return false
	}
	if kind == "identifier" && (language == "puppet" && parent.Kind() == "variable" || language == "vim" && parent.Kind() == "argument") {
		return false
	}
	if isIdentifierNode(kind) || kind == "simple_identifier" {
		return true
	}
	switch language {
	case "javascript", "typescript", "tsx", "qml", "cfscript":
		return kind == "property_identifier" || kind == "private_property_identifier"
	case "go":
		return kind == "field_identifier" || kind == "package_identifier"
	case "python":
		return kind == "attribute"
	case "rust":
		return kind == "field_identifier" || kind == "scoped_identifier"
	case "c", "cpp", "cuda":
		return kind == "field_identifier"
	case "php":
		return kind == "name" || kind == "variable_name"
	case "haskell":
		return kind == "variable" || kind == "constructor"
	case "ocaml":
		return kind == "value_path" || kind == "constructor_path"
	case "erlang":
		return kind == "atom" || kind == "var"
	case "css":
		return kind == "plain_value"
	case "scss":
		return kind == "variable_value"
	case "llvm":
		return kind == "local_var" || kind == "global_var"
	case "powershell", "puppet":
		return kind == "variable"
	case "bash", "fish", "zsh":
		return kind == "variable_name"
	case "perl":
		return kind == "scalar"
	case "clojure", "commonlisp":
		return kind == "sym_lit"
	case "vim":
		return kind == "scoped_identifier" || kind == "argument"
	case "elm":
		return kind == "lower_case_identifier"
	case "cobol":
		return kind == "qualified_word"
	case "elisp", "scheme", "fennel", "racket", "linkerscript":
		return kind == "symbol"
	case "makefile":
		return kind == "variable_reference"
	case "cmake":
		return kind == "variable"
	case "wolfram":
		return kind == "user_symbol"
	case "typst", "nickel":
		return kind == "ident"
	case "tcl":
		return kind == "variable_substitution"
	case "tlaplus":
		return kind == "identifier_ref"
	case "agda":
		return kind == "qid"
	case "rescript":
		return kind == "value_identifier"
	case "purescript":
		return kind == "variable"
	case "jsonnet":
		return kind == "id"
	case "cfml":
		return kind == "property_identifier"
	case "objectscript_udl", "objectscript_routine":
		return kind == "objectscript_identifier" || kind == "objectscript_identifier_special"
	default:
		return false
	}
}

func syntaxReferenceName(language string, node syntaxView, source []byte) string {
	value := strings.TrimSpace(nodeText(node, source))
	if language == "makefile" && node.Kind() == "variable_reference" {
		if strings.HasPrefix(value, "$(") && strings.HasSuffix(value, ")") && len(value) > 3 {
			value = value[2 : len(value)-1]
		}
	}
	return strings.TrimSpace(trimSyntaxLiteral(value))
}

func syntaxBindingOccurrence(language string, node syntaxView, ancestors []syntaxView, sets specSets) bool {
	field := node.FieldName()
	for index := len(ancestors) - 1; index >= 0; index-- {
		parent := ancestors[index]
		if syntaxValueField(field) || field == "type" {
			return false
		}
		if syntaxWholeBindingNode(parent.Kind()) || syntaxLanguageWholeBindingNode(language, parent.Kind()) {
			return true
		}
		declared := sets.functions[parent.Kind()] || sets.classes[parent.Kind()] || sets.fields[parent.Kind()] ||
			sets.variables[parent.Kind()] || syntaxFieldBindingNode(parent.Kind())
		if declared && syntaxBindingField(field) {
			return true
		}
		field = parent.FieldName()
	}
	return false
}

func syntaxWriteOccurrence(language string, node syntaxView, ancestors []syntaxView, sets specSets) bool {
	field := node.FieldName()
	for index := len(ancestors) - 1; index >= 0; index-- {
		parent := ancestors[index]
		if syntaxValueField(field) {
			return false
		}
		assignment := sets.assignments[parent.Kind()] || syntaxLanguageWriteNode(language, parent.Kind())
		if assignment {
			if syntaxAssignmentReadsTarget(parent) {
				return false
			}
			if language == "linkerscript" && parent.Kind() == "assignment" {
				return syntaxFirstChildContains(parent, node)
			}
			if field == "left" || field == "name" || field == "target" || field == "destination" {
				return true
			}
			if syntaxFirstNamedChildIsWrite(language) && syntaxNamedChildContains(parent, 0, node) {
				return true
			}
			return false
		}
		field = parent.FieldName()
	}
	return false
}

func syntaxCallArgumentLabel(node syntaxView, ancestors []syntaxView) bool {
	contained := node
	for index := len(ancestors) - 1; index >= 0; index-- {
		parent := ancestors[index]
		switch parent.Kind() {
		case "keyword_argument", "named_argument", "labeled_argument":
			return contained.FieldName() == "name" || contained.FieldName() == "label" || contained.FieldName() == "key"
		case "arguments", "argument_list", "value_arguments":
			return false
		}
		contained = parent
	}
	return false
}

func syntaxDirectArgumentValue(node syntaxView, ancestors []syntaxView) bool {
	if len(ancestors) == 0 {
		return false
	}
	parent := ancestors[len(ancestors)-1]
	switch parent.Kind() {
	case "keyword_argument", "named_argument", "labeled_argument":
		if node.FieldName() != "value" {
			return false
		}
		return syntaxDirectArgumentValue(parent, ancestors[:len(ancestors)-1])
	case "arguments", "argument_list", "value_arguments":
		return true
	case "list_expression":
		return parent.FieldName() == "arguments"
	case "argument", "value_argument":
		if len(ancestors) < 2 {
			return false
		}
		switch ancestors[len(ancestors)-2].Kind() {
		case "arguments", "argument_list", "value_arguments":
			return true
		}
		return false
	}
	return node.FieldName() == "arguments"
}

func syntaxForwardSiblingCallee(language string, node, parent syntaxView) bool {
	if language == "dart" && node.Kind() == "identifier" {
		return syntaxNextNamedSiblingType(parent, node) == "selector"
	}
	if language != "vhdl" {
		return false
	}
	owner := node
	for depth := 0; depth < 4 && syntaxVHDLCalleeWrapper(owner.Kind()); depth++ {
		if syntaxNextNamedSiblingType(parent, owner) == "parenthesis_group" {
			return true
		}
		break
	}
	return false
}

func syntaxVHDLCalleeWrapper(kind string) bool {
	switch kind {
	case "identifier", "simple_identifier", "library_function", "name", "simple_name":
		return true
	default:
		return false
	}
}

func syntaxNextNamedSiblingType(parent, node syntaxView) string {
	seen := false
	for i := 0; i < parent.ChildCount(); i++ {
		child := parent.ChildAt(i)
		if !child.IsNamed() {
			continue
		}
		if seen {
			return child.Kind()
		}
		if child.StartByte() == node.StartByte() && child.EndByte() == node.EndByte() && child.Kind() == node.Kind() {
			seen = true
		}
	}
	return ""
}

func syntaxValueField(field string) bool {
	switch field {
	case "value", "right", "initializer", "default", "default_value", "body", "arguments",
		"condition", "consequence", "alternative", "expression", "result":
		return true
	default:
		return false
	}
}

func syntaxWholeBindingNode(kind string) bool {
	switch kind {
	case "formal_parameter", "formal_parameters", "parameter", "parameters", "parameter_list",
		"parameter_declaration", "parameter_specification", "required_parameter", "optional_parameter",
		"default_parameter", "typed_parameter", "function_value_parameter", "function_value_parameters",
		"lambda_parameter", "lambda_parameters", "function_parameter_declaration", "closure_parameters",
		"block_parameters", "receiver":
		return true
	default:
		return false
	}
}

func syntaxFieldBindingNode(kind string) bool {
	switch kind {
	case "variable_declarator", "init_declarator", "variable_declaration", "const_declaration",
		"lexical_declaration", "short_var_declaration", "local_variable_declaration", "property_declaration",
		"field_declaration", "value_declaration", "val_definition", "var_definition", "let_declaration",
		"local_bind", "let_binding", "data_declaration", "net_declaration", "object_declaration",
		"number_declaration", "typed_binding", "variable_assignment":
		return true
	default:
		return false
	}
}

func syntaxLanguageWholeBindingNode(language, kind string) bool {
	switch language {
	case "sql":
		return kind == "function_argument"
	case "haskell":
		return kind == "patterns"
	case "fsharp":
		return kind == "function_declaration_left" || kind == "value_declaration_left" || kind == "argument_patterns"
	case "crystal", "awk":
		return kind == "param_list"
	case "teal":
		return kind == "function_signature"
	case "verilog", "systemverilog":
		return kind == "tf_port_item" || kind == "tf_port_item1"
	case "rescript":
		return kind == "formal_parameters" || kind == "labeled_parameter" || kind == "parameter"
	case "purescript":
		return kind == "bind_pattern" || kind == "pattern" || kind == "patterns"
	case "nickel":
		return kind == "pattern_fun"
	case "jsonnet":
		return kind == "param"
	case "llvm":
		return kind == "function_header"
	case "objectscript_udl", "objectscript_routine":
		return kind == "argument" || kind == "tag_parameter"
	default:
		return false
	}
}

func syntaxBindingField(field string) bool {
	switch field {
	case "name", "pattern", "declarator", "parameter", "parameters", "left", "variable", "variables", "key":
		return true
	default:
		return false
	}
}

func syntaxLanguageWriteNode(language, kind string) bool {
	switch language {
	case "linkerscript":
		return kind == "assignment"
	case "meson":
		return kind == "operatorunit"
	case "gn":
		return kind == "assignment_statement"
	case "objectscript_udl", "objectscript_routine":
		return kind == "set_argument"
	default:
		return false
	}
}

func syntaxFirstNamedChildIsWrite(language string) bool {
	switch language {
	case "puppet", "meson", "gn", "linkerscript", "objectscript_udl", "objectscript_routine":
		return true
	default:
		return false
	}
}

func syntaxAssignmentReadsTarget(assignment syntaxView) bool {
	switch assignment.Kind() {
	case "augmented_assignment", "augmented_assignment_expression", "compound_assignment_expr",
		"compound_assignment_expression", "operator_assignment", "operator_assign", "update_exp",
		"postfix_unary_expression", "prefix_unary_expression":
		return true
	}
	for i := 0; i < assignment.ChildCount(); i++ {
		child := assignment.ChildAt(i)
		switch child.Kind() {
		case "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>=", "??=", "++", "--":
			return true
		}
	}
	return false
}

// syntaxAssignmentWriteName resolves the variable written by an assignment
// node, mirroring resolve_write_lhs_node plus resolve_lhs_write_name Superopen.
func syntaxAssignmentWriteName(language string, node syntaxView, source []byte) string {
	left, ok := syntaxWriteTargetNode(node)
	if !ok {
		return ""
	}
	return syntaxWriteTargetName(left, source)
}

func syntaxWriteTargetNode(node syntaxView) (syntaxView, bool) {
	if left, ok := findField(node, "left"); ok {
		return left, true
	}
	switch node.Kind() {
	case "postfix_unary_expression", "prefix_unary_expression", "update_expression":
		// Only ++/-- mutate their operand; other unary forms read it.
		increment := false
		for i := 0; i < node.ChildCount(); i++ {
			child := node.ChildAt(i)
			if child.IsNamed() {
				continue
			}
			if child.Kind() == "++" || child.Kind() == "--" {
				increment = true
				break
			}
		}
		if !increment {
			return SyntaxNode{}, false
		}
		for i := 0; i < node.ChildCount(); i++ {
			child := node.ChildAt(i)
			if !child.IsNamed() {
				continue
			}
			switch child.Kind() {
			case "identifier", "simple_identifier", "member_access_expression", "field_expression",
				"field_access", "selector_expression", "subscript_expression", "index_expression":
				return child, true
			}
		}
		return SyntaxNode{}, false
	}
	if node.ChildCount() > 0 {
		return node.ChildAt(0), true
	}
	return SyntaxNode{}, false
}

func syntaxWriteTargetName(left syntaxView, source []byte) string {
	if left.Kind() == "expression_list" {
		named := make([]syntaxView, 0, left.ChildCount())
		for i := 0; i < left.ChildCount(); i++ {
			child := left.ChildAt(i)
			if child.IsNamed() {
				named = append(named, child)
			}
		}
		if len(named) != 1 {
			return ""
		}
		left = named[0]
	}
	switch left.Kind() {
	case "identifier", "simple_identifier":
		return nodeText(left, source)
	case "index_expression", "subscript_expression":
		base, ok := findField(left, "operand")
		if !ok {
			base, ok = findField(left, "object")
		}
		if !ok {
			for i := 0; i < left.ChildCount(); i++ {
				child := left.ChildAt(i)
				if child.IsNamed() {
					base, ok = child, true
					break
				}
			}
		}
		if ok && (base.Kind() == "identifier" || base.Kind() == "simple_identifier") {
			return nodeText(base, source)
		}
		return ""
	case "field_expression", "member_access_expression", "field_access", "selector_expression":
		field, ok := findField(left, "field")
		if !ok {
			field, ok = findField(left, "name")
		}
		if ok {
			return nodeText(field, source)
		}
		return ""
	}
	return ""
}

func syntaxFirstChildContains(parent, node syntaxView) bool {
	if parent.ChildCount() == 0 {
		return false
	}
	return syntaxNodeContains(parent.ChildAt(0), node)
}

func syntaxNamedChildContains(parent syntaxView, index int, node syntaxView) bool {
	named := 0
	for i := 0; i < parent.ChildCount(); i++ {
		child := parent.ChildAt(i)
		if !child.IsNamed() {
			continue
		}
		if named == index {
			return syntaxNodeContains(child, node)
		}
		named++
	}
	return false
}

func syntaxNodeContains(outer, inner syntaxView) bool {
	return outer.StartByte() <= inner.StartByte() && outer.EndByte() >= inner.EndByte()
}

func collectSyntaxDecorators(node syntaxView, source []byte, decoratorKinds map[string]bool, lines []uint32, definitions []SyntaxFact, result *[]SyntaxFact) {
	var walk func(syntaxView)
	walk = func(current syntaxView) {
		if decoratorKinds[current.Kind()] {
			name := relationshipName(current, source, []string{"name", "function", "attribute"})
			name = strings.TrimLeft(compactRelationshipText(name), "@#[]")
			if index := strings.IndexAny(name, "(,["); index >= 0 {
				name = name[:index]
			}
			target := ""
			bestSpan := ^uint32(0)
			bestDistance := ^uint32(0)
			for _, definition := range definitions {
				span := definition.EndByte - definition.StartByte
				if definition.StartByte <= current.StartByte() && current.EndByte() <= definition.EndByte && span < bestSpan {
					target, bestSpan = joinSyntaxScope(definition.Scope, definition.Name), span
				}
				if target == "" && definition.StartByte >= current.EndByte() && definition.StartByte-current.EndByte() < bestDistance {
					target, bestDistance = joinSyntaxScope(definition.Scope, definition.Name), definition.StartByte-current.EndByte()
				}
			}
			if name != "" && target != "" {
				*result = append(*result, syntaxFact("decorator", name, target, current, lines, .9))
			}
			return
		}
		for i := 0; i < current.ChildCount(); i++ {
			child := current.ChildAt(i)
			walk(child)
		}
	}
	walk(node)
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

// extractJSLexicalVariables matches Superopen extract_js_vars: skip function
// assignments and plain require() bindings, but keep destructured require names.
func extractJSLexicalVariables(node syntaxView, source []byte, scope string, lines []uint32, local bool, variableDepth int, enclosedByVariable bool) []SyntaxFact {
	var facts []SyntaxFact
	for i := 0; i < node.ChildCount(); i++ {
		child := node.ChildAt(i)
		if !child.IsNamed() || child.Kind() != "variable_declarator" {
			continue
		}
		value, hasValue := findField(child, "value")
		if hasValue {
			switch value.Kind() {
			case "arrow_function", "function_expression", "generator_function":
				continue
			}
		}
		nameNode, ok := findField(child, "name")
		if !ok {
			continue
		}
		isRequire := hasValue && isRequireImportCall(value, source)
		switch nameNode.Kind() {
		case "object_pattern", "array_pattern":
			for _, binding := range destructurePatternNames(nameNode, source) {
				fact := syntaxFact("variable", binding, scope, child, lines, 1)
				fact.Local, fact.EnclosedByVariable, fact.VariableDepth = local, enclosedByVariable, variableDepth
				fact.IsExported = pinnedIsExported(binding, "javascript")
				facts = append(facts, fact)
			}
		default:
			if isRequire {
				continue
			}
			name := strings.TrimSpace(nodeText(nameNode, source))
			if name == "" {
				continue
			}
			fact := syntaxFact("variable", name, scope, child, lines, 1)
			fact.Local, fact.EnclosedByVariable, fact.VariableDepth = local, enclosedByVariable, variableDepth
			fact.IsExported = pinnedIsExported(name, "javascript")
			facts = append(facts, fact)
		}
	}
	return facts
}

func isRequireImportCall(node syntaxView, source []byte) bool {
	if node.Kind() != "call_expression" {
		return false
	}
	fn, ok := findField(node, "function")
	if !ok || fn.Kind() != "identifier" || strings.TrimSpace(nodeText(fn, source)) != "require" {
		return false
	}
	args, ok := findField(node, "arguments")
	if !ok {
		return false
	}
	for i := 0; i < args.ChildCount(); i++ {
		child := args.ChildAt(i)
		if !child.IsNamed() {
			continue
		}
		if child.Kind() == "string" || child.Kind() == "string_literal" || child.Kind() == "template_string" {
			return true
		}
	}
	return false
}

// syntaxCommonJSRequireFact mirrors process_commonjs_require: emit one IMPORT
// for require("path"), preferring the enclosing declarator identifier as local_name.
func syntaxCommonJSRequireFact(node, parent syntaxView, source []byte, scope string, lines []uint32) (SyntaxFact, bool) {
	if !isRequireImportCall(node, source) {
		return SyntaxFact{}, false
	}
	args, _ := findField(node, "arguments")
	modulePath := ""
	for i := 0; i < args.ChildCount(); i++ {
		child := args.ChildAt(i)
		if !child.IsNamed() {
			continue
		}
		if child.Kind() == "string" || child.Kind() == "string_literal" || child.Kind() == "template_string" {
			modulePath = trimSyntaxLiteral(strings.TrimSpace(nodeText(child, source)))
			break
		}
	}
	if modulePath == "" {
		return SyntaxFact{}, false
	}
	localName := importPathLast(modulePath)
	if parent.Kind() == "variable_declarator" {
		if nameNode, ok := findField(parent, "name"); ok && isIdentifierNode(nameNode.Kind()) {
			if name := strings.TrimSpace(nodeText(nameNode, source)); name != "" {
				localName = name
			}
		}
	}
	fact := syntaxFact("import", modulePath, scope, node, lines, 1)
	fact.LocalName = localName
	return fact, true
}

func destructurePatternNames(pattern syntaxView, source []byte) []string {
	var names []string
	for i := 0; i < pattern.ChildCount(); i++ {
		child := pattern.ChildAt(i)
		if !child.IsNamed() {
			continue
		}
		ident := destructureIdent(child)
		if ident.Kind() == "" {
			continue
		}
		if name := strings.TrimSpace(nodeText(ident, source)); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func destructureIdent(node syntaxView) syntaxView {
	switch node.Kind() {
	case "shorthand_property_identifier_pattern", "identifier":
		return node
	case "pair_pattern":
		if value, ok := findField(node, "value"); ok {
			return value
		}
	}
	for i := 0; i < node.ChildCount(); i++ {
		child := node.ChildAt(i)
		if child.IsNamed() {
			return child
		}
	}
	return SyntaxNode{}
}

func isEnumDeclarationNode(kind string) bool {
	return kind == "enum_declaration" || kind == "enum_specifier" || kind == "enum_item"
}

func isEnumMemberNode(kind string) bool {
	return kind == "enum_member_declaration" || kind == "enum_constant" ||
		kind == "enum_member" || kind == "enum_assignment" || kind == "enumerator"
}

func extractEnumMemberFacts(node syntaxView, source []byte, scope string, lines []uint32) []SyntaxFact {
	body := findEnumBody(node)
	if body.Kind() == "" {
		return nil
	}
	var facts []SyntaxFact
	for i := 0; i < body.ChildCount(); i++ {
		member := body.ChildAt(i)
		if !member.IsNamed() || !isEnumMemberNode(member.Kind()) {
			continue
		}
		nameNode, ok := findField(member, "name")
		if !ok {
			nameNode = findChildByType(member, "identifier")
		}
		if nameNode.Kind() == "" {
			continue
		}
		name := strings.TrimSpace(nodeText(nameNode, source))
		if name == "" {
			continue
		}
		fact := syntaxFact("variable", name, scope, member, lines, 1)
		facts = append(facts, fact)
	}
	return facts
}

func findEnumBody(node syntaxView) syntaxView {
	if body, ok := findField(node, "body"); ok {
		return body
	}
	for _, kind := range []string{"enum_body", "declaration_list", "field_declaration_list", "class_body"} {
		if child := findChildByType(node, kind); child.Kind() != "" {
			return child
		}
	}
	return SyntaxNode{}
}

func findChildByType(node syntaxView, kind string) syntaxView {
	for i := 0; i < node.ChildCount(); i++ {
		child := node.ChildAt(i)
		if child.Kind() == kind {
			return child
		}
	}
	return SyntaxNode{}
}

func yamlInfraURLRoute(node syntaxView, source []byte, key string, lines []uint32) SyntaxFact {
	if key == "" || isUpstreamConfigKey(key) {
		return SyntaxFact{}
	}
	value, ok := findField(node, "value")
	if !ok {
		return SyntaxFact{}
	}
	text := strings.TrimSpace(nodeText(value, source))
	text = strings.Trim(text, `"'`)
	if text == "" || strings.Contains(text, " ") || !strings.Contains(text, "://") {
		return SyntaxFact{}
	}
	fact := syntaxFact("infra_url", text, key, node, lines, 1)
	fact.FirstStringArg = key
	return fact
}

func isUpstreamConfigKey(keyPath string) bool {
	lower := strings.ToLower(keyPath)
	for _, deny := range []string{"jwks", "registry", "registries", "healthcheck", "engine", "_service_url", "auth"} {
		if strings.Contains(lower, deny) {
			return true
		}
	}
	return false
}

// findIniSectionName matches Superopen find_ini_section_name: prefer the bare
// text child inside section_name so brackets/newlines never enter the QN.
func findIniSectionName(node syntaxView, source []byte) string {
	for i := 0; i < node.ChildCount(); i++ {
		child := node.ChildAt(i)
		if child.Kind() != "section_name" {
			continue
		}
		for i := 0; i < child.ChildCount(); i++ {
			inner := child.ChildAt(i)
			if inner.Kind() == "text" {
				if name := strings.TrimSpace(nodeText(inner, source)); name != "" {
					return name
				}
			}
		}
		if name := strings.TrimSpace(nodeText(child, source)); name != "" {
			return strings.Trim(name, "[] \t\r\n")
		}
	}
	return ""
}

// findIniSettingName matches Superopen extract_ini_vars.
func findIniSettingName(node syntaxView, source []byte) string {
	for i := 0; i < node.ChildCount(); i++ {
		child := node.ChildAt(i)
		if child.Kind() == "setting_name" || child.Kind() == "name" {
			return strings.TrimSpace(nodeText(child, source))
		}
	}
	if node.ChildCount() > 0 {
		return strings.TrimSpace(nodeText(node.ChildAt(0), source))
	}
	return ""
}

func definitionName(language string, node syntaxView, source []byte, kind string) string {
	if language == "ini" {
		if kind == "class" && node.Kind() == "section" {
			return findIniSectionName(node, source)
		}
		if kind == "variable" && node.Kind() == "setting" {
			return findIniSettingName(node, source)
		}
	}
	if kind == "function" && node.Kind() == "rule" {
		line := nodeText(node, source)
		if end := strings.IndexAny(line, ":\r\n"); end >= 0 {
			line = line[:end]
		}
		if fields := strings.Fields(line); len(fields) > 0 {
			if strings.HasPrefix(fields[0], ".") {
				return ""
			}
			return fields[0]
		}
	}
	if kind == "function" {
		// Bash and PowerShell expose function names as bare grammar-specific
		// children rather than identifier/name fields. Match the pinned
		// extractor by reading the declaration token for those forms.
		if name := commandLanguageFunctionName(node.Kind(), nodeText(node, source)); name != "" {
			return name
		}
	}
	if language == "python" && kind == "variable" {
		if left, ok := findField(node, "left"); ok && (left.Kind() == "attribute" || left.Kind() == "subscript") {
			return ""
		}
	}
	for _, field := range []string{"name", "declarator", "pattern", "left", "key"} {
		if child, ok := findField(node, field); ok {
			// Superopen engine helper keeps the full computed_property_name
			// text (e.g. [Symbol.asyncIterator]), not the nested identifier.
			if child.Kind() == "computed_property_name" {
				if name := strings.TrimSpace(nodeText(child, source)); name != "" {
					return name
				}
			}
			if name := identifierText(child, source); name != "" {
				return name
			}
			if kind == "variable" || kind == "field" {
				if name := trimSyntaxLiteral(nodeText(child, source)); name != "" && !strings.ContainsAny(name, "\r\n{}[]") {
					return name
				}
			}
		}
	}
	if kind == "variable" || kind == "field" {
		for i := 0; i < node.ChildCount(); i++ {
			child := node.ChildAt(i)
			if name := identifierText(child, source); name != "" {
				return name
			}
		}
	}
	return ""
}

func commandLanguageFunctionName(nodeType, text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if nodeType == "function_statement" {
		fields := strings.Fields(text)
		if len(fields) < 2 {
			return ""
		}
		switch strings.ToLower(fields[0]) {
		case "function", "filter", "workflow":
			name := fields[1]
			if end := strings.IndexAny(name, "({"); end >= 0 {
				name = name[:end]
			}
			return name
		default:
			return ""
		}
	}
	if nodeType != "function_definition" {
		return ""
	}
	if strings.HasPrefix(text, "function ") {
		text = strings.TrimSpace(strings.TrimPrefix(text, "function "))
	}
	open := strings.Index(text, "()")
	if open <= 0 {
		return ""
	}
	name := strings.TrimSpace(text[:open])
	if name == "" || strings.ContainsAny(name, " \t\r\n{}[]") {
		return ""
	}
	return name
}

func relationshipName(node syntaxView, source []byte, fields []string) string {
	for _, field := range fields {
		if child, ok := findField(node, field); ok {
			if text := compactRelationshipText(nodeText(child, source)); text != "" {
				return text
			}
		}
	}
	for i := 0; i < node.ChildCount(); i++ {
		child := node.ChildAt(i)
		if isIdentifierNode(child.Kind()) || isStringNode(child.Kind()) || strings.Contains(child.Kind(), "member") || strings.Contains(child.Kind(), "selector") {
			if text := compactRelationshipText(nodeText(child, source)); text != "" {
				return text
			}
		}
	}
	return ""
}

func findField(node syntaxView, field string) (syntaxView, bool) {
	if node == nil {
		return SyntaxNode{}, false
	}
	if lookup, ok := node.(fieldLookupView); ok {
		return lookup.ChildByField(field)
	}
	var found syntaxView
	ok := false
	viewEachChild(node, func(child syntaxView) {
		if ok || child.FieldName() != field {
			return
		}
		found = child
		ok = true
	})
	if !ok {
		return SyntaxNode{}, false
	}
	return found, true
}

func identifierText(node syntaxView, source []byte) string {
	if isIdentifierNode(node.Kind()) || isStringNode(node.Kind()) {
		return trimSyntaxLiteral(nodeText(node, source))
	}
	for i := 0; i < node.ChildCount(); i++ {
		child := node.ChildAt(i)
		if child.FieldName() == "parameters" || child.FieldName() == "body" || child.Kind() == "parameter_list" || strings.Contains(child.Kind(), "body") {
			continue
		}
		if value := identifierText(child, source); value != "" {
			return value
		}
	}
	return ""
}

func isIdentifierNode(kind string) bool {
	return kind == "identifier" || kind == "type_identifier" || kind == "field_identifier" ||
		kind == "property_identifier" || kind == "namespace_identifier" || kind == "constant" ||
		kind == "name" || strings.HasSuffix(kind, "_identifier")
}

func isStringNode(kind string) bool {
	return kind == "string" || kind == "string_literal" || kind == "interpreted_string_literal" ||
		strings.HasSuffix(kind, "_string")
}

func nodeText(node syntaxView, source []byte) string {
	if node == nil || node.StartByte() > node.EndByte() || uint64(node.EndByte()) > uint64(len(source)) {
		return ""
	}
	return string(source[node.StartByte():node.EndByte()])
}

func trimSyntaxLiteral(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "`\"'")
	return strings.TrimSpace(value)
}

func compactRelationshipText(value string) string {
	value = trimSyntaxLiteral(value)
	if index := strings.IndexAny(value, "\r\n;{"); index >= 0 {
		value = value[:index]
	}
	return strings.TrimSpace(value)
}

func sourceLineIndex(source []byte) []uint32 {
	lines := []uint32{0}
	for index, value := range source {
		if value == '\n' {
			lines = append(lines, uint32(index+1))
		}
	}
	return lines
}

func syntaxFact(kind, name, scope string, node syntaxView, lines []uint32, confidence float64) SyntaxFact {
	startLine, startColumn := bytePosition(lines, node.StartByte())
	endLine, endColumn := bytePosition(lines, node.EndByte())
	return SyntaxFact{
		Kind: kind, Name: internString(name), Scope: internString(scope), NodeType: internString(node.Kind()), StartByte: node.StartByte(), EndByte: node.EndByte(),
		StartLine: startLine, StartColumn: startColumn, EndLine: endLine, EndColumn: endColumn, Confidence: confidence,
		Lines: endLine - startLine + 1,
	}
}

func bytePosition(lines []uint32, offset uint32) (int, int) {
	index := sort.Search(len(lines), func(index int) bool { return lines[index] > offset }) - 1
	if index < 0 {
		index = 0
	}
	return index + 1, int(offset-lines[index]) + 1
}

func sortSyntaxFacts(result *SyntaxExtraction) {
	less := func(values []SyntaxFact) {
		sort.SliceStable(values, func(i, j int) bool {
			if values[i].StartByte != values[j].StartByte {
				return values[i].StartByte < values[j].StartByte
			}
			if values[i].Kind != values[j].Kind {
				return values[i].Kind < values[j].Kind
			}
			return values[i].Name < values[j].Name
		})
	}
	less(result.Definitions)
	less(result.Calls)
	less(result.Imports)
	less(result.Decorators)
	less(result.Sections)
	less(result.Branches)
	less(result.Inheritance)
	sortOccurrenceFacts(result.Usages)
	sortOccurrenceFacts(result.Writes)
	sortOccurrenceFacts(result.Bindings)
}
