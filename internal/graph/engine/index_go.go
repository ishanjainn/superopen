package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/format"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// IndexGoDevelopment builds the Go extraction slice of the native graph.
func IndexGoDevelopment(ctx context.Context, request api.BuildRequest, engineVersion string) (api.BuildResult, error) {
	return indexGoDevelopment(ctx, request, engineVersion, nil)
}

func indexGoDevelopment(ctx context.Context, request api.BuildRequest, engineVersion string, model *pretrainedVectors) (api.BuildResult, error) {
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
	parsed, coverage, generation, err := parseGoFiles(root, project, files)
	if err != nil {
		return api.BuildResult{}, err
	}
	separateUnresolvedRelationships(&parsed)
	revision := gitRevision(ctx, root)
	if request.ExpectedSource != "" && request.ExpectedSource != revision {
		return api.BuildResult{}, fmt.Errorf("source revision changed: expected %s, found %s", request.ExpectedSource, revision)
	}
	database, err := Publish(ctx, root, func(ctx context.Context, path string) error {
		store, err := OpenWritable(path)
		if err != nil {
			return err
		}
		buildErr := store.Build(ctx, func(builder *Builder) error {
			if err := builder.PutProject(ProjectRecord{
				Name: project, RootPath: root, Generation: generation, SourceRevision: revision,
				EngineVersion: engineVersion, IndexedAt: time.Now().UTC(),
			}); err != nil {
				return err
			}
			for _, file := range parsed.files {
				if err := builder.PutFile(file); err != nil {
					return err
				}
			}
			ids := make(map[string]int64, len(parsed.nodes))
			for _, node := range parsed.nodes {
				id, err := builder.PutNode(node)
				if err != nil {
					return err
				}
				ids[node.QualifiedName] = id
			}
			if err := putGraphSemantics(builder, parsed, ids, project, model); err != nil {
				return err
			}
			for _, edge := range parsed.edges {
				source, sourceOK := ids[edge.source]
				target, targetOK := ids[edge.target]
				if !sourceOK || !targetOK {
					continue
				}
				if _, err := builder.PutEdge(api.Edge{
					Project: project, SourceID: source, TargetID: target, Type: edge.kind,
					Properties: edge.dumpProperties(), Evidence: edge.evidence,
				}); err != nil {
					return err
				}
			}
			for _, edge := range parsed.unresolved {
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
	return api.BuildResult{
		Status: "ok", Project: project, Database: database,
		SourceRevision: revision, Generation: generation, NodeCount: nodeCount,
		EdgeCount: edgeCount, FileCount: len(parsed.files), Duration: time.Since(started), Coverage: summarizedCoverage(coverage, 100), Changes: changes,
	}, nil
}

type goGraph struct {
	files      []FileRecord
	nodes      []api.Node
	edges      []pendingEdge
	unresolved []pendingEdge
	intern     *graphIntern
	edgeSeen   map[edgeKey]struct{}
}

func (g *goGraph) ensureIntern() *graphIntern {
	if g.intern == nil {
		g.intern = newGraphIntern(len(g.nodes) + len(g.edges) + 8)
	}
	return g.intern
}

func (g *goGraph) addEdge(edge pendingEdge) {
	intern := g.ensureIntern()
	if g.edgeSeen == nil {
		g.edgeSeen = make(map[edgeKey]struct{}, 1024)
	}
	key := edgeIdentityKey(intern, edge)
	if _, ok := g.edgeSeen[key]; ok {
		return
	}
	g.edgeSeen[key] = struct{}{}
	g.edges = append(g.edges, edge)
}

func (g *goGraph) addEdges(edges []pendingEdge) {
	for _, edge := range edges {
		g.addEdge(edge)
	}
}

type pendingEdge struct {
	source     string
	target     string
	kind       string
	callee     string
	properties api.Properties
	evidence   *api.Evidence
}

func (e pendingEdge) dumpProperties() api.Properties {
	if e.callee == "" {
		return e.properties
	}
	if e.properties == nil {
		return api.Properties{"callee": e.callee}
	}
	out := make(api.Properties, len(e.properties)+1)
	for key, value := range e.properties {
		out[key] = value
	}
	out["callee"] = e.callee
	return out
}

func (e pendingEdge) Callee() string {
	if e.callee != "" {
		return e.callee
	}
	value, _ := e.properties["callee"].(string)
	return value
}

type parsedFile struct {
	rel       string
	abs       string
	file      *ast.File
	fset      *token.FileSet
	pkg       string
	pkgQN     string
	fileQN    string
	imports   map[string]string
	functions map[string]string
	methods   map[string]string
	types     map[string]string
}

func parseGoFiles(root, project string, files []string) (goGraph, api.Coverage, string, error) {
	return parseGoFilesWithRegistry(root, project, files, nil)
}

func parseGoFilesWithRegistry(root, project string, files []string, seedNodes []api.Node) (goGraph, api.Coverage, string, error) {
	graph := goGraph{}
	coverage := api.Coverage{IndexMode: "development-go", RecordingStatus: "truncated"}
	var generationParts []string
	var parsedFiles []*parsedFile
	sharedFileSet := token.NewFileSet()
	for _, rel := range files {
		ext := strings.ToLower(filepath.Ext(rel))
		if ext != ".go" {
			if isLikelySource(rel) {
				coverage.Rows = append(coverage.Rows, api.CoverageRow{
					Path: filepath.ToSlash(rel), Kind: "unsupported_language",
					Detail: "native development index currently accepts Go only",
				})
			}
			continue
		}
		abs := filepath.Join(root, filepath.FromSlash(rel))
		body, err := os.ReadFile(abs)
		if err != nil {
			coverage.Rows = append(coverage.Rows, api.CoverageRow{Path: rel, Kind: "read", Detail: err.Error()})
			continue
		}
		hash := sha256.Sum256(body)
		info, _ := os.Stat(abs)
		record := FileRecord{Project: project, Path: rel, SHA256: hex.EncodeToString(hash[:]), Language: "go"}
		if info != nil {
			record.MTimeNS = info.ModTime().UnixNano()
			record.Size = info.Size()
		}
		graph.files = append(graph.files, record)
		generationParts = append(generationParts, rel+":"+record.SHA256)
		fset := sharedFileSet
		file, parseErr := parser.ParseFile(fset, abs, body, parser.ParseComments|parser.AllErrors)
		if file == nil {
			coverage.Rows = append(coverage.Rows, api.CoverageRow{Path: rel, Kind: "extract", Detail: parseErr.Error()})
			continue
		}
		if parseErr != nil {
			coverage.Rows = append(coverage.Rows, api.CoverageRow{Path: rel, Kind: "parse_partial", Detail: parseErr.Error()})
		}
		rel = filepath.ToSlash(rel)
		parsedFiles = append(parsedFiles, &parsedFile{
			rel: rel, abs: abs, file: file, fset: fset, pkg: file.Name.Name, pkgQN: goPackageQN(rel, file.Name.Name),
			fileQN: fileQualifiedName(rel), imports: map[string]string{},
			functions: map[string]string{}, methods: map[string]string{}, types: map[string]string{},
		})
	}
	sort.Strings(generationParts)
	generationHash := sha256.Sum256([]byte(strings.Join(generationParts, "\n")))
	coverage.Generation = hex.EncodeToString(generationHash[:])
	now := time.Now().UTC()
	coverage.RecordedAt = &now
	coverage.HashRecordsComplete = len(coverage.Rows) == 0
	if len(coverage.Rows) == 0 {
		coverage.Status = "complete"
		coverage.RecordingStatus = "complete"
	} else {
		coverage.Status = "partial"
	}
	indexGoStructure(project, parsedFiles, &graph)
	indexGoDefinitions(project, parsedFiles, &graph)
	registryNodes := make([]api.Node, 0, len(seedNodes)+len(graph.nodes))
	registryNodes = append(registryNodes, seedNodes...)
	registryNodes = append(registryNodes, graph.nodes...)
	indexGoCalls(project, parsedFiles, &graph, registryNodes)
	indexGoReferences(parsedFiles, &graph, registryNodes)
	indexGoTypeRelationships(parsedFiles, &graph)
	resolveGoImportEdges(parsedFiles, &graph, seedNodes)
	sortGraph(&graph)
	return graph, coverage, coverage.Generation, nil
}

func indexGoDefinitions(project string, files []*parsedFile, graph *goGraph) {
	for _, parsed := range files {
		// Go package modules share qualified names with their repository
		// folders in the pinned graph. The folder wins the node collision, but
		// each source file still retains its DEFINES edge to that package QN.
		graph.edges = append(graph.edges, pendingEdge{source: parsed.fileQN, target: parsed.pkgQN, kind: "DEFINES", evidence: evidenceAt(parsed, parsed.file.Package, "go_package", 1)})
		for _, imp := range parsed.file.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				continue
			}
			alias := filepath.Base(path)
			if imp.Name != nil {
				alias = imp.Name.Name
			}
			parsed.imports[alias] = path
			target := "external:" + path
			graph.nodes = append(graph.nodes, api.Node{
				Project: project, Label: "Module", Name: filepath.Base(path), QualifiedName: target,
				Properties: api.Properties{"import_path": path, "external": true},
			})
			graph.edges = append(graph.edges, pendingEdge{
				source: parsed.fileQN, target: target, kind: "IMPORTS",
				properties: api.Properties{"local_name": alias, "import_path": path},
				evidence:   evidenceAt(parsed, imp.Pos(), "go_ast_import", 1),
			})
		}
		for _, declaration := range parsed.file.Decls {
			switch decl := declaration.(type) {
			case *ast.FuncDecl:
				label := "Function"
				qn := parsed.pkgQN + "." + decl.Name.Name
				if decl.Recv != nil && len(decl.Recv.List) > 0 {
					label = "Method"
					qn = parsed.pkgQN + "." + decl.Name.Name
					parsed.methods[decl.Name.Name] = qn
				} else {
					parsed.functions[decl.Name.Name] = qn
				}
				properties := functionProperties(parsed, decl)
				graph.nodes = append(graph.nodes, nodeAt(project, label, decl.Name.Name, qn, parsed, decl.Pos(), decl.End(), properties))
				graph.edges = append(graph.edges, pendingEdge{source: parsed.fileQN, target: qn, kind: "DEFINES", evidence: evidenceAt(parsed, decl.Pos(), "go_ast_definition", 1)})
				if decl.Recv != nil && len(decl.Recv.List) > 0 {
					receiverQN := parsed.pkgQN + "." + receiverName(decl.Recv.List[0].Type)
					graph.edges = append(graph.edges, pendingEdge{source: receiverQN, target: qn, kind: "DEFINES_METHOD", evidence: evidenceAt(parsed, decl.Pos(), "go_receiver_method", 1)})
				}
			case *ast.GenDecl:
				indexGenDecl(project, parsed, decl, graph)
			}
		}
	}
}

func indexGoStructure(project string, files []*parsedFile, graph *goGraph) {
	projectQN := project
	graph.nodes = append(graph.nodes, api.Node{Project: project, Label: "Project", Name: project, QualifiedName: projectQN,
		Location: api.Location{File: "{}"}})
	folders := map[string]bool{}
	for _, parsed := range files {
		graph.nodes = append(graph.nodes, api.Node{
			Project: project, Label: "File", Name: filepath.Base(parsed.rel), QualifiedName: parsed.fileQN,
			Location:   api.Location{File: parsed.rel, StartLine: 1, EndLine: parsed.fset.Position(parsed.file.End()).Line},
			Properties: api.Properties{"language": "go", "package": parsed.pkg, "package_qualified_name": parsed.pkgQN},
		})
		directory := filepath.ToSlash(filepath.Dir(parsed.rel))
		if directory == "." || directory == "" {
			graph.edges = append(graph.edges, pendingEdge{source: projectQN, target: parsed.fileQN, kind: "CONTAINS_FILE", evidence: layoutEvidence(parsed.rel)})
			continue
		}
		parts := strings.Split(directory, "/")
		parent := projectQN
		for index := range parts {
			folder := strings.Join(parts[:index+1], "/")
			folderQN := folderQualifiedName(folder)
			if !folders[folder] {
				folders[folder] = true
				graph.nodes = append(graph.nodes, api.Node{
					Project: project, Label: "Folder", Name: parts[index], QualifiedName: folderQN,
					Location: api.Location{File: folder},
				})
				graph.edges = append(graph.edges, pendingEdge{source: parent, target: folderQN, kind: "CONTAINS_FOLDER", evidence: layoutEvidence(folder)})
			}
			parent = folderQN
		}
		graph.edges = append(graph.edges, pendingEdge{source: parent, target: parsed.fileQN, kind: "CONTAINS_FILE", evidence: layoutEvidence(parsed.rel)})
	}
}

func layoutEvidence(path string) *api.Evidence {
	return &api.Evidence{
		Strategy: "repository_layout", Confidence: 1,
		Location: &api.Location{File: filepath.ToSlash(path)},
	}
}

func indexGenDecl(project string, parsed *parsedFile, decl *ast.GenDecl, graph *goGraph) {
	for _, spec := range decl.Specs {
		switch value := spec.(type) {
		case *ast.TypeSpec:
			label := "Class"
			if value.Assign.IsValid() {
				label = "Type"
			}
			switch value.Type.(type) {
			case *ast.StructType:
				label = "Struct"
			case *ast.InterfaceType:
				label = "Interface"
			}
			qn := parsed.pkgQN + "." + value.Name.Name
			parsed.types[value.Name.Name] = qn
			graph.nodes = append(graph.nodes, nodeAt(project, label, value.Name.Name, qn, parsed, value.Pos(), value.End(), nil))
			graph.edges = append(graph.edges, pendingEdge{source: parsed.fileQN, target: qn, kind: "DEFINES", evidence: evidenceAt(parsed, value.Pos(), "go_ast_definition", 1)})
			if iface, ok := value.Type.(*ast.InterfaceType); ok {
				for _, field := range iface.Methods.List {
					funcType, ok := field.Type.(*ast.FuncType)
					if !ok {
						continue
					}
					for _, methodName := range field.Names {
						methodQN := qn + "." + methodName.Name
						signature, returnType := goFunctionSignature(parsed.fset, funcType)
						properties := api.Properties{"signature": signature, "is_exported": ast.IsExported(methodName.Name)}
						if returnType != "" {
							properties["return_type"] = returnType
						}
						graph.nodes = append(graph.nodes, nodeAt(project, "Method", methodName.Name, methodQN, parsed, field.Pos(), field.End(), properties))
						graph.edges = append(graph.edges,
							pendingEdge{source: parsed.fileQN, target: methodQN, kind: "DEFINES", evidence: evidenceAt(parsed, field.Pos(), "go_interface_method", 1)},
							pendingEdge{source: qn, target: methodQN, kind: "DEFINES_METHOD", evidence: evidenceAt(parsed, field.Pos(), "go_interface_method", 1)},
						)
					}
				}
			}
		case *ast.ValueSpec:
			// The pinned Go extractor does not publish variables from grouped
			// var declarations (it does publish grouped constants). Preserve
			// that observable behavior rather than broadening the symbol set.
			if decl.Tok == token.VAR && decl.Lparen.IsValid() {
				continue
			}
			for _, name := range value.Names {
				if name.Name == "_" {
					continue
				}
				qn := parsed.pkgQN + "." + name.Name
				properties := api.Properties{"is_constant": decl.Tok == token.CONST}
				graph.nodes = append(graph.nodes, nodeAt(project, "Variable", name.Name, qn, parsed, name.Pos(), value.End(), properties))
				graph.edges = append(graph.edges, pendingEdge{source: parsed.fileQN, target: qn, kind: "DEFINES", evidence: evidenceAt(parsed, name.Pos(), "go_ast_definition", 1)})
			}
		}
	}
}

func indexGoCalls(project string, files []*parsedFile, graph *goGraph, registryNodes []api.Node) {
	resolvedIdentifiers, resolvedSelectors := resolveGoObjects(files)
	localImports := localGoImportPackages(files)
	registry := newSymbolRegistry(registryNodes)
	functions := map[string]string{}
	methods := map[string]string{}
	methodsByName := map[string][]string{}
	for _, file := range files {
		for name, qn := range file.functions {
			functions[file.pkgQN+"."+name] = qn
		}
		for name, qn := range file.methods {
			methods[file.pkgQN+"."+name] = qn
			methodsByName[name] = append(methodsByName[name], qn)
		}
	}
	for _, parsed := range files {
		registryImports := goRegistryImports(parsed, localImports)
		for _, declaration := range parsed.file.Decls {
			decl, ok := declaration.(*ast.FuncDecl)
			if !ok || decl.Body == nil {
				continue
			}
			source := parsed.functions[decl.Name.Name]
			if decl.Recv != nil && len(decl.Recv.List) > 0 {
				source = parsed.pkgQN + "." + decl.Name.Name
			}
			ast.Inspect(decl.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				calleeName := goCallName(call.Fun)
				if goBuiltinCall(calleeName) {
					return true
				}
				var target string
				strategy := "go_ast_call"
				confidence := 0.95
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					target = localGoTarget(resolvedIdentifiers[fun], localImports)
					if target != "" {
						strategy, confidence = "go_types", 1
					} else {
						target = functions[parsed.pkgQN+"."+fun.Name]
					}
				case *ast.SelectorExpr:
					target = localGoTarget(resolvedSelectors[fun], localImports)
					if target == "" {
						target = localGoTarget(resolvedIdentifiers[fun.Sel], localImports)
					}
					if target != "" {
						strategy, confidence = "go_types", 1
					}
					if ident, ok := fun.X.(*ast.Ident); ok {
						if importPath, exists := parsed.imports[ident.Name]; exists && target == "" {
							target = "external:" + importPath + "." + fun.Sel.Name
							graph.nodes = append(graph.nodes, api.Node{
								Project: project, Label: "ExternalSymbol", Name: fun.Sel.Name, QualifiedName: target,
								Properties: api.Properties{"import_path": importPath, "external": true},
							})
						} else if target == "" {
							target = methods[parsed.pkgQN+"."+fun.Sel.Name]
						}
					}
					if target == "" && len(methodsByName[fun.Sel.Name]) == 1 {
						target = methodsByName[fun.Sel.Name][0]
						strategy, confidence = "go_unique_method_name", .7
					}
				}
				target = localGoTarget(target, localImports)
				if target == "" || strings.HasPrefix(target, "external:") {
					if resolution := registry.resolve(calleeName, parsed.pkgQN, registryImports); resolution.qn != "" {
						target, strategy, confidence = resolution.qn, resolution.strategy, resolution.confidence
					}
				}
				if source != "" && target != "" {
					if source == target {
						return true
					}
					if strings.HasPrefix(target, "external:") {
						graph.nodes = append(graph.nodes, api.Node{Project: project, Label: "Function", Name: externalShortName(target),
							QualifiedName: target, Properties: api.Properties{"external": true}})
					}
					graph.edges = append(graph.edges, pendingEdge{
						source: source, target: target, kind: "CALLS",
						properties: api.Properties{"local_name": calleeName},
						evidence:   evidenceAt(parsed, call.Pos(), strategy, confidence),
					})
				}
				return true
			})
		}
	}
}

func indexGoReferences(files []*parsedFile, graph *goGraph, registryNodes []api.Node) {
	registry := newSymbolRegistry(registryNodes)
	localImports := localGoImportPackages(files)
	locals := goLocalIdentifiers(files)
	resolvedIdentifiers, resolvedSelectors := resolveGoObjects(files)
	for _, parsed := range files {
		imports := goRegistryImports(parsed, localImports)
		for _, declaration := range parsed.file.Decls {
			source := parsed.fileQN
			var root ast.Node
			switch decl := declaration.(type) {
			case *ast.FuncDecl:
				source, root = parsed.pkgQN+"."+decl.Name.Name, decl
			case *ast.GenDecl:
				root = decl
			}
			if root == nil {
				continue
			}
			walker := &goReferenceWalker{parsed: parsed, source: source, registry: registry, imports: imports,
				locals: locals, graph: graph, localPackages: localImports,
				resolvedIdentifiers: resolvedIdentifiers, resolvedSelectors: resolvedSelectors}
			ast.Walk(walker, root)
		}
	}
}

type goReferenceWalker struct {
	parsed              *parsedFile
	source              string
	registry            symbolRegistry
	imports             map[string]string
	locals              map[*ast.Ident]bool
	graph               *goGraph
	localPackages       map[string]string
	resolvedIdentifiers map[*ast.Ident]string
	resolvedSelectors   map[*ast.SelectorExpr]string
}

func (walker *goReferenceWalker) Visit(node ast.Node) ast.Visitor {
	switch value := node.(type) {
	case nil:
		return walker
	case *ast.CallExpr:
		// A call consumes only the exact callable occurrence. Its receiver,
		// qualifiers, type arguments, ordinary arguments, and nested bodies
		// remain value references in Superopen's unified occurrence walker.
		walker.walkCallOperands(value.Fun)
		for _, argument := range value.Args {
			ast.Walk(walker, argument)
		}
		return nil
	case *ast.FuncDecl:
		walker.walkFieldTypes(value.Recv)
		walker.walkFieldTypes(value.Type.Params)
		walker.walkFieldTypes(value.Type.Results)
		if value.Body != nil {
			ast.Walk(walker, value.Body)
		}
		return nil
	case *ast.AssignStmt:
		for _, expression := range value.Rhs {
			ast.Walk(walker, expression)
		}
		for _, expression := range value.Lhs {
			walker.writeExpression(expression)
		}
		return nil
	case *ast.IncDecStmt:
		walker.writeExpression(value.X)
		return nil
	case *ast.RangeStmt:
		ast.Walk(walker, value.X)
		walker.writeExpression(value.Key)
		walker.writeExpression(value.Value)
		ast.Walk(walker, value.Body)
		return nil
	case *ast.ValueSpec:
		if value.Type != nil {
			ast.Walk(walker, value.Type)
		}
		for _, expression := range value.Values {
			ast.Walk(walker, expression)
		}
		return nil
	case *ast.TypeSpec:
		ast.Walk(walker, value.Type)
		return nil
	case *ast.StructType:
		walker.walkFieldTypes(value.Fields)
		return nil
	case *ast.InterfaceType:
		walker.walkFieldTypes(value.Methods)
		return nil
	case *ast.FuncType:
		walker.walkFieldTypes(value.Params)
		walker.walkFieldTypes(value.Results)
		return nil
	case *ast.Ident:
		walker.readIdentifier(value, false)
		return nil
	case *ast.SelectorExpr:
		if target := localGoTarget(walker.resolvedSelectors[value], walker.localPackages); target != "" && !strings.HasPrefix(target, "external:") {
			walker.appendReference(value.Sel, target, "USAGE")
		} else {
			walker.readIdentifier(value.Sel, true)
		}
		return nil
	default:
		return walker
	}
}

func (walker *goReferenceWalker) walkCallOperands(expression ast.Expr) {
	switch value := expression.(type) {
	case *ast.Ident:
		// Exact callee leaf: represented by CALLS, not USAGE.
		return
	case *ast.SelectorExpr:
		ast.Walk(walker, value.X)
	case *ast.IndexExpr:
		walker.walkCallOperands(value.X)
		ast.Walk(walker, value.Index)
	case *ast.IndexListExpr:
		walker.walkCallOperands(value.X)
		for _, index := range value.Indices {
			ast.Walk(walker, index)
		}
	case *ast.ParenExpr:
		walker.walkCallOperands(value.X)
	case *ast.CallExpr:
		ast.Walk(walker, value)
	default:
		ast.Walk(walker, expression)
	}
}

func (walker *goReferenceWalker) walkFieldTypes(fields *ast.FieldList) {
	if fields == nil {
		return
	}
	for _, field := range fields.List {
		if field.Type != nil {
			ast.Walk(walker, field.Type)
		}
	}
}

func (walker *goReferenceWalker) readIdentifier(identifier *ast.Ident, allowLocal bool) {
	if identifier == nil || identifier.Name == "_" || !allowLocal && walker.locals[identifier] {
		return
	}
	if target := localGoTarget(walker.resolvedIdentifiers[identifier], walker.localPackages); target != "" && !strings.HasPrefix(target, "external:") {
		walker.appendReference(identifier, target, "USAGE")
		return
	}
	resolution := walker.registry.resolve(identifier.Name, walker.parsed.pkgQN, walker.imports)
	if resolution.qn == "" || resolution.qn == walker.source {
		return
	}
	walker.graph.edges = append(walker.graph.edges, pendingEdge{source: walker.source, target: resolution.qn, kind: "USAGE",
		properties: api.Properties{"access": "read", "local_name": identifier.Name},
		evidence:   evidenceAt(walker.parsed, identifier.Pos(), resolution.strategy, resolution.confidence)})
}

func (walker *goReferenceWalker) appendReference(identifier *ast.Ident, target, kind string) {
	if target == "" || strings.HasPrefix(target, "external:") || target == walker.source {
		return
	}
	access := "read"
	if kind == "WRITES" {
		access = "write"
	}
	walker.graph.edges = append(walker.graph.edges, pendingEdge{source: walker.source, target: target, kind: kind,
		properties: api.Properties{"access": access, "local_name": identifier.Name},
		evidence:   evidenceAt(walker.parsed, identifier.Pos(), "go_types", 1)})
}

func (walker *goReferenceWalker) writeExpression(expression ast.Expr) {
	var identifier *ast.Ident
	switch value := expression.(type) {
	case *ast.Ident:
		identifier = value
	case *ast.SelectorExpr:
		if target := localGoTarget(walker.resolvedSelectors[value], walker.localPackages); target != "" && !strings.HasPrefix(target, "external:") {
			walker.appendReference(value.Sel, target, "WRITES")
			return
		}
		identifier = value.Sel
	case *ast.IndexExpr:
		walker.writeExpression(value.X)
		return
	}
	if identifier == nil || identifier.Name == "_" {
		return
	}
	resolution := walker.registry.resolve(identifier.Name, walker.parsed.pkgQN, walker.imports)
	if resolution.qn == "" || resolution.qn == walker.source {
		return
	}
	walker.graph.edges = append(walker.graph.edges, pendingEdge{source: walker.source, target: resolution.qn, kind: "WRITES",
		properties: api.Properties{"access": "write", "local_name": identifier.Name},
		evidence:   evidenceAt(walker.parsed, identifier.Pos(), resolution.strategy, resolution.confidence)})
}

func goLocalIdentifiers(files []*parsedFile) map[*ast.Ident]bool {
	result := map[*ast.Ident]bool{}
	groups := map[string][]*parsedFile{}
	for _, parsed := range files {
		groups[parsed.pkgQN] = append(groups[parsed.pkgQN], parsed)
	}
	for packageQN, group := range groups {
		info := &types.Info{Uses: map[*ast.Ident]types.Object{}}
		astFiles := make([]*ast.File, 0, len(group))
		for _, parsed := range group {
			astFiles = append(astFiles, parsed.file)
		}
		config := types.Config{Importer: importer.Default(), Error: func(error) {}}
		_, _ = config.Check(packageQN, group[0].fset, astFiles, info)
		for identifier, object := range info.Uses {
			if variable, ok := object.(*types.Var); ok && variable.Pkg() != nil && variable.Parent() != nil && variable.Parent() != variable.Pkg().Scope() {
				result[identifier] = true
			}
		}
	}
	return result
}

func goRegistryImports(parsed *parsedFile, localImports map[string]string) map[string]string {
	result := make(map[string]string, len(parsed.imports))
	for alias, importPath := range parsed.imports {
		if local := localImports[importPath]; local != "" {
			result[alias] = local
		} else {
			result[alias] = strings.ReplaceAll(importPath, "/", ".")
		}
	}
	return result
}

func goCallName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		prefix := goCallName(value.X)
		if prefix == "" {
			return value.Sel.Name
		}
		return prefix + "." + value.Sel.Name
	case *ast.CallExpr:
		return goCallName(value.Fun)
	case *ast.IndexExpr:
		return goCallName(value.X)
	case *ast.IndexListExpr:
		return goCallName(value.X)
	case *ast.ParenExpr:
		return goCallName(value.X)
	default:
		return ""
	}
}

