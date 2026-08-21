package engine

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

type syntaxDefinitionRef struct {
	file, scope, name, qn, label string
}

// IndexSyntaxDevelopment runs the complete generic discovery/parse/assemble/
// atomic-publication spine. It is intentionally not selected by Server until
// embedded assets and every family resolver pass their readiness gates.
func IndexSyntaxDevelopment(ctx context.Context, request api.BuildRequest, engineVersion string, parser SyntaxParser, workers int) (api.BuildResult, error) {
	started := time.Now()
	root, err := CanonicalRoot(request.RepoRoot)
	if err != nil {
		return api.BuildResult{}, err
	}
	project := request.Project
	if project == "" {
		project, err = ProjectName(root)
		if err != nil {
			return api.BuildResult{}, err
		}
	}
	var changes *api.ChangeSet
	if request.Incremental {
		planned, err := PlanIncremental(ctx, root, project, request.Exclude)
		if err != nil {
			return api.BuildResult{}, err
		}
		changes = &planned
	}
	files, err := discoverTrackedFiles(ctx, root, request.Exclude)
	if err != nil {
		return api.BuildResult{}, err
	}
	applyMemoryBudget()
	repository, err := ParseSyntaxRepository(ctx, parser, root, project, files, nil, workers)
	if err != nil {
		return api.BuildResult{}, err
	}
	graph, coverage := AssembleSyntaxGraph(repository, project)
	dropExtractedOccurrences(repository.Files)
	indexRepositoryBranch(ctx, root, project, &graph)
	indexPythonBuiltins(project, repository.Files, &graph)
	indexEnvironmentAccesses(project, root, repository.Files, &graph)
	indexHTTPRoutes(project, repository.Files, &graph)
	repository.Files = nil
	separateUnresolvedRelationships(&graph)
	revision := gitRevision(ctx, root)
	if request.ExpectedSource != "" && request.ExpectedSource != revision {
		return api.BuildResult{}, fmt.Errorf("source revision changed: expected %s, found %s", request.ExpectedSource, revision)
	}
	database, err := Publish(ctx, root, func(ctx context.Context, path string) error {
		store, err := OpenWritableFresh(path)
		if err != nil {
			return err
		}
		buildErr := store.BuildFresh(ctx, func(builder *Builder) error {
			if err := builder.PutProject(ProjectRecord{Name: project, RootPath: root, Generation: repository.Generation,
				SourceRevision: revision, EngineVersion: engineVersion, IndexedAt: time.Now().UTC()}); err != nil {
				return err
			}
			for _, file := range graph.files {
				if err := builder.PutFile(file); err != nil {
					return err
				}
			}
			ids := make(map[string]int64, len(graph.nodes))
			for i := range graph.nodes {
				node := &graph.nodes[i]
				id, err := builder.PutNode(*node)
				if err != nil {
					return err
				}
				ids[node.QualifiedName] = id
			}
			if err := builder.flushNodeBatch(); err != nil {
				return err
			}
			if err := putGraphSemantics(builder, graph, ids, project, nil); err != nil {
				return err
			}
			for i := range graph.nodes {
				graph.nodes[i].Properties = nil
			}
			for _, edge := range graph.edges {
				source, sourceOK := ids[edge.source]
				target, targetOK := ids[edge.target]
				if !sourceOK || !targetOK {
					return fmt.Errorf("syntax edge endpoint missing: %s -[%s]-> %s", edge.source, edge.kind, edge.target)
				}
				if _, err := builder.PutEdge(api.Edge{Project: project, SourceID: source, TargetID: target,
					Type: edge.kind, Properties: edge.dumpProperties(), Evidence: edge.evidence}); err != nil {
					return err
				}
			}
			for _, edge := range graph.unresolved {
				if err := builder.PutUnresolved(api.UnresolvedRelationship{Project: project, Source: edge.source,
					TargetText: edge.target, Type: edge.kind, Properties: edge.dumpProperties(), Evidence: edge.evidence}); err != nil {
					return err
				}
			}
			return builder.PutCoverage(project, coverage)
		})
		if buildErr == nil {
			buildErr = store.Seal(ctx)
		}
		if closeErr := store.Close(); buildErr == nil {
			buildErr = closeErr
		}
		return buildErr
	})
	if err != nil {
		return api.BuildResult{}, err
	}
	nodeCount, edgeCount, err := publishedCounts(ctx, database, project)
	if err != nil {
		return api.BuildResult{}, err
	}
	return api.BuildResult{Status: "ok", Project: project, Database: database,
		SourceRevision: revision, Generation: repository.Generation, NodeCount: nodeCount, EdgeCount: edgeCount,
		FileCount: len(graph.files), Duration: time.Since(started), Coverage: summarizedCoverage(coverage, 100), Changes: changes}, nil
}

