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
	project := request.Project
	if project == "" {
		project, err = ProjectName(root)
		if err != nil {
			return api.BuildResult{}, err
		}
	}
	if !request.Force {
		if unchanged, ok, err := tryUnchangedBuild(ctx, root, project, request.Exclude, started); err != nil {
			return api.BuildResult{}, err
		} else if ok {
			return unchanged, nil
		}
	}
	files, err := discoverTrackedFiles(ctx, root, request.Exclude)
	if err != nil {
		return api.BuildResult{}, err
	}
	runtime, _, err := LoadGrammarAssets(ctx, assets, "assets/grammars/manifest.json")
	if err != nil {
		return api.BuildResult{}, err
	}
	defer runtime.Close(ctx)
	repository, err := ParseSyntaxRepository(ctx, runtime, root, project, files, nil, 8)
	if err != nil {
		return api.BuildResult{}, err
	}
	fileOrder := make(map[string]int, len(files))
	for index, path := range files {
		fileOrder[path] = index
	}
	sort.SliceStable(repository.Files, func(i, j int) bool {
		return fileOrder[repository.Files[i].File.Path] < fileOrder[repository.Files[j].File.Path]
	})
	if err := enrichGoResolvedCalls(ctx, root, repository.Files); err != nil {
		return api.BuildResult{}, err
	}
	graph, coverage := AssembleSyntaxGraph(repository, project)
	registry := newSymbolRegistry(graph.nodes)
	applyPackageMap(project, repository, &graph)
	joinResolvedCalls(&graph, repository.Files, registry)
	indexSyntaxInheritance(project, repository.Files, &graph, registry)
	indexSyntaxGoImplements(&graph)
	indexSyntaxExplicitOverrides(&graph)
	// Builtins participate in the pinned global registry and therefore must be
	// present before family resolvers perform cross-language name fallback.
	indexPythonBuiltins(project, repository.Files, &graph)
	indexRepositoryBranch(ctx, root, project, &graph)
	indexEnvironmentAccesses(project, repository.Files, &graph)
	indexConfigLinks(&graph)
	indexHTTPRoutes(project, repository.Files, &graph)
	appendTestRelationships(&graph)
	indexGitCochange(ctx, root, project, &graph)
	annotateCrossRepositoryEdges(&graph)
	separateUnresolvedRelationships(&graph)
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
		return api.BuildResult{}, err
	}
	nodeCount, edgeCount, err := publishedCounts(ctx, database, project)
	if err != nil {
		return api.BuildResult{}, err
	}
	return api.BuildResult{Status: "development_incomplete", Project: project, Database: database,
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
		store, err := OpenWritable(path)
		if err != nil {
			return err
		}
		buildErr := store.Build(ctx, func(builder *Builder) error {
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
			for _, node := range graph.nodes {
				id, err := builder.PutNode(node)
				if err != nil {
					return err
				}
				ids[node.QualifiedName] = id
			}
			if err := putGraphSemantics(builder, graph, ids, project, model); err != nil {
				return err
			}
			if err := emitSemanticEdges(builder, graph, ids, project); err != nil {
				return err
			}
			for _, edge := range graph.edges {
				source, sourceOK := ids[edge.source]
				target, targetOK := ids[edge.target]
				if !sourceOK || !targetOK {
					return fmt.Errorf("graph edge endpoint missing: %s -[%s]-> %s", edge.source, edge.kind, edge.target)
				}
				if _, err := builder.PutEdge(api.Edge{Project: project, SourceID: source, TargetID: target,
					Type: edge.kind, Properties: edge.properties, Evidence: edge.evidence}); err != nil {
					return err
				}
			}
			for _, edge := range graph.unresolved {
				if err := builder.PutUnresolved(api.UnresolvedRelationship{Project: project, Source: edge.source,
					TargetText: edge.target, Type: edge.kind, Properties: edge.properties, Evidence: edge.evidence}); err != nil {
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