func goBuiltinCall(name string) bool {
	if strings.ContainsAny(name, ".:") {
		return false
	}
	switch name {
	case "append", "cap", "clear", "close", "complex", "copy", "delete", "imag", "len", "make", "max", "min",
		"new", "panic", "print", "println", "real", "recover", "bool", "byte", "comparable", "error", "int",
		"int8", "int16", "int32", "int64", "rune", "string", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
		"float32", "float64", "complex64", "complex128", "any":
		return true
	default:
		return false
	}
}

func localGoImportPackages(files []*parsedFile) map[string]string {
	result := make(map[string]string)
	for _, parsed := range files {
		directory := filepath.ToSlash(filepath.Dir(parsed.rel))
		if directory == "." {
			directory = ""
		}
		for _, candidate := range files {
			for _, importPath := range candidate.imports {
				if importPath == directory || directory != "" && strings.HasSuffix(importPath, "/"+directory) {
					result[importPath] = parsed.pkgQN
				}
			}
		}
	}
	return result
}

func localGoTarget(target string, localImports map[string]string) string {
	if !strings.HasPrefix(target, "external:") {
		return target
	}
	qualified := strings.TrimPrefix(target, "external:")
	bestPath := ""
	for importPath := range localImports {
		if strings.HasPrefix(qualified, importPath+".") && len(importPath) > len(bestPath) {
			bestPath = importPath
		}
	}
	if bestPath == "" {
		return target
	}
	return localImports[bestPath] + strings.TrimPrefix(qualified, bestPath)
}