// AssembleSyntaxGraph turns language-neutral Tree-sitter facts into the common
// graph schema. Family semantic resolvers may replace its conservative
// CALL_REFERENCE fallbacks, but the output is already deterministic and fully
// source-grounded, making it useful for pass-by-pass golden comparison.
func AssembleSyntaxGraph(repository SyntaxRepository, project string) (goGraph, api.Coverage) {
	graph := goGraph{files: make([]FileRecord, 0, len(repository.Files))}
	coverage := repository.Coverage
	projectQN := project
	graph.nodes = append(graph.nodes, api.Node{Project: project, Label: "Project", Name: project, QualifiedName: projectQN,
		Location: api.Location{File: "{}"}, Properties: api.Properties{}})
	folders := map[string]bool{}
	byName := map[string][]syntaxDefinitionRef{}
	byFileScope := map[string]syntaxDefinitionRef{}
	typeDefinitions := map[string]bool{}
	localBindings := map[string]map[string]bool{}

	for _, parsed := range repository.Files {
		graph.files = append(graph.files, parsed.File)
		rel := filepath.ToSlash(parsed.File.Path)
		fileQN := fileQualifiedName(rel)
		graph.nodes = append(graph.nodes, api.Node{
			Project: project, Label: "File", Name: filepath.Base(rel), QualifiedName: fileQN,
			Location: api.Location{File: rel}, Properties: api.Properties{"extension": filepath.Ext(filepath.Base(rel))},
		})
		directory := filepath.ToSlash(filepath.Dir(rel))
		if directory != "." && directory != "" {
			// pass_structure creates a new chain bottom-up. Its parent lookup is
			// performed before the next ancestor is created, so the first-seen
			// deepest directory may temporarily have no CONTAINS_FOLDER edge.
			// Later files repair it only if they independently traverse that path.
			// Preserve that observable insertion-order behavior.
			for walk := directory; walk != "" && !folders[walk]; {
				folders[walk] = true
				folderQN := folderQualifiedName(walk)
				graph.nodes = append(graph.nodes, api.Node{Project: project, Label: "Folder", Name: filepath.Base(walk),
					QualifiedName: folderQN, Location: api.Location{File: walk}, Properties: api.Properties{}})
				parentDirectory := filepath.ToSlash(filepath.Dir(walk))
				if parentDirectory == "." {
					parentDirectory = ""
				}
				parentQN := projectQN
				parentExists := parentDirectory == ""
				if parentDirectory != "" {
					parentQN = folderQualifiedName(parentDirectory)
					parentExists = folders[parentDirectory]
				}
				if parentExists {
					graph.edges = append(graph.edges, pendingEdge{source: parentQN, target: folderQN, kind: "CONTAINS_FOLDER", evidence: layoutEvidence(walk)})
				}
				walk = parentDirectory
			}
		}
		parent := projectQN
		if directory != "." && directory != "" {
			parent = folderQualifiedName(directory)
		}
		graph.edges = append(graph.edges, pendingEdge{source: parent, target: fileQN, kind: "CONTAINS_FILE", evidence: layoutEvidence(rel)})
		// Java and Go derive their module identity from the containing
		// directory. The structure pass has already published that identity as
		// the project/folder node, so the extracted module definition collides
		// and is not separately observable Superopen.
		if parsed.Extraction.RootModule && (parsed.File.Language == "go" || parsed.File.Language == "java") {
			moduleQN := syntaxDefinitionModuleQN(parsed.File.Language, rel)
			if moduleQN == "" {
				moduleQN = project
			}
			graph.edges = append(graph.edges, pendingEdge{source: fileQN, target: moduleQN, kind: "DEFINES",
				evidence: layoutEvidence(rel)})
		} else if parsed.Extraction.RootModule {
			moduleQN := syntaxModuleQN(rel)
			graph.nodes = append(graph.nodes, api.Node{
				Project: project, Label: "Module", Name: rel, QualifiedName: moduleQN,
				Location: api.Location{File: rel, StartLine: 1}, Properties: syntaxModuleProperties(rel),
			})
			graph.edges = append(graph.edges, pendingEdge{source: fileQN, target: moduleQN, kind: "DEFINES",
				evidence: layoutEvidence(rel)})
		}
		for _, fact := range parsed.Extraction.Sections {
			qn := syntaxModuleQN(rel) + "." + strings.Join(strings.Fields(fact.Name), "-")
			graph.nodes = append(graph.nodes, api.Node{Project: project, Label: "Section", Name: fact.Name,
				QualifiedName: qn, Location: syntaxLocation(rel, fact), Properties: syntaxModuleProperties(rel)})
			graph.edges = append(graph.edges, pendingEdge{source: fileQN, target: qn, kind: "DEFINES",
				evidence: syntaxEvidence(rel, fact, "tree_sitter_heading")})
		}

		pythonTopClasses := map[string]bool{}
		if parsed.File.Language == "python" {
			for _, fact := range parsed.Extraction.Definitions {
				if fact.Kind == "class" && !fact.Local {
					pythonTopClasses[fact.Name] = true
				}
			}
		}
		for _, fact := range parsed.Extraction.Definitions {
			if fact.Kind == "function" {
				scopeKey := rel + "\x00" + joinSyntaxScope(fact.Scope, fact.Name)
				if localBindings[scopeKey] == nil {
					localBindings[scopeKey] = map[string]bool{}
				}
				for _, parameter := range fact.ParamNames {
					localBindings[scopeKey][parameter] = true
				}
			} else if fact.Local && fact.Name != "" {
				scopeKey := rel + "\x00" + fact.Scope
				if localBindings[scopeKey] == nil {
					localBindings[scopeKey] = map[string]bool{}
				}
				localBindings[scopeKey][fact.Name] = true
			}
		}
		for _, fact := range parsed.Extraction.Definitions {
			// Go struct fields are resolver metadata (field_defs), not graph
			// definitions. The class extractor consumes the type_spec body and the
			// top-level definition walker never publishes these nodes.
			if parsed.File.Language == "go" && fact.Kind == "field" {
				continue
			}
			if parsed.File.Language == "go" && fact.Kind == "class" && fact.Local {
				continue
			}
			// The pinned Python resolver indexes module-level classes. Classes
			// declared inside a function are local implementation details rather
			// than independently addressable graph symbols.
			if parsed.File.Language == "python" && fact.Kind == "class" && fact.Local {
				continue
			}
			if parsed.File.Language == "python" && fact.Kind == "function" && fact.Local && !pythonTopClasses[fact.Scope] {
				continue
			}
			if parsed.File.Language == "python" && fact.Kind == "variable" && fact.Local {
				continue
			}
			// JS/TS: Superopen walk_defs still emits class_declaration nodes nested
			// inside callbacks (common in tests). Keep them addressable.
			if fact.Kind == "variable" && fact.Local && !dataLanguageVariables(parsed.File.Language) {
				continue
			}
			if parsed.File.Language == "yaml" && fact.Kind == "variable" && fact.VariableDepth > 0 {
				continue
			}
			base := syntaxDefinitionFactQN(parsed.File.Language, rel, fact)
			qn := base
			properties := syntaxDefinitionProperties(parsed.File.Language, rel, fact)
			if fact.StructuralProfile != "" {
				properties["sp"] = fact.StructuralProfile
			}
			if fact.MinHash != "" {
				properties["fp"] = fact.MinHash
			}
			label := syntaxNodeLabel(parsed.File.Language, fact)
			node := api.Node{Project: project, Label: label, Name: fact.Name,
				QualifiedName: qn, Location: syntaxLocation(rel, fact), Properties: properties}
			graph.nodes = append(graph.nodes, node)
			graph.edges = append(graph.edges, pendingEdge{source: fileQN, target: qn, kind: "DEFINES",
				evidence: syntaxEvidence(rel, fact, "tree_sitter_definition")})
			if fact.Kind == "function" {
				owner, ok := byFileScope[rel+"\x00"+fact.Scope]
				if parsed.File.Language == "go" && fact.ParentClass != "" {
					owner = syntaxDefinitionRef{qn: joinSyntaxScope(syntaxDefinitionModuleQN(parsed.File.Language, rel), fact.ParentClass)}
					ok = true
				}
				if ok && typeDefinitions[owner.qn] {
					graph.edges = append(graph.edges, pendingEdge{source: owner.qn, target: qn, kind: "DEFINES_METHOD",
						evidence: syntaxEvidence(rel, fact, "tree_sitter_parent_type")})
				}
			}
			if label == "Class" || label == "Interface" || label == "Struct" || label == "Trait" {
				typeDefinitions[qn] = true
			}
			ref := syntaxDefinitionRef{file: rel, scope: fact.Scope, name: fact.Name, qn: qn, label: label}
			key := parsed.File.Language + "\x00" + fact.Name
			byName[key] = append(byName[key], ref)
			byFileScope[rel+"\x00"+joinSyntaxScope(fact.Scope, fact.Name)] = ref
		}
		for _, write := range parsed.Extraction.Bindings {
			if write.Scope == "" || strings.ContainsAny(write.Name, ".:") {
				continue
			}
			scopeKey := rel + "\x00" + write.Scope
			if localBindings[scopeKey] == nil {
				localBindings[scopeKey] = map[string]bool{}
			}
			localBindings[scopeKey][write.Name] = true
		}
	}
	indexGoModDependencies(project, repository.Root, repository.Files, &graph)
	// The pinned builtin symbols must exist before resolution: Superopen keeps
	// one global registry, so any language's reference to a name such as `int`
	// resolves against them.
	seedPythonBuiltinNodes(project, repository.Files, &graph)
	registry := newSymbolRegistry(graph.nodes)
	nodeQNs := make(map[string]bool, len(graph.nodes))
	for _, node := range graph.nodes {
		nodeQNs[node.QualifiedName] = true
	}
	importIndex := newImportTargetIndex(graph.nodes)
	importsByFile := make(map[string]map[string]string, len(repository.Files))
	for _, parsed := range repository.Files {
		rel := filepath.ToSlash(parsed.File.Path)
		fileQN := fileQualifiedName(rel)
		imports := map[string]string{}
		for _, fact := range parsed.Extraction.Imports {
			// Ambient declaration files publish type shapes; Superopen does not
			// materialize IMPORTS edges from them into the comparable graph.
			if strings.HasSuffix(strings.ToLower(rel), ".d.ts") {
				qn := localSyntaxImportTargetForLanguage(parsed.File.Language, rel, fact.Name, repository.GoModule, importIndex)
				if fact.LocalName != "" && qn != "" {
					imports[fact.LocalName] = qn
				}
				continue
			}
			qn := localSyntaxImportTargetForLanguage(parsed.File.Language, rel, fact.Name, repository.GoModule, importIndex)
			strategy := "module_path"
			if qn == "" {
				continue
			}
			if fact.LocalName != "" {
				imports[fact.LocalName] = qn
			}
			graph.edges = append(graph.edges, pendingEdge{source: fileQN, target: qn, kind: "IMPORTS",
				properties: api.Properties{"local_name": fact.LocalName}, evidence: syntaxEvidence(rel, fact, strategy)})
		}
		importsByFile[rel] = imports
	}

	reportIndexProgress("assemble usages/calls files=%d", len(repository.Files))
	resolveStarted := time.Now()
	type fileResolveResult struct {
		edges      []pendingEdge
		unresolved []pendingEdge
	}
	resolved := make([]fileResolveResult, len(repository.Files))
	workers := parseWorkerCount()
	if workers < 1 {
		workers = 1
	}
	var cursor atomic.Int64
	var group sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for {
				index := int(cursor.Add(1) - 1)
				if index >= len(repository.Files) {
					return
				}
				parsed := &repository.Files[index]
				edges, unresolved := resolveSyntaxFileRelationships(project, parsed, registry, nodeQNs, byName, byFileScope, localBindings, importsByFile)
				resolved[index] = fileResolveResult{edges: edges, unresolved: unresolved}
				parsed.Extraction.Usages = nil
				parsed.Extraction.Writes = nil
			}
		}()
	}
	group.Wait()
	for _, result := range resolved {
		graph.addEdges(result.edges)
		graph.unresolved = append(graph.unresolved, result.unresolved...)
	}
	reportIndexProgress("assemble usages/calls elapsed=%s", indexElapsed(resolveStarted))

	for _, parsed := range repository.Files {
		rel := filepath.ToSlash(parsed.File.Path)
		for _, fact := range parsed.Extraction.Decorators {
			source, ok := byFileScope[rel+"\x00"+fact.Scope]
			if !ok {
				continue
			}
			name := syntaxCallBase(fact.Name)
			resolvedCall, strategy, confidence := resolveSyntaxCall(rel, fact.Scope, byName[parsed.File.Language+"\x00"+name])
			target := resolvedCall.qn
			if target == "" {
				target = "<decorator:" + fact.Name + ">"
				graph.nodes = append(graph.nodes, api.Node{Project: project, Label: "Decorator", Name: name,
					QualifiedName: target, Properties: api.Properties{}})
				strategy, confidence = "tree_sitter_synthetic_decorator", .7
			}
			graph.addEdge(pendingEdge{source: source.qn, target: target, kind: "DECORATES",
				properties: api.Properties{"decorator": fact.Name}, evidence: syntaxEvidence(rel, fact, strategy, confidence)})
		}
	}
	dropUsagesShadowedByCalls(&graph)
	enrichTransitiveComplexity(&graph)
	sortGraph(&graph)
	sort.Slice(coverage.Rows, func(i, j int) bool {
		if coverage.Rows[i].Path != coverage.Rows[j].Path {
			return coverage.Rows[i].Path < coverage.Rows[j].Path
		}
		if coverage.Rows[i].Kind != coverage.Rows[j].Kind {
			return coverage.Rows[i].Kind < coverage.Rows[j].Kind
		}
		return coverage.Rows[i].Detail < coverage.Rows[j].Detail
	})
	if len(coverage.Rows) > 0 {
		coverage.Status = "partial"
		if coverage.RecordingStatus == "complete" {
			coverage.RecordingStatus = "truncated"
		}
	}
	return graph, coverage
}

