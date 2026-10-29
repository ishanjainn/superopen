package engine

import (
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// symbolRegistry is the Go representation of Superopen's observable registry
// chain: import map, same module, unique name, then proximity-ranked suffix.
// It intentionally resolves only published definition labels.
type symbolRegistry struct {
	exact          map[string]api.Node
	byName         map[string][]api.Node
	projectQN      string
	packageExports map[string]string
	decorated map[string]bool
}

// guessedEdgeConfidence caps an edge the resolver could only guess from a
// shared name in a shape where that name rarely means the matched method.
// It sits below traceMinConfidence: trace and query skip such edges unless
// asked, impact still sees them.
const guessedEdgeConfidence = 0.2

// pythonBuiltinTypeMethods are the methods of Python's dict, list, set,
// str, bytes and tuple types.
var pythonBuiltinTypeMethods = func() map[string]bool {
	out := map[string]bool{}
	for _, name := range strings.Fields(`clear copy fromkeys get items keys pop popitem setdefault update values
		append count extend index insert remove reverse sort
		add difference difference_update discard intersection intersection_update isdisjoint issubset issuperset
		symmetric_difference symmetric_difference_update union
		capitalize casefold center encode decode endswith expandtabs find format format_map isalnum isalpha
		isascii isdecimal isdigit isidentifier islower isnumeric isprintable isspace istitle isupper join ljust
		lower lstrip partition removeprefix removesuffix replace rfind rindex rjust rpartition rsplit rstrip split
		splitlines startswith strip swapcase title translate upper zfill hex`) {
		out[name] = true
	}
	return out
}()

// resolveOwnMember resolves self.name / cls.name (Python) and this.name
// (JavaScript, TypeScript) through the enclosing class: its own member
// first, then the nearest base class that defines one, walking the base
// names each class declares. self is set when the callee has such a
// receiver; a miss then means any name match elsewhere is only a guess.
func (registry symbolRegistry) resolveOwnMember(language, callee, source string) (resolution symbolResolution, self bool) {
	receiver, member, dotted := strings.Cut(callee, ".")
	if !dotted || member == "" || strings.ContainsAny(member, ".:") {
		return symbolResolution{}, false
	}
	parent := false
	switch {
	case language == "python" && (receiver == "self" || receiver == "cls"):
	case (isJSLanguage(language) || language == "jsx") && receiver == "this":
	case language == "python" && strings.HasPrefix(receiver, "super("),
		(isJSLanguage(language) || language == "jsx") && receiver == "super":
		// super().name skips the class's own definition.
		parent = true
	default:
		return symbolResolution{}, false
	}
	class := registry.enclosingClass(source)
	if class == "" {
		return symbolResolution{}, false
	}
	visited := map[string]bool{class: true}
	level := []string{class}
	if parent {
		level = nil
		for _, base := range registry.baseClasses(class) {
			if !visited[base] {
				visited[base] = true
				level = append(level, base)
			}
		}
	}
	for depth := 0; depth < 8 && len(level) > 0; depth++ {
		var next []string
		for _, qn := range level {
			if _, ok := registry.exact[qn+"."+member]; ok {
				if depth == 0 && !parent {
					return symbolResolution{qn: qn + "." + member, strategy: "own_member", confidence: .9, candidates: 1}, true
				}
				return symbolResolution{qn: qn + "." + member, strategy: "inherited_member", confidence: .8, candidates: 1}, true
			}
			for _, base := range registry.baseClasses(qn) {
				if !visited[base] {
					visited[base] = true
					next = append(next, base)
				}
			}
		}
		level = next
	}
	return symbolResolution{}, true
}

// classMember is true for a definition directly inside a class.
func (registry symbolRegistry) classMember(qn string) bool {
	parent, ok := registry.exact[packagePrefix(qn)]
	return ok && parent.Label == "Class"
}

// enclosingClass is the class a definition sits in, looking past nested
// functions (a closure inside a method), or "" at module level.
func (registry symbolRegistry) enclosingClass(source string) string {
	for qn, step := packagePrefix(source), 0; qn != source && step < 4; qn, step = packagePrefix(qn), step+1 {
		if node, ok := registry.exact[qn]; ok && node.Label == "Class" {
			return qn
		}
		source = qn
	}
	return ""
}

// baseClasses resolves a class's declared base names to class definitions
// by their last segment, nearest module first among same-named classes.
func (registry symbolRegistry) baseClasses(class string) []string {
	node, ok := registry.exact[class]
	if !ok {
		return nil
	}
	var names []string
	switch bases := node.Properties["base_classes"].(type) {
	case []string:
		names = bases
	case []any:
		for _, base := range bases {
			if name, ok := base.(string); ok {
				names = append(names, name)
			}
		}
	}
	var out []string
	for _, name := range names {
		name = strings.TrimSpace(name)
		if cut := strings.IndexAny(name, "[(<"); cut >= 0 {
			name = name[:cut]
		}
		simple := simpleRegistryName(name)
		for _, candidate := range registry.byName[simple] {
			if candidate.Label == "Class" && candidate.QualifiedName != class {
				out = append(out, candidate.QualifiedName)
			}
		}
	}
	return out
}

// guessStrategy is true for a resolution made from the name alone.
func guessStrategy(strategy string) bool {
	switch strategy {
	case "unique_name", "suffix_match", "field_type_hint", "fuzzy":
		return true
	default:
		return false
	}
}

// receiverGuessIsWeak is true when a name-only resolution of a Python
// attribute access on a non-imported receiver is unlikely to be right. A
// read or write (not a call) of obj.select needs receiver type evidence even
// when the target is an accessor; obj.f() names a module function only through an
// imported module; obj.values() is most often a builtin type's method.
// self and cls receivers resolve through the class hierarchy instead.
func (registry symbolRegistry) receiverGuessIsWeak(language, callee string, imports map[string]string, resolution symbolResolution, call bool) bool {
	if language != "python" || !guessStrategy(resolution.strategy) {
		return false
	}
	receiver, _, dotted := strings.Cut(callee, ".")
	if !dotted {
		return false
	}
	target, ok := registry.exact[resolution.qn]
	if !ok {
		return false
	}
	if imports[receiver] != "" {
		return false
	}
	if !registry.classMember(resolution.qn) {
		// obj.f() or obj.name reaches a module-level definition only when
		// obj is a module, and a module receiver comes from an import,
		// which the import map resolves before any guess.
		return true
	}
	switch target.Label {
	case "Method":
		if !call {
			return true
		}
		// x.values() on a receiver of unknown type is most often the dict,
		// list, set or str method of that name, not a project method.
		return pythonBuiltinTypeMethods[callee[strings.LastIndexByte(callee, '.')+1:]]
	}
	return false
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
func (registry symbolRegistry) applyFieldTypeHint(callee, source, sourceFile string, resolution symbolResolution) symbolResolution {
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
		if sourceFile != "" && !isTestPath(sourceFile) && isTestPath(candidate.Location.File) {
			continue
		}
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
		if node.Label == "Project" && registry.projectQN == "" {
			registry.projectQN = node.QualifiedName
		}
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
	session := newResolveSession(module, imports)
	return registry.resolveWith(session, callee)
}

type resolveSession struct {
	module  string
	imports map[string]string
	results map[string]symbolResolution
	reach   map[string]bool
	// receiverMethods is set for languages where a method is reached only
	// through a receiver (self.save(), obj.Save()). A bare name there is a
	// local, a function or a builtin, never a method of some other class.
	receiverMethods bool
	// sourceIsProduction is set for a file outside the test suite. Its name
	// guesses skip test-file definitions: library code imports from tests
	// only explicitly, and that resolves through the import map.
	sourceIsProduction bool
	// family is the source file's language family. A name guess never
	// crosses families: a TypeScript .length is not a Python function.
	family string
	// external holds local names bound by imports outside the project
	// (import os, from os.path import join). They are never guessed into
	// project symbols.
	external map[string]bool
}

// languageFamily groups languages whose code calls each other's symbols
// directly. Anything else is its own family.
func languageFamily(language string) string {
	switch language {
	case "javascript", "typescript", "tsx", "jsx", "vue", "svelte":
		return "js"
	case "c", "cpp", "cuda", "objc", "swift":
		return "c"
	case "java", "kotlin", "scala", "groovy":
		return "jvm"
	case "c_sharp", "fsharp", "vb":
		return "dotnet"
	default:
		return language
	}
}

// pathLanguageFamily is the family of a file by its extension, or "" for a
// path no language claims (synthetic builtin files, folders).
func pathLanguageFamily(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return ""
	}
	return languageFamily(extensionLanguages[ext])
}

// receiverMethodLanguage is true where calling a method needs an explicit
// receiver. Java, C#, C++, Kotlin, Swift, Scala and Ruby call methods of
// the enclosing class by bare name, so they are left out.
func receiverMethodLanguage(language string) bool {
	switch language {
	case "python", "go", "javascript", "typescript", "tsx", "jsx", "rust", "php", "lua":
		return true
	default:
		return false
	}
}

func newResolveSession(module string, imports map[string]string) *resolveSession {
	return &resolveSession{
		module:  module,
		imports: imports,
		results: map[string]symbolResolution{},
		reach:   map[string]bool{},
	}
}

func (registry symbolRegistry) resolveWith(session *resolveSession, callee string) symbolResolution {
	if session != nil {
		if cached, ok := session.results[callee]; ok {
			return cached
		}
	}
	resolution := registry.resolveUncached(callee, session)
	if session != nil {
		session.results[callee] = resolution
	}
	return resolution
}

func (registry symbolRegistry) resolveUncached(callee string, session *resolveSession) symbolResolution {
	module := ""
	var imports map[string]string
	if session != nil {
		module = session.module
		imports = session.imports
	}
	prefix, suffix := splitRegistryCallee(callee)
	if session != nil && session.external[prefix] {
		if node, ok := registry.exact[joinSyntaxScope(module, callee)]; ok && jsxCallableLabel(node.Label) {
			return symbolResolution{qn: node.QualifiedName, strategy: "same_module", confidence: .9, candidates: 1}
		}
		return symbolResolution{}
	}
	if resolved := imports[prefix]; resolved != "" {
		if resolved == registry.projectQN {
			rootSymbol := suffix
			if rootSymbol == "" {
				rootSymbol = prefix
			}
			if node, ok := registry.exact[rootSymbol]; ok {
				return symbolResolution{qn: node.QualifiedName, strategy: "import_map", confidence: .95, candidates: 1}
			}
		}
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
			packageMember := resolved + ".__init__." + suffix
			if node, ok := registry.exact[packageMember]; ok && jsxCallableLabel(node.Label) {
				return symbolResolution{qn: node.QualifiedName, strategy: "import_map", confidence: .95, candidates: 1}
			}
			if exported := registry.packageExports[resolved+"."+suffix]; exported != "" {
				return symbolResolution{qn: exported, strategy: "import_map", confidence: .95, candidates: 1}
			}
		}
		if suffix == "" && !strings.HasSuffix(resolved, "."+prefix) {
			// The import bound a package or module rather than the name
			// (from pkg import Name, re-exported by pkg/__init__): look for
			// the one definition of that name under the package.
			match := ""
			for _, node := range registry.byName[prefix] {
				if strings.HasPrefix(node.QualifiedName, resolved+".") {
					if match != "" {
						match = ""
						break
					}
					match = node.QualifiedName
				}
			}
			if match != "" {
				return symbolResolution{qn: match, strategy: "import_map_suffix", confidence: .85, candidates: 1}
			}
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
		candidate := joinSyntaxScope(module, name)
		if _, ok := registry.exact[candidate]; ok {
			return symbolResolution{qn: candidate, strategy: "same_module", confidence: .9, candidates: 1}
		}
	}
	candidates := registryGuessCandidates(session, registry.byName[simpleRegistryName(callee)], suffix == "")
	if len(candidates) == 0 || len(candidates) > 256 {
		return symbolResolution{}
	}
	if len(candidates) == 1 {
		confidence := .75
		if len(imports) > 0 && !registryImportReachableCached(candidates[0].QualifiedName, imports, session) {
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
			if registryImportReachableCached(candidate.QualifiedName, imports, session) {
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

// registryGuessCandidates drops the definitions a name guess cannot mean
// from this session's file: methods for a bare name in a receiver language,
// and test-file definitions for production code.
func registryGuessCandidates(session *resolveSession, candidates []api.Node, bare bool) []api.Node {
	if session == nil {
		return candidates
	}
	dropMethods := session.receiverMethods && bare
	if !dropMethods && !session.sourceIsProduction && session.family == "" {
		return candidates
	}
	kept := candidates[:0:0]
	for _, candidate := range candidates {
		if dropMethods && candidate.Label == "Method" {
			continue
		}
		if session.sourceIsProduction && isTestPath(candidate.Location.File) {
			continue
		}
		if family := pathLanguageFamily(candidate.Location.File); session.family != "" && family != "" && family != session.family {
			continue
		}
		kept = append(kept, candidate)
	}
	return kept
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

func registryImportReachableCached(candidate string, imports map[string]string, session *resolveSession) bool {
	if session != nil {
		if cached, ok := session.reach[candidate]; ok {
			return cached
		}
	}
	reachable := registryImportReachable(candidate, imports)
	if session != nil {
		session.reach[candidate] = reachable
	}
	return reachable
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
	return bytesContainAny(qn, []string{"Test", "test", "Mock", "mock", "Stub", "stub", "Fake", "fake", "Fixture", "spec"})
}

func bytesContainAny(value string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func registryCommonPrefix(left, right string) int {
	count := 0
	leftAt, rightAt := 0, 0
	for {
		leftSeg, leftNext := scanDottedSegment(left, leftAt)
		rightSeg, rightNext := scanDottedSegment(right, rightAt)
		if leftSeg != rightSeg {
			return count
		}
		count++
		if leftNext < 0 || rightNext < 0 {
			return count
		}
		leftAt, rightAt = leftNext, rightNext
	}
}

func scanDottedSegment(value string, start int) (string, int) {
	if start > len(value) {
		return "", -1
	}
	rest := value[start:]
	index := strings.IndexByte(rest, '.')
	if index < 0 {
		return rest, -1
	}
	return rest[:index], start + index + 1
}

func registryCandidatePenalty(base float64, count int) float64 {
	if count <= 3 {
		return base
	}
	return base * (3 / float64(count))
}