func resolveGoImportEdges(files []*parsedFile, graph *goGraph, seedNodes []api.Node) {
	localImports := localGoImportPackages(files)
	byName := map[string][]api.Node{}
	modulesByQN := map[string]bool{}
	registryNodes := make([]api.Node, 0, len(seedNodes)+len(graph.nodes))
	registryNodes = append(registryNodes, seedNodes...)
	registryNodes = append(registryNodes, graph.nodes...)
	for _, node := range registryNodes {
		if node.Label == "Module" {
			modulesByQN[node.QualifiedName] = true
		}
		if external, _ := node.Properties["external"].(bool); external || !goImportTargetable(node.Label) {
			continue
		}
		byName[node.Name] = append(byName[node.Name], node)
	}
	for name := range byName {
		sort.Slice(byName[name], func(i, j int) bool {
			return byName[name][i].QualifiedName < byName[name][j].QualifiedName
		})
	}
	for index := range graph.edges {
		edge := &graph.edges[index]
		if edge.kind != "IMPORTS" {
			continue
		}
		path, _ := edge.properties["import_path"].(string)
		if packageQN := localImports[path]; packageQN != "" {
			edge.target = packageQN
			var location *api.Location
			if edge.evidence != nil {
				location = edge.evidence.Location
			}
			edge.evidence = &api.Evidence{Strategy: "go_local_import", Confidence: 1, Location: location}
			continue
		}
		if slash := strings.IndexByte(path, '/'); slash > 0 {
			root := path[:slash]
			if modulesByQN[root] {
				edge.target = root
				var location *api.Location
				if edge.evidence != nil {
					location = edge.evidence.Location
				}
				edge.evidence = &api.Evidence{Strategy: "module_path", Confidence: 1, Location: location}
				continue
			}
		}
		// Pinned import resolution falls back through path segments from the
		// leaf toward the root and chooses the lexicographically first
		// importable in-graph definition with that name. This intentionally
		// preserves observable resolutions such as path/filepath -> path.
		segments := strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' || r == '.' })
		for segment := len(segments) - 1; segment >= 0; segment-- {
			candidates := byName[segments[segment]]
			if len(candidates) == 0 {
				continue
			}
			edge.target = candidates[0].QualifiedName
			var location *api.Location
			if edge.evidence != nil {
				location = edge.evidence.Location
			}
			edge.evidence = &api.Evidence{Strategy: "import_symbol_fallback", Confidence: .55, Location: location}
			break
		}
	}
}