func resolveSyntaxFileRelationships(
	project string,
	parsed *ParsedSyntaxFile,
	registry symbolRegistry,
	nodeQNs map[string]bool,
	byName map[string][]syntaxDefinitionRef,
	byFileScope map[string]syntaxDefinitionRef,
	localBindings map[string]map[string]bool,
	importsByFile map[string]map[string]string,
) ([]pendingEdge, []pendingEdge) {
	rel := filepath.ToSlash(parsed.File.Path)
	fileQN := fileQualifiedName(rel)
	moduleQN := syntaxDefinitionModuleQN(parsed.File.Language, rel)
	imports := importsByFile[rel]
	session := newResolveSession(moduleQN, imports)
	defaultSource := fileQN
	if parsed.File.Language != "go" && parsed.File.Language != "java" && nodeQNs[moduleQN] {
		defaultSource = moduleQN
	}
	edges := make([]pendingEdge, 0, len(parsed.Extraction.Usages)+len(parsed.Extraction.Calls))
	unresolved := make([]pendingEdge, 0)
	for _, relationship := range []struct {
		facts []OccurrenceFact
		kind  string
	}{{parsed.Extraction.Usages, "USAGE"}, {parsed.Extraction.Writes, "WRITES"}} {
		for _, fact := range relationship.facts {
			if relationship.kind == "USAGE" && occurrenceLocallyBound(rel, fact, localBindings) {
				continue
			}
			source := syntaxOccurrenceSource(rel, fact.Scope, defaultSource, byFileScope)
			resolution := registry.resolveWith(session, fact.Name)
			if resolution.qn == "" || resolution.qn == source {
				continue
			}
			kind := relationship.kind
			callee := ""
			if relationship.kind == "USAGE" {
				callee = internString(fact.Name)
				if fact.MayBeCallReference {
					if target, ok := registry.exact[resolution.qn]; ok &&
						jsxCallableLabel(target.Label) &&
						resolution.strategy == "same_module" {
						if src, ok := registry.exact[source]; ok && jsxCallableLabel(src.Label) {
							kind = "CALL_REFERENCE"
						}
					}
				}
			}
			edges = append(edges, pendingEdge{
				source: internString(source), target: internString(resolution.qn), kind: internString(kind),
				callee: callee, evidence: occurrenceEvidence(rel, fact, resolution.strategy, resolution.confidence),
			})
		}
	}
	for _, fact := range parsed.Extraction.Throws {
		source := syntaxOccurrenceSource(rel, fact.Scope, defaultSource, byFileScope)
		resolution := registry.resolveWith(session, fact.Name)
		if resolution.qn == "" || resolution.qn == source {
			continue
		}
		kind := "THROWS"
		if strings.Contains(fact.Name, "Error") || strings.Contains(fact.Name, "Panic") ||
			strings.Contains(fact.Name, "error") || strings.Contains(fact.Name, "panic") {
			kind = "RAISES"
		}
		edges = append(edges, pendingEdge{source: source, target: resolution.qn, kind: kind,
			evidence: syntaxEvidence(rel, fact, resolution.strategy, resolution.confidence)})
	}
	for _, fact := range parsed.Extraction.Calls {
		source := syntaxOccurrenceSource(rel, fact.Scope, defaultSource, byFileScope)
		name := syntaxCallBase(fact.Name)
		candidates := byName[parsed.File.Language+"\x00"+name]
		resolution := registry.resolveWith(session, fact.Name)
		resolution = registry.applyFieldTypeHint(fact.Name, source, resolution)
		if isJSXComponentCall(fact) {
			switch resolution.strategy {
			case "import_map", "import_map_suffix":
				resolution.strategy = "lsp_ts_jsx_import"
				resolution.confidence = .95
			case "same_module":
				resolution.strategy = "lsp_ts_jsx"
				resolution.confidence = .95
			default:
				resolution = symbolResolution{}
			}
			if resolution.qn != "" {
				if target, ok := registry.exact[resolution.qn]; !ok || !jsxCallableLabel(target.Label) {
					resolution = symbolResolution{}
				}
			}
		}
		if resolution.qn != "" && resolution.qn != source {
			edges = append(edges, pendingEdge{source: source, target: resolution.qn, kind: "CALLS",
				properties: syntaxCallProperties(fact, resolution), evidence: syntaxEvidence(rel, fact, resolution.strategy, resolution.confidence)})
			continue
		}
		if isCFamilyLanguage(parsed.File.Language) && coveredByResolvedCall(*parsed, fact) {
			continue
		}
		if isJSXComponentCall(fact) {
			continue
		}
		if len(candidates) > 1 {
			alternatives := make([]string, len(candidates))
			for index := range candidates {
				alternatives[index] = candidates[index].qn
			}
			sort.Strings(alternatives)
			evidence := syntaxEvidence(rel, fact, "tree_sitter_ambiguous_call", .2)
			evidence.Ambiguous, evidence.Alternatives, evidence.Unresolved = true, alternatives, "multiple local definitions"
			unresolved = append(unresolved, pendingEdge{source: source, target: fact.Name, kind: "CALL_REFERENCE",
				properties: api.Properties{"callee": fact.Name}, evidence: evidence})
			continue
		}
		evidence := syntaxEvidence(rel, fact, "tree_sitter_unresolved_call", .35)
		evidence.Unresolved = "no local definition"
		unresolved = append(unresolved, pendingEdge{source: source, target: fact.Name, kind: "CALL_REFERENCE",
			properties: api.Properties{"callee": fact.Name}, evidence: evidence})
	}
	_ = project
	return edges, unresolved
}

