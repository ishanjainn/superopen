package engine

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// IndexAllDevelopment is the private all-language build path. Every language,
// including Go, now crosses the same Tree-sitter FileResult boundary. Family
// resolvers may enrich that result in pinned pass order, but no language gets a
// separate graph whose stricter identities change observable Superopen output.
func IndexAllDevelopment(ctx context.Context, request api.BuildRequest, engineVersion string, assets fs.FS) (api.BuildResult, error) {
	started := time.Now()
	if assets == nil {
		return api.BuildResult{}, fmt.Errorf("complete engine assets are required")
	}
	root, err := CanonicalRoot(request.RepoRoot)
	if err != nil {
		return api.BuildResult{}, err
	}
	if err := MigrateLegacyCacheIfNeeded(root); err != nil {
		return api.BuildResult{}, err
	}
	applyMemoryBudget()
	project := request.Project
	if project == "" {
		project, err = ProjectName(root)
		if err != nil {
			return api.BuildResult{}, err
		}
	}
	if !request.Force {
		if !databaseExists(root) {
			// First init has no prior generation; skip the SHA walk.
		} else if unchanged, ok, err := tryUnchangedBuild(ctx, root, project, request.Exclude, started); err != nil {
			return api.BuildResult{}, err
		} else if ok {
			return unchanged, nil
		}
	}
	files, err := discoverTrackedFiles(ctx, root, request.Exclude)
	if err != nil {
		return api.BuildResult{}, err
	}
	reportIndexProgress("Building native graph...")
	grammars := grammarsForFiles(root, files, nil)
	var grammarRuntime *GrammarRuntime
	if len(grammars) == 0 {
		grammarRuntime, _, err = loadGrammarAssets(ctx, assets, "assets/grammars/manifest.json", false)
	} else {
		reportIndexProgress("Loading %d grammars...", len(grammars))
		grammarRuntime, _, err = loadSelectedGrammarAssets(ctx, assets, "assets/grammars/manifest.json", false, grammars)
	}
	if err != nil {
		return api.BuildResult{}, err
	}
	defer grammarRuntime.Close(ctx)
	parser := SyntaxParser(grammarRuntime)
	if native := nativeSyntaxParser(); native != nil {
		parser = &fallbackSyntaxParser{native: native, wasm: grammarRuntime}
	}
	workers := parseWorkerCount()
	reportIndexProgress("parse 0/%d workers=%d", len(files), workers)
	parseStarted := time.Now()
	repository, err := ParseSyntaxRepository(ctx, parser, root, project, files, nil, workers)
	if err != nil {
		return api.BuildResult{}, err
	}
	reportIndexProgress("parse done files=%d elapsed=%s", len(repository.Files), indexElapsed(parseStarted))
	fileOrder := make(map[string]int, len(files))
	for index, path := range files {
		fileOrder[path] = index
	}
	sort.SliceStable(repository.Files, func(i, j int) bool {
		return fileOrder[repository.Files[i].File.Path] < fileOrder[repository.Files[j].File.Path]
	})
	assembleStarted := time.Now()
	if err := enrichCResolvedCalls(ctx, repository.Files); err != nil {
		return api.BuildResult{}, err
	}
	graph, coverage := AssembleSyntaxGraph(repository, project)
	registry := newSymbolRegistry(graph.nodes)
	reportIndexProgress("assemble overlays...")
	overlayStarted := time.Now()
	clockOverlay := func(name string, fn func()) {
		started := time.Now()
		fn()
		reportIndexProgress("assemble %s elapsed=%s", name, indexElapsed(started))
	}
	clockOverlay("env", func() { indexEnvironmentAccesses(project, root, repository.Files, &graph) })
	clockOverlay("pkgmap", func() { applyPackageMap(project, repository, &graph) })
	joinResolvedCalls(&graph, repository.Files, registry)
	clockOverlay("inheritance", func() { indexSyntaxInheritance(project, repository.Files, &graph, registry) })
	clockOverlay("implements", func() { indexSyntaxGoImplements(&graph) })
	indexSyntaxExplicitOverrides(&graph)
	indexPythonBuiltins(project, repository.Files, &graph)
	indexRepositoryBranch(ctx, root, project, &graph)
	clockOverlay("configlinks", func() { indexConfigLinks(&graph) })
	clockOverlay("routes", func() { indexHTTPRoutes(project, repository.Files, &graph) })
	dropExtractedOccurrences(repository.Files)
	repository.Files = nil
	appendTestRelationships(&graph)
	clockOverlay("git", func() { indexGitCochange(ctx, root, project, &graph) })
	annotateCrossRepositoryEdges(&graph)
	reportIndexProgress("assemble overlays elapsed=%s", indexElapsed(overlayStarted))
	separateUnresolvedRelationships(&graph)
	reportIndexProgress("assemble done nodes=%d edges=%d elapsed=%s", len(graph.nodes), len(graph.edges), indexElapsed(assembleStarted))
	model, err := loadPinnedPretrainedVectors(assets, "assets/model/code_tokens.txt", "assets/model/code_vectors.bin")
	if err != nil {
		return api.BuildResult{}, err
	}
	revision := gitRevision(ctx, root)
	if request.ExpectedSource != "" && request.ExpectedSource != revision {
		return api.BuildResult{}, fmt.Errorf("source revision changed: expected %s, found %s", request.ExpectedSource, revision)
	}
	var changes *api.ChangeSet
	if request.Incremental {
		planned, err := PlanIncremental(ctx, root, project, request.Exclude)
		if err != nil {
			return api.BuildResult{}, err
		}
		changes = &planned
	}
	database, err := publishDevelopmentGraph(ctx, root, project, engineVersion, revision, repository.Generation, graph, coverage, model, request.Incremental && !request.Force)
	if err != nil {
		if errors.Is(err, ErrBuildInProgress) {
			return api.BuildResult{Status: "refresh_in_progress", Project: project, Duration: time.Since(started)}, nil
		}
		if errors.Is(err, ErrBuildPoolFull) {
			return api.BuildResult{Status: "pool_full", Project: project, Duration: time.Since(started)}, nil
		}
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

func tryUnchangedBuild(ctx context.Context, root, project string, excludes []string, started time.Time) (api.BuildResult, bool, error) {
	planned, err := PlanIncremental(ctx, root, project, excludes)
	if err != nil {
		return api.BuildResult{}, false, nil
	}
	if planned.RequiresFull || planned.RevisionChanged || changeVolume(planned) > 0 {
		return api.BuildResult{}, false, nil
	}
	paths, err := CachePaths(root)
	if err != nil {
		return api.BuildResult{}, false, err
	}
	store, err := OpenReadOnly(paths.Database)
	if err != nil {
		return api.BuildResult{}, false, nil
	}
	defer store.Close()
	status, err := store.Status(ctx, project)
	if err != nil || status.State == "missing" || status.NodeCount == 0 {
		return api.BuildResult{}, false, nil
	}
	return api.BuildResult{
		Status:         "unchanged",
		Project:        project,
		Database:       paths.Database,
		SourceRevision: planned.SourceRevision,
		Generation:     status.Generation,
		NodeCount:      status.NodeCount,
		EdgeCount:      status.EdgeCount,
		FileCount:      status.FileCount,
		Duration:       time.Since(started),
		Changes:        &planned,
	}, true, nil
}

func changeVolume(changes api.ChangeSet) int {
	return len(changes.Added) + len(changes.Modified) + len(changes.Deleted) + len(changes.Renamed)
}

func publishedCounts(ctx context.Context, database, project string) (int, int, error) {
	store, err := OpenReadOnly(database)
	if err != nil {
		return 0, 0, err
	}
	defer store.Close()
	status, err := store.Status(ctx, project)
	if err != nil {
		return 0, 0, err
	}
	return status.NodeCount, status.EdgeCount, nil
}

func summarizedCoverage(coverage api.Coverage, limit int) api.Coverage {
	coverage.Total = len(coverage.Rows)
	if limit > 0 && len(coverage.Rows) > limit {
		coverage.Rows = append([]api.CoverageRow(nil), coverage.Rows[:limit]...)
		coverage.Truncated = true
	}
	return coverage
}

func replaceGenericGoGraph(generic, resolved goGraph, goPaths map[string]bool) goGraph {
	goFiles := make(map[string]bool, len(goPaths))
	for path := range goPaths {
		goFiles[fileQualifiedName(path)] = true
	}
	files := make([]FileRecord, 0, len(generic.files)+len(resolved.files))
	for _, file := range generic.files {
		if !goPaths[filepath.ToSlash(file.Path)] {
			files = append(files, file)
		}
	}
	files = append(files, resolved.files...)
	resolvedQN := map[string]string{}
	resolvedByIdentity := map[string]string{}
	for _, node := range resolved.nodes {
		resolvedQN[node.QualifiedName] = node.Label
		key := replacementNodeIdentity(node)
		if prior, exists := resolvedByIdentity[key]; !exists {
			resolvedByIdentity[key] = node.QualifiedName
		} else if prior != node.QualifiedName {
			resolvedByIdentity[key] = ""
		}
	}
	removedQN := map[string]bool{}
	replacementQN := map[string]string{}
	nodes := generic.nodes[:0]
	for _, node := range generic.nodes {
		remove := resolvedQN[node.QualifiedName] == node.Label || strings.HasPrefix(node.QualifiedName, "go:") ||
			strings.HasPrefix(node.QualifiedName, "external:call:go:") || goPaths[filepath.ToSlash(node.Location.File)]
		if remove {
			removedQN[node.QualifiedName] = true
			if replacement := resolvedByIdentity[replacementNodeIdentity(node)]; replacement != "" {
				replacementQN[node.QualifiedName] = replacement
			}
			continue
		}
		nodes = append(nodes, node)
	}
	edges := generic.edges[:0]
	for _, edge := range generic.edges {
		layout := edge.kind == "CONTAINS_FOLDER" || edge.kind == "CONTAINS_FILE"
		originalSource := edge.source
		originalTarget := edge.target
		if replacement := replacementQN[originalSource]; replacement != "" {
			edge.source = replacement
		}
		if replacement := replacementQN[originalTarget]; replacement != "" {
			edge.target = replacement
		}
		sourceWasReplaced := removedQN[originalSource] && resolvedQN[edge.source] == ""
		targetWasReplaced := removedQN[originalTarget] && resolvedQN[edge.target] == ""
		preserveSemantic := preserveGenericGoRelationship(edge.kind)
		if goFiles[originalSource] && !preserveSemantic ||
			!layout && (sourceWasReplaced || targetWasReplaced || removedQN[originalSource] && !preserveSemantic) {
			continue
		}
		edges = append(edges, edge)
	}
	generic.nodes = append(nodes, resolved.nodes...)
	for _, edge := range resolved.edges {
		if preserveGenericGoRelationship(edge.kind) {
			continue
		}
		edges = append(edges, edge)
	}
	generic.edges = edges
	generic.unresolved = append(generic.unresolved, resolved.unresolved...)
	generic.files = files
	sortGraph(&generic)
	return generic
}

func preserveGenericGoRelationship(kind string) bool {
	switch kind {
	case "USAGE", "WRITES", "READS", "CALL_REFERENCE", "RAISES", "THROWS":
		return true
	default:
		return false
	}
}

func replacementNodeIdentity(node api.Node) string {
	label := node.Label
	switch label {
	case "Class", "Struct", "Interface", "Enum", "Type", "Trait":
		// The generic grammar pass can only prove that a declaration is a
		// type. Family resolvers refine that declaration to its concrete kind.
		// They are the same source symbol for replacement and incoming edges
		// from other languages must survive the refinement.
		label = "Type"
	case "Function", "Method":
		// Some grammars do not expose receiver ownership on the generic pass;
		// the family resolver upgrades the same declaration to Method.
		label = "Callable"
	}
	return filepath.ToSlash(node.Location.File) + "\x00" + label + "\x00" + node.Name
}

func publishDevelopmentGraph(ctx context.Context, root, project, engineVersion, revision, generation string, graph goGraph, coverage api.Coverage, model *pretrainedVectors, nonBlocking bool) (string, error) {
	publishFn := Publish
	if nonBlocking {
		publishFn = PublishNonBlocking
	}
	return publishFn(ctx, root, func(ctx context.Context, path string) error {
		store, err := OpenWritableFresh(path)
		if err != nil {
			return err
		}
		buildErr := store.BuildFresh(ctx, func(builder *Builder) error {
			if err := builder.PutProject(ProjectRecord{Name: project, RootPath: root, Generation: generation,
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
				if (i+1)%dumpPartitionSize == 0 {
					reportIndexProgress("dump %d/%d", i+1, len(graph.nodes))
				}
			}
			if err := builder.flushNodeBatch(); err != nil {
				return err
			}
			semanticsStarted := time.Now()
			if err := putGraphSemantics(builder, graph, ids, project, model); err != nil {
				return err
			}
			if err := emitSemanticEdges(builder, graph, ids, project); err != nil {
				return err
			}
			reportIndexProgress("semantics elapsed=%s", indexElapsed(semanticsStarted))
			for i := range graph.nodes {
				graph.nodes[i].Properties = nil
			}
			for _, edge := range graph.edges {
				source, sourceOK := ids[edge.source]
				target, targetOK := ids[edge.target]
				if !sourceOK || !targetOK {
					return fmt.Errorf("graph edge endpoint missing: %s -[%s]-> %s", edge.source, edge.kind, edge.target)
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
}