func goImportTargetable(label string) bool {
	switch label {
	case "Class", "Interface", "Function", "Method", "Module", "Struct", "Enum", "Trait", "Type", "File":
		return true
	default:
		return false
	}
}

func resolveGoObjects(files []*parsedFile) (map[*ast.Ident]string, map[*ast.SelectorExpr]string) {
	identifiers := map[*ast.Ident]string{}
	selectors := map[*ast.SelectorExpr]string{}
	resolver := newGoSourceImporter(files)
	for _, packagePath := range resolver.paths {
		resolver.check(packagePath)
	}
	for packagePath, info := range resolver.infos {
		group := resolver.groups[packagePath]
		objectNames := map[types.Object]string{}
		for _, parsed := range group {
			for _, declaration := range parsed.file.Decls {
				switch decl := declaration.(type) {
				case *ast.FuncDecl:
					qn := parsed.functions[decl.Name.Name]
					if decl.Recv != nil && len(decl.Recv.List) > 0 {
						qn = parsed.pkgQN + "." + decl.Name.Name
					}
					if object := info.Defs[decl.Name]; object != nil {
						objectNames[object] = qn
					}
				case *ast.GenDecl:
					for _, spec := range decl.Specs {
						switch value := spec.(type) {
						case *ast.TypeSpec:
							if object := info.Defs[value.Name]; object != nil {
								objectNames[object] = parsed.pkgQN + "." + value.Name.Name
							}
						case *ast.ValueSpec:
							for _, name := range value.Names {
								if object := info.Defs[name]; object != nil {
									objectNames[object] = parsed.pkgQN + "." + name.Name
								}
							}
						}
					}
				}
			}
		}
		for ident, object := range info.Uses {
			if qn := goObjectQN(object, objectNames); qn != "" {
				identifiers[ident] = qn
			}
		}
		for selector, selection := range info.Selections {
			if selection != nil {
				selectors[selector] = goObjectQN(selection.Obj(), objectNames)
			} else if object := info.Uses[selector.Sel]; object != nil {
				selectors[selector] = goObjectQN(object, objectNames)
			}
		}
	}
	return identifiers, selectors
}