// dropUsagesShadowedByCalls removes a USAGE edge when the exact same
// occurrence (source, target, line) is already recorded as a CALLS or
// CALL_REFERENCE. A JSX component tag and a `new X()` constructor each surface
// once as a call fact and once as a reference fact; Superopen keeps only the
// call for that occurrence. Lazy-loaded JSX bindings mint no call edge, so
// their USAGE is line-distinct and survives.
func dropUsagesShadowedByCalls(graph *goGraph) {
	type occurrence struct {
		source, target string
		line           int
	}
	edgeLine := func(edge pendingEdge) int {
		if edge.evidence != nil && edge.evidence.Location != nil {
			return edge.evidence.Location.StartLine
		}
		return 0
	}
	calls := make(map[occurrence]bool)
	for _, edge := range graph.edges {
		if edge.kind == "CALLS" || edge.kind == "CALL_REFERENCE" {
			calls[occurrence{edge.source, edge.target, edgeLine(edge)}] = true
		}
	}
	if len(calls) == 0 {
		return
	}
	filtered := graph.edges[:0]
	for _, edge := range graph.edges {
		if edge.kind == "USAGE" && calls[occurrence{edge.source, edge.target, edgeLine(edge)}] {
			continue
		}
		filtered = append(filtered, edge)
	}
	graph.edges = filtered
}