// goSourceImporter type-checks repository packages from their source ASTs and
// delegates only true external dependencies to the standard importer. Without
// this, a selector on a local imported interface loses its receiver type and a
// weak same-name fallback can bind it to an unrelated method elsewhere.
type goSourceImporter struct {
	groups   map[string][]*parsedFile
	paths    []string
	packages map[string]*types.Package
	infos    map[string]*types.Info
	checking map[string]bool
	fallback types.Importer
}

func newGoSourceImporter(files []*parsedFile) *goSourceImporter {
	byQNPackage := map[string][]*parsedFile{}
	for _, parsed := range files {
		key := parsed.pkgQN + "\x00" + parsed.pkg
		byQNPackage[key] = append(byQNPackage[key], parsed)
	}
	localPaths := localGoImportPackages(files)
	pathByQN := map[string]string{}
	for importPath, qn := range localPaths {
		pathByQN[qn] = importPath
	}
	resolver := &goSourceImporter{groups: map[string][]*parsedFile{}, packages: map[string]*types.Package{},
		infos: map[string]*types.Info{}, checking: map[string]bool{}, fallback: importer.Default()}
	for key, group := range byQNPackage {
		qn, packageName, _ := strings.Cut(key, "\x00")
		packagePath := pathByQN[qn]
		// External test packages occupy the same directory but are distinct Go
		// packages. Giving them a private checking path prevents one malformed
		// mixed package from discarding semantic information for both groups.
		if packagePath != "" && strings.HasSuffix(packageName, "_test") {
			packagePath += "#" + packageName
		} else if packagePath == "" {
			packagePath = "so-graph.local/" + qn
			if strings.HasSuffix(packageName, "_test") {
				packagePath += "#" + packageName
			}
		}
		resolver.groups[packagePath] = group
		resolver.paths = append(resolver.paths, packagePath)
	}
	sort.Strings(resolver.paths)
	return resolver
}

func (resolver *goSourceImporter) Import(packagePath string) (*types.Package, error) {
	if resolver.groups[packagePath] != nil {
		return resolver.check(packagePath)
	}
	return resolver.fallback.Import(packagePath)
}

func (resolver *goSourceImporter) check(packagePath string) (*types.Package, error) {
	if checked := resolver.packages[packagePath]; checked != nil {
		return checked, nil
	}
	group := resolver.groups[packagePath]
	if len(group) == 0 {
		return resolver.fallback.Import(packagePath)
	}
	if resolver.checking[packagePath] {
		return types.NewPackage(packagePath, group[0].pkg), nil
	}
	resolver.checking[packagePath] = true
	defer delete(resolver.checking, packagePath)
	info := &types.Info{Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{}}
	astFiles := make([]*ast.File, 0, len(group))
	for _, parsed := range group {
		astFiles = append(astFiles, parsed.file)
	}
	config := types.Config{Importer: resolver, Error: func(error) {}}
	checked, err := config.Check(packagePath, group[0].fset, astFiles, info)
	if checked == nil {
		checked = types.NewPackage(packagePath, group[0].pkg)
	}
	resolver.packages[packagePath] = checked
	resolver.infos[packagePath] = info
	return checked, err
}

func goObjectQN(object types.Object, locals map[types.Object]string) string {
	if object == nil {
		return ""
	}
	if qn := locals[object]; qn != "" {
		return qn
	}
	if object.Pkg() == nil {
		return ""
	}
	if variable, ok := object.(*types.Var); ok && variable.Parent() != nil && variable.Parent() != object.Pkg().Scope() {
		return ""
	}
	return "external:" + object.Pkg().Path() + "." + object.Name()
}

func goResolutionStrategy(resolved string) string {
	if resolved != "" {
		return "go_types"
	}
	return "go_ast_identifier"
}

func goResolutionConfidence(resolved string) float64 {
	if resolved != "" {
		return 1
	}
	return 0.9
}

func externalShortName(qn string) string {
	if index := strings.LastIndexByte(qn, '.'); index >= 0 {
		return qn[index+1:]
	}
	return strings.TrimPrefix(qn, "external:")
}

func indexGoTypeRelationships(files []*parsedFile, graph *goGraph) {
	type methodSet map[string]string
	structMethods := map[string]methodSet{}
	interfaces := map[string]methodSet{}
	packageTypes := map[string]map[string]string{}
	for _, parsed := range files {
		if packageTypes[parsed.pkgQN] == nil {
			packageTypes[parsed.pkgQN] = map[string]string{}
		}
		for name, qn := range parsed.types {
			packageTypes[parsed.pkgQN][name] = qn
		}
	}
	for _, parsed := range files {
		for _, declaration := range parsed.file.Decls {
			switch decl := declaration.(type) {
			case *ast.FuncDecl:
				if decl.Recv == nil || len(decl.Recv.List) == 0 {
					continue
				}
				receiver := parsed.pkgQN + "." + receiverName(decl.Recv.List[0].Type)
				if structMethods[receiver] == nil {
					structMethods[receiver] = methodSet{}
				}
				structMethods[receiver][decl.Name.Name] = parsed.pkgQN + "." + decl.Name.Name
			case *ast.GenDecl:
				for _, spec := range decl.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					source := parsed.pkgQN + "." + typeSpec.Name.Name
					switch value := typeSpec.Type.(type) {
					case *ast.StructType:
						for _, field := range value.Fields.List {
							if len(field.Names) != 0 {
								continue
							}
							if target := localTypeQN(packageTypes[parsed.pkgQN], field.Type); target != "" {
								graph.edges = append(graph.edges, pendingEdge{source: source, target: target, kind: "INHERITS", evidence: evidenceAt(parsed, field.Pos(), "go_type_embedding", 1)})
							}
						}
					case *ast.InterfaceType:
						set := methodSet{}
						for _, field := range value.Methods.List {
							for _, name := range field.Names {
								set[name.Name] = source + "." + name.Name
							}
							if len(field.Names) == 0 {
								if target := localTypeQN(packageTypes[parsed.pkgQN], field.Type); target != "" {
									graph.edges = append(graph.edges, pendingEdge{source: source, target: target, kind: "INHERITS", evidence: evidenceAt(parsed, field.Pos(), "go_interface_embedding", 1)})
								}
							}
						}
						interfaces[source] = set
					}
				}
			}
		}
	}
	for concrete, methods := range structMethods {
		for iface, required := range interfaces {
			if concrete == iface || len(required) == 0 || !containsMethods(methods, required) {
				continue
			}
			graph.edges = append(graph.edges, pendingEdge{
				source: concrete, target: iface, kind: "IMPLEMENTS",
				properties: api.Properties{"method_count": len(required)},
				evidence:   &api.Evidence{Strategy: "go_method_set", Confidence: 0.9},
			})
			for name, interfaceMethod := range required {
				graph.edges = append(graph.edges, pendingEdge{
					source: methods[name], target: interfaceMethod, kind: "OVERRIDE",
					evidence: &api.Evidence{Strategy: "go_method_set", Confidence: 0.9},
				})
			}
		}
	}
}

func functionProperties(parsed *parsedFile, decl *ast.FuncDecl) api.Properties {
	signature, returnType := goFunctionSignature(parsed.fset, decl.Type)
	paramNames, paramTypes := goFieldMetadata(parsed.fset, decl.Type.Params)
	properties := api.Properties{
		"signature": signature, "param_count": len(paramNames),
		"param_names":    paramNames,
		"param_types":    paramTypes,
		"is_test":        strings.HasSuffix(parsed.rel, "_test.go") || strings.HasPrefix(decl.Name.Name, "Test"),
		"is_entry_point": decl.Name.Name == "main",
		"is_exported":    ast.IsExported(decl.Name.Name),
	}
	if returnType != "" {
		properties["return_type"] = returnType
	}
	if decl.Body != nil {
		properties["bt"] = goBodyIdentifierTokens(decl.Body)
	}
	if decl.Doc != nil {
		lines := strings.Split(strings.TrimSpace(decl.Doc.Text()), "\n")
		properties["docstring"] = lines[len(lines)-1]
	}
	complexity, loopCount, loopDepth := goComplexity(decl.Body)
	properties["complexity"] = complexity
	properties["loop_count"] = loopCount
	properties["loop_depth"] = loopDepth
	return properties
}

func goFunctionSignature(fileSet *token.FileSet, function *ast.FuncType) (string, string) {
	if function == nil {
		return "", ""
	}
	var rendered bytes.Buffer
	_ = format.Node(&rendered, fileSet, function)
	text := strings.TrimPrefix(rendered.String(), "func")
	if text == "" || text[0] != '(' {
		return text, ""
	}
	depth := 0
	for index, char := range text {
		switch char {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return text[:index+1], strings.TrimSpace(text[index+1:])
			}
		}
	}
	return text, ""
}