// syntaxOccurrenceSource matches Superopen enclosing_func_qn ownership: only
// Function/Method scopes own value/call occurrences. Class/Enum/Interface/Type
// scopes qualify nested definitions but leave occurrences on the module/file.
func syntaxOccurrenceSource(file, scope, defaultSource string, byFileScope map[string]syntaxDefinitionRef) string {
	for scope != "" {
		owner, ok := byFileScope[file+"\x00"+scope]
		if !ok {
			break
		}
		switch owner.label {
		case "Function", "Method":
			return owner.qn
		}
		index := strings.LastIndexByte(scope, '.')
		if index < 0 {
			break
		}
		scope = scope[:index]
	}
	return defaultSource
}

func isJSXComponentCall(fact SyntaxFact) bool {
	return fact.NodeType == "jsx_self_closing_element" || fact.NodeType == "jsx_opening_element"
}

func jsxCallableLabel(label string) bool {
	switch label {
	case "Function", "Method", "Constructor", "Class":
		return true
	default:
		return false
	}
}

func syntaxFactLocallyBound(file string, fact SyntaxFact, bindings map[string]map[string]bool) bool {
	if fact.Scope == "" || strings.ContainsAny(fact.Name, ".:") {
		return false
	}
	scope := fact.Scope
	for {
		if bindings[file+"\x00"+scope][fact.Name] {
			return true
		}
		index := strings.LastIndexByte(scope, '.')
		if index < 0 {
			return false
		}
		scope = scope[:index]
	}
}