func goBodyIdentifierTokens(body *ast.BlockStmt) string {
	if body == nil {
		return ""
	}
	seen := make(map[string]bool, 128)
	tokens := make([]string, 0, 128)
	length := 0
	ast.Inspect(body, func(node ast.Node) bool {
		if len(tokens) >= 128 {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok || identifier.Name == "" || len(identifier.Name) >= 64 || seen[identifier.Name] ||
			identifier.Name == "nil" || identifier.Name == "true" || identifier.Name == "false" {
			return true
		}
		additional := len(identifier.Name)
		if len(tokens) > 0 {
			additional++
		}
		if length+additional >= 2048 {
			return false
		}
		seen[identifier.Name] = true
		tokens = append(tokens, identifier.Name)
		length += additional
		return true
	})
	return strings.Join(tokens, " ")
}

func goFieldMetadata(fileSet *token.FileSet, fields *ast.FieldList) ([]string, []string) {
	if fields == nil {
		return []string{}, []string{}
	}
	names := make([]string, 0, len(fields.List))
	types := make([]string, 0, len(fields.List))
	for _, field := range fields.List {
		if len(field.Names) == 0 {
			names = append(names, "")
		} else {
			names = append(names, field.Names[0].Name)
		}
		if typeName := goSemanticParamType(fileSet, field.Type); typeName != "" {
			types = append(types, typeName)
		}
	}
	return names, types
}

func goSemanticParamType(fileSet *token.FileSet, expression ast.Expr) string {
	for {
		pointer, ok := expression.(*ast.StarExpr)
		if !ok {
			break
		}
		expression = pointer.X
	}
	switch value := expression.(type) {
	case *ast.SelectorExpr:
		var rendered bytes.Buffer
		_ = format.Node(&rendered, fileSet, value)
		return rendered.String()
	case *ast.Ident:
		switch value.Name {
		case "string", "bool", "byte", "rune", "error", "int", "int8", "int16", "int32", "int64",
			"uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "float32", "float64", "complex64", "complex128", "any":
			return ""
		default:
			return value.Name
		}
	case *ast.MapType:
		return "map"
	case *ast.FuncType:
		var rendered bytes.Buffer
		_ = format.Node(&rendered, fileSet, value)
		text := rendered.String()
		if comma := strings.IndexByte(text, ','); comma >= 0 {
			return text[:comma+1]
		}
		return text
	default:
		return ""
	}
}

func fieldCount(fields *ast.FieldList) int {
	if fields == nil {
		return 0
	}
	count := 0
	for _, field := range fields.List {
		if len(field.Names) == 0 {
			count++
		} else {
			count += len(field.Names)
		}
	}
	return count
}

func goComplexity(body *ast.BlockStmt) (complexity, loopCount, maxLoopDepth int) {
	complexity = 1
	if body == nil {
		return complexity, 0, 0
	}
	depth := 0
	var visit func(ast.Node)
	visit = func(node ast.Node) {
		if node == nil {
			return
		}
		isLoop := false
		switch node.(type) {
		case *ast.IfStmt, *ast.CaseClause, *ast.CommClause:
			complexity++
		case *ast.ForStmt, *ast.RangeStmt:
			complexity++
			loopCount++
			depth++
			isLoop = true
		case *ast.BinaryExpr:
			expr := node.(*ast.BinaryExpr)
			if expr.Op == token.LAND || expr.Op == token.LOR {
				complexity++
			}
		}
		if depth > maxLoopDepth {
			maxLoopDepth = depth
		}
		ast.Inspect(node, func(child ast.Node) bool {
			if child == node {
				return true
			}
			visit(child)
			return false
		})
		if isLoop {
			depth--
		}
	}
	visit(body)
	return complexity, loopCount, maxLoopDepth
}

func localTypeQN(types map[string]string, expr ast.Expr) string {
	name := receiverName(expr)
	return types[name]
}

func qnPackage(qn string) string {
	if index := strings.LastIndexByte(qn, '.'); index >= 0 {
		return qn[:index]
	}
	return qn
}

func goPackageQN(rel, packageName string) string {
	directory := filepath.ToSlash(filepath.Dir(rel))
	if directory == "." || directory == "" {
		return packageName
	}
	return strings.ReplaceAll(strings.TrimPrefix(directory, "./"), "/", ".")
}

func fileQualifiedName(rel string) string {
	return strings.ReplaceAll(filepath.ToSlash(rel), "/", ".") + ".__file__"
}

func folderQualifiedName(rel string) string {
	return strings.ReplaceAll(filepath.ToSlash(rel), "/", ".")
}

func containsMethods(have, required map[string]string) bool {
	for name := range required {
		if have[name] == "" {
			return false
		}
	}
	return true
}

func hasNode(nodes []api.Node, qn string) bool {
	for _, node := range nodes {
		if node.QualifiedName == qn {
			return true
		}
	}
	return false
}

func separateUnresolvedRelationships(graph *goGraph) {
	known := make(map[string]bool, len(graph.nodes))
	keptNodes := graph.nodes[:0]
	for _, node := range graph.nodes {
		if (semanticPropertyBool(node.Properties, "external") && node.Label != "Package") || semanticPropertyBool(node.Properties, "unresolved") {
			continue
		}
		known[node.QualifiedName] = true
		keptNodes = append(keptNodes, node)
	}
	keptEdges := graph.edges[:0]
	for _, edge := range graph.edges {
		if known[edge.source] && known[edge.target] {
			keptEdges = append(keptEdges, edge)
			continue
		}
		if known[edge.source] {
			graph.unresolved = append(graph.unresolved, edge)
		}
	}
	graph.nodes = keptNodes
	graph.edges = keptEdges
	sortGraph(graph)
}

func edgeProject(graph *goGraph) string {
	if len(graph.nodes) > 0 {
		return graph.nodes[0].Project
	}
	return ""
}

func nodeAt(project, label, name, qn string, parsed *parsedFile, start, end token.Pos, props api.Properties) api.Node {
	startPos := parsed.fset.Position(start)
	endPos := parsed.fset.Position(end)
	return api.Node{
		Project: project, Label: label, Name: name, QualifiedName: qn,
		Location:   api.Location{File: parsed.rel, StartLine: startPos.Line, StartColumn: startPos.Column, EndLine: endPos.Line, EndColumn: endPos.Column},
		Properties: props,
	}
}

func evidenceAt(parsed *parsedFile, pos token.Pos, strategy string, confidence float64) *api.Evidence {
	p := parsed.fset.Position(pos)
	return &api.Evidence{
		Strategy: strategy, Confidence: confidence,
		Location: &api.Location{File: parsed.rel, StartLine: p.Line, StartColumn: p.Column},
	}
}

func receiverName(expr ast.Expr) string {
	switch value := expr.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.StarExpr:
		return receiverName(value.X)
	case *ast.IndexExpr:
		return receiverName(value.X)
	case *ast.IndexListExpr:
		return receiverName(value.X)
	default:
		return "Receiver"
	}
}

func sortGraph(graph *goGraph) {
	sort.Slice(graph.files, func(i, j int) bool { return graph.files[i].Path < graph.files[j].Path })
	// The pinned graph buffer keeps the first definition encountered for a
	// colliding qualified name. File traversal and pass order are deterministic,
	// so a stable sort preserves that observable winner.
	sort.SliceStable(graph.nodes, func(i, j int) bool { return graph.nodes[i].QualifiedName < graph.nodes[j].QualifiedName })
	unique := graph.nodes[:0]
	for _, node := range graph.nodes {
		if len(unique) > 0 && unique[len(unique)-1].QualifiedName == node.QualifiedName {
			prior := &unique[len(unique)-1]
			// The pinned upsert keeps the earliest file in repository order,
			// while a later declaration in that same file replaces an earlier
			// colliding declaration.
			if node.Location.File < prior.Location.File || node.Location.File == prior.Location.File &&
				(nodeCollisionPriority(node.Label) >= nodeCollisionPriority(prior.Label)) {
				*prior = node
			}
			continue
		}
		unique = append(unique, node)
	}
	graph.nodes = unique
	uniqueEdgesHashSet(graph)
	sort.Slice(graph.edges, func(i, j int) bool {
		return graphEdgeLessPair(graph.edges[i], graph.edges[j])
	})
}

func uniqueEdgesHashSet(graph *goGraph) {
	if len(graph.edges) == 0 {
		graph.edgeSeen = map[edgeKey]struct{}{}
		return
	}
	intern := graph.ensureIntern()
	seen := make(map[edgeKey]struct{}, len(graph.edges))
	unique := graph.edges[:0]
	for _, edge := range graph.edges {
		key := edgeIdentityKey(intern, edge)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, edge)
	}
	graph.edges = unique
	graph.edgeSeen = seen
}

func graphEdgeIdentity(edge pendingEdge) string {
	key := edge.source + "\x00" + edge.kind + "\x00" + edge.target
	if edge.kind == "IMPORTS" {
		localName, _ := edge.properties["local_name"].(string)
		key += "\x00" + localName
	}
	return key
}

func graphEdgeLessPair(a, b pendingEdge) bool {
	if a.source != b.source {
		return a.source < b.source
	}
	if a.kind != b.kind {
		return a.kind < b.kind
	}
	if a.target != b.target {
		return a.target < b.target
	}
	if a.kind == "IMPORTS" {
		left, _ := a.properties["local_name"].(string)
		right, _ := b.properties["local_name"].(string)
		return left < right
	}
	return false
}

func nodeCollisionPriority(label string) int {
	switch label {
	case "Variable":
		return 2
	case "Function":
		return 1
	default:
		return 0
	}
}

func discoverTrackedFiles(ctx context.Context, root string, excludes []string) ([]string, error) {
	ignore, err := loadGraphIgnore(root, excludes)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "git", "-C", root, "ls-files", "-co", "--exclude-standard", "-z")
	if output, err := cmd.Output(); err == nil {
		parts := bytes.Split(output, []byte{0})
		files := make([]string, 0, len(parts))
		for _, part := range parts {
			rel := filepath.ToSlash(string(part))
			regular := safeRegularFile(root, rel)
			if rel != "" && regular && !managedPath(rel) && !hardIgnoredPath(rel) && !hardIgnoredFile(rel) && !ignore.Match(rel) {
				files = append(files, rel)
			}
		}
		return discoveryOrder(root, files), nil
	}
	var files []string
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "." {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if managedPath(rel+"/") || hardIgnoredPath(rel+"/") || ignore.Match(rel+"/") {
				return filepath.SkipDir
			}
			return nil
		}
		if !managedPath(rel) && !hardIgnoredPath(rel) && !hardIgnoredFile(rel) && !ignore.Match(rel) && safeRegularFile(root, rel) {
			files = append(files, rel)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

// discoveryOrder reproduces the pinned engine's observable iterative
// directory walk: files in the current directory are emitted in readdir order,
// while discovered subdirectories are processed as a LIFO stack. Git remains
// authoritative for the allowed set, so this changes ordering only—not ignore
// or repository-boundary behavior.
func discoveryOrder(root string, allowed []string) []string {
	wanted := make(map[string]bool, len(allowed))
	for _, path := range allowed {
		wanted[filepath.ToSlash(path)] = true
	}
	type frame struct{ abs, rel string }
	stack := []frame{{abs: root}}
	ordered := make([]string, 0, len(allowed))
	seen := make(map[string]bool, len(allowed))
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		directory, err := os.Open(current.abs)
		if err != nil {
			continue
		}
		entries, readErr := directory.Readdir(-1)
		_ = directory.Close()
		if readErr != nil {
			continue
		}
		for _, entry := range entries {
			name := entry.Name()
			if name == "." || name == ".." || entry.Mode()&os.ModeSymlink != 0 {
				continue
			}
			rel := name
			if current.rel != "" {
				rel = current.rel + "/" + name
			}
			rel = filepath.ToSlash(rel)
			if entry.IsDir() {
				if managedPath(rel+"/") || hardIgnoredPath(rel+"/") {
					continue
				}
				stack = append(stack, frame{abs: filepath.Join(current.abs, name), rel: rel})
				continue
			}
			if wanted[rel] && !seen[rel] {
				seen[rel] = true
				ordered = append(ordered, rel)
			}
		}
	}
	if len(ordered) != len(allowed) {
		var remainder []string
		for _, path := range allowed {
			path = filepath.ToSlash(path)
			if !seen[path] {
				remainder = append(remainder, path)
			}
		}
		sort.Strings(remainder)
		ordered = append(ordered, remainder...)
	}
	return ordered
}

func safeRegularFile(root, rel string) bool {
	abs := filepath.Join(root, filepath.FromSlash(rel))
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return false
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	inside, err := filepath.Rel(canonicalRoot, resolved)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return false
	}
	info, err := os.Stat(resolved)
	return err == nil && info.Mode().IsRegular()
}

func managedPath(rel string) bool {
	// Only exclude Superopen-managed product paths. Superopen indexes .codex/,
	// .claude/, AGENTS.md, and CLAUDE.md when present in a repository, so those
	// must remain visible for graph readiness.
	rel = filepath.ToSlash(rel)
	for _, prefix := range []string{".so/", ".git/"} {
		if strings.HasPrefix(rel, prefix) {
			return true
		}
	}
	return false
}

func hardIgnoredFile(rel string) bool {
	name := filepath.Base(rel)
	for _, suffix := range []string{
		".tmp", "~", ".pyc", ".pyo", ".o", ".a", ".so", ".dll", ".class",
		".png", ".jpg", ".jpeg", ".gif", ".ico", ".bmp", ".tiff", ".webp", ".svg",
		".wasm", ".node", ".exe", ".bin", ".dat", ".db", ".sqlite", ".sqlite3",
		".woff", ".woff2", ".ttf", ".eot", ".otf",
	} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	if strings.EqualFold(filepath.Ext(name), ".json") {
		for _, ignored := range []string{"package.json", "package-lock.json", "tsconfig.json", "jsconfig.json", "composer.json", "composer.lock", "yarn.lock", "openapi.json", "swagger.json", "jest.config.json", ".eslintrc.json", ".prettierrc.json", ".babelrc.json", "tslint.json", "angular.json", "firebase.json", "renovate.json", "lerna.json", "turbo.json", ".stylelintrc.json", "pnpm-lock.json", "deno.json", "biome.json", "devcontainer.json", ".devcontainer.json", "launch.json", "settings.json", "extensions.json", "tasks.json"} {
			if name == ignored {
				return true
			}
		}
	}
	return false
}

func hardIgnoredPath(rel string) bool {
	for _, segment := range strings.Split(strings.Trim(filepath.ToSlash(rel), "/"), "/") {
		for _, ignored := range []string{
			".git", ".hg", ".svn", ".worktrees", ".idea", ".vs", ".vscode", ".eclipse", ".claude", ".claude-worktrees", "Antigravity",
			".cache", ".eggs", ".env", ".mypy_cache", ".nox", ".pytest_cache", ".ruff_cache", ".tox", ".venv", "__pycache__", "env", "htmlcov", "site-packages", "venv",
			".npm", ".nyc_output", ".pnpm-store", ".yarn", "bower_components", "coverage", "node_modules", ".next", ".nuxt", ".svelte-kit", ".angular", ".turbo", ".parcel-cache", ".docusaurus", ".expo",
			"dist", "obj", "Pods", "target", "temp", "tmp", ".terraform", ".serverless", "bazel-bin", "bazel-out", "bazel-testlogs",
			".cargo", ".stack-work", ".dart_tool", "zig-cache", "zig-out", ".metals", ".bloop", ".bsp", ".ccls-cache", ".clangd", "elm-stuff", "_opam", ".cpcache", ".shadow-cljs",
			".vercel", ".netlify", "deploy", "deployed", ".qdrant_code_embeddings", ".tmp", "vendor", "vendored",
		} {
			if segment == ignored {
				return true
			}
		}
	}
	return false
}

func isLikelySource(rel string) bool {
	ext := strings.ToLower(filepath.Ext(rel))
	if ext == "" {
		return false
	}
	for _, nonSource := range []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".svg", ".pdf", ".zip", ".gz", ".sum", ".lock"} {
		if ext == nonSource {
			return false
		}
	}
	return true
}

func gitRevision(ctx context.Context, root string) string {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "HEAD")
	if output, err := cmd.Output(); err == nil {
		return strings.TrimSpace(string(output))
	}
	return ""
}