func enrichTransitiveComplexity(graph *goGraph) {
	index := make(map[string]int, len(graph.nodes))
	for position := range graph.nodes {
		node := &graph.nodes[position]
		if node.Label == "Function" || node.Label == "Method" {
			index[node.QualifiedName] = position
		}
	}
	callees := make(map[int][]int)
	for _, edge := range graph.edges {
		if edge.kind != "CALLS" {
			continue
		}
		source, sourceOK := index[edge.source]
		target, targetOK := index[edge.target]
		if sourceOK && targetOK {
			callees[source] = append(callees[source], target)
		}
	}
	loopDepth := make([]int, len(graph.nodes))
	transitive := make([]int, len(graph.nodes))
	state := make([]uint8, len(graph.nodes))
	recursive := make([]bool, len(graph.nodes))
	for _, position := range index {
		loopDepth[position] = propertyInt(graph.nodes[position].Properties, "loop_depth")
		if value, ok := graph.nodes[position].Properties["self_recursive"].(bool); ok {
			recursive[position] = value
		}
	}
	var visit func(int, int) int
	visit = func(position, depth int) int {
		if state[position] == 2 {
			return transitive[position]
		}
		if state[position] == 1 {
			recursive[position] = true
			return 0
		}
		if depth > 256 {
			return loopDepth[position]
		}
		state[position] = 1
		best := 0
		for _, callee := range callees[position] {
			if callee == position {
				recursive[position] = true
				continue
			}
			if value := visit(callee, depth+1); value > best {
				best = value
			}
		}
		transitive[position] = loopDepth[position] + best
		state[position] = 2
		return transitive[position]
	}
	// Iteration order deliberately follows node insertion order, matching the
	// pinned buffer's ID-order write-back behavior for cycle back edges.
	for position := range graph.nodes {
		if _, ok := index[graph.nodes[position].QualifiedName]; !ok {
			continue
		}
		if state[position] != 2 {
			visit(position, 0)
		}
		// A non-callable node can share a qualified name with a callable one; only
		// the callable carries recursion metrics Superopen.
		if label := graph.nodes[position].Label; label != "Function" && label != "Method" {
			continue
		}
		graph.nodes[position].Properties["transitive_loop_depth"] = transitive[position]
		graph.nodes[position].Properties["recursive"] = recursive[position]
	}
}

func propertyInt(properties api.Properties, key string) int {
	switch value := properties[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func syntaxCallProperties(fact SyntaxFact, resolution symbolResolution) api.Properties {
	properties := api.Properties{
		"callee":     fact.Name,
		"confidence": math.Round(resolution.confidence*100) / 100,
		"strategy":   resolution.strategy,
		"candidates": resolution.candidates,
	}
	if fact.StartLine > 0 {
		properties["line"] = fact.StartLine
	}
	if len(fact.Arguments) > 0 {
		arguments := make([]map[string]any, 0, len(fact.Arguments))
		for _, argument := range fact.Arguments {
			item := map[string]any{"i": argument.Index, "e": argument.Expr}
			if argument.Literal {
				item["v"] = argument.Value
			}
			if argument.Keyword != "" {
				item["k"] = argument.Keyword
			}
			arguments = append(arguments, item)
		}
		properties["args"] = arguments
	}
	return properties
}

func syntaxImportTargetable(label string) bool {
	switch label {
	case "Class", "Interface", "Function", "Method", "Module", "Struct", "Enum", "Trait", "Type", "File":
		return true
	default:
		return false
	}
}

func dataLanguageVariables(language string) bool {
	switch language {
	case "json", "json5", "yaml", "toml", "ini", "hcl", "properties":
		return true
	default:
		return false
	}
}

func syntaxDefinitionQN(language, file, scope, name string) string {
	module := syntaxDefinitionModuleQN(language, file)
	qualified := joinSyntaxScope(scope, name)
	if module == "" {
		return qualified
	}
	return module + "." + qualified
}

func syntaxDefinitionFactQN(language, file string, fact SyntaxFact) string {
	// JS/TS: class members keep Class.method; everything else is module-flat.
	if isJSLanguage(language) && fact.Kind == "function" {
		if fact.Scope != "" {
			return syntaxDefinitionQN(language, file, fact.Scope, fact.Name)
		}
		return joinSyntaxScope(syntaxDefinitionModuleQN(language, file), fact.Name)
	}
	if isJSLanguage(language) && fact.Kind == "class" && fact.NodeType == "type_alias_declaration" {
		return joinSyntaxScope(syntaxDefinitionModuleQN(language, file), fact.Name)
	}
	return syntaxDefinitionQN(language, file, fact.Scope, fact.Name)
}

func syntaxDefinitionModuleQN(language, file string) string {
	if language == "go" || language == "java" {
		directory := filepath.ToSlash(filepath.Dir(file))
		if directory == "." || directory == "" {
			return ""
		}
		return strings.ReplaceAll(strings.TrimPrefix(directory, "./"), "/", ".")
	}
	module := syntaxModuleQN(file)
	if isJSLanguage(language) && strings.EqualFold(strings.TrimSuffix(filepath.Base(file), filepath.Ext(file)), "index") {
		if index := strings.LastIndexByte(module, '.'); index >= 0 {
			return module[:index]
		}
	}
	return module
}

func syntaxModuleQN(file string) string {
	module := strings.TrimLeft(strings.TrimSuffix(filepath.ToSlash(file), filepath.Ext(file)), ".")
	segments := strings.Split(module, "/")
	for index := range segments {
		segments[index] = strings.TrimLeft(segments[index], ".")
	}
	return strings.Join(segments, ".")
}

func joinSyntaxScope(scope, name string) string {
	if scope == "" {
		return name
	}
	return scope + "." + name
}

func syntaxNodeLabel(language string, fact SyntaxFact) string {
	switch fact.Kind {
	case "function":
		if language == "go" && fact.NodeType == "method_declaration" {
			return "Method"
		}
		if isJSLanguage(language) {
			// Class members (method_definition / class-field arrows) carry a
			// non-empty class scope; object-literal methods publish flat.
			if fact.Scope != "" {
				return "Method"
			}
			return "Function"
		}
		if fact.Scope != "" {
			return "Method"
		}
		return "Function"
	case "class":
		if language == "go" {
			switch fact.NodeType {
			case "type_alias":
				return "Type"
			case "type_spec":
				switch fact.TypeKind {
				case "interface":
					return "Interface"
				case "struct":
					return "Struct"
				}
			}
		}
		switch fact.NodeType {
		case "interface_declaration", "interface_definition", "interface_type", "protocol_declaration":
			return "Interface"
		case "trait_item", "trait_definition":
			return "Trait"
		case "enum_specifier", "enum_declaration", "enum_item":
			return "Enum"
		case "type_alias_declaration", "type_alias", "type_item", "type_definition":
			return "Type"
		}
		return "Class"
	case "field":
		return "Field"
	case "variable":
		return "Variable"
	case "module":
		return "Module"
	default:
		return "Type"
	}
}

// syntaxModuleProperties returns the definition property set Superopen records for
// Module and Section nodes, where only the test flag varies by file.
func syntaxModuleProperties(file string) api.Properties {
	return api.Properties{"complexity": 0, "lines": 0, "is_exported": true,
		"is_test": isTestPath(file), "is_entry_point": false}
}

func syntaxDefinitionProperties(language, file string, fact SyntaxFact) api.Properties {
	// A variable definition carries no span or body, so Superopen reports zeroed
	// metrics for it even when the initializer spans many lines.
	if fact.Kind == "variable" {
		return api.Properties{"complexity": 0, "lines": 0, "is_exported": fact.IsExported,
			"is_test": fact.IsTest, "is_entry_point": fact.IsEntryPoint}
	}
	properties := api.Properties{
		"complexity":  fact.Complexity,
		"lines":       fact.Lines,
		"is_exported": fact.IsExported,
		// Superopen marks a definition as a test only from a language test
		// attribute; residing in a test file marks the Module, not its symbols.
		"is_test":        fact.IsTest,
		"is_entry_point": fact.IsEntryPoint,
	}
	if fact.Kind == "function" {
		properties["cognitive"] = fact.Cognitive
		properties["loop_count"] = fact.LoopCount
		properties["loop_depth"] = fact.LoopDepth
		properties["self_recursive"] = false
		properties["param_count"] = len(fact.ParamNames)
		properties["max_access_depth"] = fact.MaxAccessDepth
		properties["linear_scan_in_loop"] = 0
		properties["alloc_in_loop"] = 0
		properties["recursion_in_loop"] = false
		properties["unguarded_recursion"] = false
	}
	if fact.Docstring != "" {
		properties["docstring"] = fact.Docstring
	}
	if fact.Signature != "" {
		properties["signature"] = fact.Signature
	}
	if fact.ReturnType != "" {
		properties["return_type"] = fact.ReturnType
	}
	if fact.ParentClass != "" {
		properties["parent_class"] = joinSyntaxScope(syntaxDefinitionModuleQN(language, file), fact.ParentClass)
	}
	if len(fact.BaseClasses) > 0 {
		properties["base_classes"] = fact.BaseClasses
	}
	if len(fact.ParamNames) > 0 {
		properties["param_names"] = fact.ParamNames
	}
	if len(fact.ParamTypes) > 0 {
		properties["param_types"] = fact.ParamTypes
	}
	if fact.MinHash != "" {
		properties["fp"] = fact.MinHash
	}
	if fact.StructuralProfile != "" {
		properties["sp"] = fact.StructuralProfile
	}
	if fact.BodyTokens != "" {
		properties["bt"] = fact.BodyTokens
	}
	return properties
}

func syntaxLocation(file string, fact SyntaxFact) api.Location {
	return api.Location{File: file, StartLine: fact.StartLine, StartColumn: fact.StartColumn,
		EndLine: fact.EndLine, EndColumn: fact.EndColumn}
}

func locationPointer(location api.Location) *api.Location { return &location }

func syntaxEvidence(file string, fact SyntaxFact, strategy string, confidence ...float64) *api.Evidence {
	value := fact.Confidence
	if len(confidence) > 0 {
		value = confidence[0]
	}
	return &api.Evidence{Strategy: strategy, Confidence: value, Location: locationPointer(syntaxLocation(file, fact))}
}

func syntaxCallBase(name string) string {
	name = strings.TrimSpace(name)
	if index := strings.LastIndexAny(name, ".:"); index >= 0 {
		name = name[index+1:]
	}
	return strings.Trim(name, "()")
}

func resolveSyntaxCall(file, scope string, candidates []syntaxDefinitionRef) (syntaxDefinitionRef, string, float64) {
	var zero syntaxDefinitionRef
	parentScope := scope
	if index := strings.LastIndexByte(parentScope, '.'); index >= 0 {
		parentScope = parentScope[:index]
	} else {
		parentScope = ""
	}
	var lexical []syntaxDefinitionRef
	for _, candidate := range candidates {
		if candidate.file == file && candidate.scope == parentScope {
			lexical = append(lexical, candidate)
		}
	}
	if len(lexical) == 1 {
		return lexical[0], "tree_sitter_lexical_scope", .9
	}
	var sameFile []syntaxDefinitionRef
	for _, candidate := range candidates {
		if candidate.file == file {
			sameFile = append(sameFile, candidate)
		}
	}
	if len(sameFile) == 1 {
		return sameFile[0], "tree_sitter_same_file", .8
	}
	if len(candidates) == 1 {
		return candidates[0], "tree_sitter_unique_name", .65
	}
	return zero, "", 0
}
