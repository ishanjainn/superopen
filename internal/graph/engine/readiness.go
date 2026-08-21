package engine

import "github.com/ishanjainn/superopen/internal/graph/api"

const (
	gateVerified   = "verified"
	gateInProgress = "in_progress"
	gatePending    = "pending"
)

// NodeLabels and EdgeTypes are the observable schema inventories at the pinned
// commit. EdgeTypes includes 27 intra-repository producers and six cross-repo
// producers; keeping them explicit prevents a generic typed-edge gate from
// hiding a missing relationship family.
var NodeLabels = []string{
	"Annotation", "Branch", "Channel", "Chart", "Class", "Constant", "Constructor", "Decorator",
	"Destructor", "Enum", "EnumMember", "EnvVar", "Field", "File", "Folder", "Function",
	"Impl", "Interface", "Macro", "Method", "Mixin", "Module", "Namespace", "Object",
	"Package", "Project", "Property", "Protocol", "Record", "Resource", "Route", "Section",
	"Storage", "Struct", "Trait", "Trigger", "Type", "TypeAlias", "Union", "Variable",
}

var EdgeTypes = []string{
	"ASYNC_CALLS", "CALL_REFERENCE", "CALLS", "CONFIGURES", "CONTAINS_FILE", "CONTAINS_FOLDER",
	"CROSS_ASYNC_CALLS", "CROSS_CHANNEL", "CROSS_GRAPHQL_CALLS", "CROSS_GRPC_CALLS", "CROSS_HTTP_CALLS", "CROSS_TRPC_CALLS",
	"DATA_FLOWS", "DECORATES", "DEFINES", "DEFINES_METHOD", "DEPENDS_ON", "FILE_CHANGES_WITH",
	"GRAPHQL_CALLS", "GRPC_CALLS", "HANDLES", "HAS_BRANCH", "HTTP_CALLS", "IMPLEMENTS", "IMPORTS", "INFRA_MAPS",
	"INHERITS", "OVERRIDE", "RAISES", "READS", "SEMANTICALLY_RELATED", "SIMILAR_TO", "TESTS", "TESTS_FILE",
	"THROWS", "TRPC_CALLS", "USAGE", "WRITES",
}

func readinessGates() []api.ReadinessGate {
	return readinessGatesWithManifest()
}

// readinessGateDefinitions is the static acceptance ledger. Gate state is merged
// from readiness_manifest.json at runtime; a gate may become verified only when
// the manifest records an executed golden proof for that gate id.
func readinessGateDefinitions() []api.ReadinessGate {
	gates := []api.ReadinessGate{
		gate("protocol.provider-neutral-v1", "protocol", gateInProgress, "versioned request/response envelopes and typed graph results; golden operation proof pending"),
		gate("store.sqlite-fts5", "storage", gateInProgress, "pure-Go SQLite store with FTS5/BM25 and transactional writes; pinned ranking differential pending"),
		gate("store.atomic-publication", "storage", gateInProgress, "failed builds preserve the last valid generation on every supported OS"),
		gate("index.discovery-ignore", "indexing", gateInProgress, "repository discovery, gitignore, explicit excludes, symlinks, submodules, generated and vendored files"),
		gate("index.incremental-watch", "indexing", gateInProgress, "content-grounded add/change/delete plus conservative one-to-one rename and branch-revision planning implemented; selective publication, watcher coalescing and golden differential pending"),
		gate("index.recovery-upgrade", "indexing", gateInProgress, "verification, corruption recovery, development schema upgrades and clean rebuild"),
		gate("index.team-artifact", "indexing", gateInProgress, "pure-Go Zstandard content-addressed export/import with integrity verification, root rebinding and atomic rollback; cross-platform differential pending"),
		gate("extract.nodes-edges", "extraction", gateInProgress, "deterministic multi-language repository/file/folder, definition, local import, resolved-call and unresolved-relationship publication implemented; remaining typed categories and family enrichments pending"),
		gate("extract.partial-ambiguous", "extraction", gateInProgress, "partial parse coverage, duplicate-definition identity, non-node unresolved relationship records, sorted alternatives, ambiguity flags, external fallbacks and unresolved evidence implemented; family golden differential pending"),
		gate("extract.framework-config", "extraction", gateInProgress, "routes, protocols, channels, configuration and IaC relationships"),
		gate("extract.data-flow", "extraction", gateInProgress, "reads/writes extraction and publication; arguments, return values, other families and interprocedural data flow pending"),
		gate("resolver.python", "resolution", gatePending, "Python semantic resolver"),
		gate("resolver.javascript-typescript", "resolution", gatePending, "JavaScript, TypeScript, JSX and TSX semantic resolver"),
		gate("resolver.php", "resolution", gatePending, "PHP semantic resolver"),
		gate("resolver.csharp", "resolution", gatePending, "C# semantic resolver"),
		gate("resolver.go", "resolution", gateInProgress, "Go AST/types resolver enriches FileResult.ResolvedCalls; golden precision/recall pending"),
		gate("resolver.c-cpp", "resolution", gateInProgress, "C/C++/CUDA preprocessor second pass plus in-process type-registry ResolvedCalls; clang-level templates/ADL pending"),
		gate("resolver.java", "resolution", gatePending, "Java semantic resolver"),
		gate("resolver.kotlin", "resolution", gatePending, "Kotlin semantic resolver"),
		gate("resolver.rust", "resolution", gatePending, "Rust semantic resolver"),
		gate("resolver.perl", "resolution", gatePending, "Perl semantic resolver"),
		gate("trace.evidence", "resolution", gateInProgress, "strategy, confidence, location, ambiguity, fallback and unresolved evidence on relationships"),
		gate("search.lexical", "search", gateInProgress, "BM25, camel/token splitting, exact/fuzzy lookup, nullable zero-degree filters, entry-point exclusion, validated relationship filtering, one-hop connected enrichment and structural boosts; golden differential pending"),
		gate("search.semantic", "search", gateInProgress, "pinned tokenizer, verified 40,856x768 int8 pretrained-vector loader with sparse fallback, corpus IDF/two-pass enrichment, pinned raw-int8 node/token publication, semantic-only results, combined lexical/semantic sections and multi-keyword min-cosine ranking implemented; golden ranking differential still failing"),
		gate("search.fusion-diffusion", "search", gateInProgress, "pinned weighted AST MinHash/LSH and normalized graph-vector diffusion implemented; multi-signal combined scoring and edge publication pending"),
		gate("search.render-budget", "search", gateInProgress, "stable pagination, compact trees and bounded token output"),
		gate("query.cypher-lexer-parser", "query", gateInProgress, "read-only Cypher lexer/parser and bounded diagnostics implemented; full Superopen differential pending"),
		gate("query.cypher-patterns", "query", gateInProgress, "MATCH, OPTIONAL MATCH, multi-pattern, directions, alternatives and variable paths parsed; execution differential pending"),
		gate("query.cypher-expressions", "query", gateInProgress, "WHERE boolean/value expressions, functions, lists, nulls and properties parsed; execution differential pending"),
		gate("query.cypher-projection", "query", gateInProgress, "WITH, RETURN, DISTINCT, aggregation, ordering, skip and limit parsed; grouping execution pending"),
		gate("query.cypher-union-unwind", "query", gateInProgress, "UNION/UNION ALL and UNWIND parsed; execution differential pending"),
		gate("query.cypher-security", "query", gateInProgress, "read-only keyword rejection, full-consumption parsing, complexity bounds, deadlines, hard row ceilings and pinned explicit-LIMIT precedence implemented; full golden differential pending"),
		gate("analysis.paths-dependencies", "analysis", gateInProgress, "paths, callers/callees, dependencies and reverse dependencies"),
		gate("analysis.quality", "analysis", gateInProgress, "pinned dead-code degree and entry-point semantics plus CALLS fan-in hotspots implemented; god nodes and coverage gaps pending"),
		gate("analysis.architecture", "analysis", gateInProgress, "pinned aspect dispatch, path scoping/totals, compact default, languages, package fallback, entry points, routes, CALLS hotspots, boundaries, layers, shallow file tree and clusters implemented; golden differential pending"),
		gate("analysis.cycles", "analysis", gateInProgress, "deterministic strongly connected call-graph components, including self recursion; golden differential pending"),
		gate("analysis.communities", "analysis", gateInProgress, "pinned weighted deterministic Leiden local move, connected refinement, multilevel aggregation, resolution control, cohesion, package labels and representative hubs implemented; large-corpus golden differential pending"),
		gate("analysis.change-impact", "analysis", gateInProgress, "git diff seeds, blast radius, risk and module rollups"),
		gate("analysis.missed-graph", "analysis", gateInProgress, "transactionally derived project/folder/file graph for failed coverage rows, excluding intentional not_indexed entries, selectable through provider-neutral Cypher; golden differential pending"),
		gate("projects.cross-repository", "projects", gatePending, "multi-project global graph and cross-service relationship matching"),
		gate("runtime.traces", "projects", gateInProgress, "runtime trace ingestion and graph enrichment"),
		gate("assets.visualization", "packaging", gatePending, "verified content-addressed visualization side asset"),
		gate("packaging.cross-platform", "packaging", gateInProgress, "CGO-free builds and clean installation on supported macOS, Linux and Windows architectures"),
		gate("quality.differential", "acceptance", gateInProgress, "normalized differential suite against the Superopen asset oracle"),
		gate("quality.precision-recall", "acceptance", gatePending, "source-grounded extraction and resolution precision/recall gates"),
		gate("quality.performance", "acceptance", gatePending, "five-run cold/warm benchmarks with no unexplained regression above five percent"),
		gate("quality.determinism-race", "acceptance", gateInProgress, "deterministic cross-platform output, race tests, vet and protocol compatibility"),
	}
	for _, language := range Languages {
		gates = append(gates, api.ReadinessGate{
			ID:          "grammar." + language,
			Area:        "grammar",
			State:       gateInProgress,
			Requirement: "content-verified embedded Tree-sitter WASM parser present and executable; language golden extraction and golden differential pending",
			Languages:   []string{language},
		})
	}
	implementedNodes := map[string]bool{
		"Branch": true, "Channel": true, "Class": true, "Constant": true, "Decorator": true, "EnvVar": true,
		"Field": true, "File": true, "Folder": true, "Function": true, "Interface": true, "Method": true,
		"Module": true, "Package": true, "Project": true, "Resource": true, "Route": true, "Section": true,
		"Struct": true, "Trait": true, "Type": true, "Variable": true,
	}
	for _, label := range NodeLabels {
		state := gatePending
		if implementedNodes[label] {
			state = gateInProgress
		}
		gates = append(gates, gate("node."+label, "node_schema", state, "source-grounded "+label+" extraction with required properties"))
	}
	implementedEdges := map[string]bool{
		"CALLS": true, "CALL_REFERENCE": true, "CONFIGURES": true, "CONTAINS_FILE": true, "CONTAINS_FOLDER": true,
		"DECORATES": true, "DEFINES": true, "DEFINES_METHOD": true, "DEPENDS_ON": true, "FILE_CHANGES_WITH": true,
		"HANDLES": true, "HAS_BRANCH": true, "HTTP_CALLS": true, "IMPLEMENTS": true, "IMPORTS": true,
		"INHERITS": true, "OVERRIDE": true, "RAISES": true, "READS": true, "SIMILAR_TO": true,
		"TESTS": true, "TESTS_FILE": true, "THROWS": true, "USAGE": true, "WRITES": true,
	}
	for _, edgeType := range EdgeTypes {
		state := gatePending
		if implementedEdges[edgeType] {
			state = gateInProgress
		}
		gates = append(gates, gate("edge."+edgeType, "edge_schema", state, "source-grounded "+edgeType+" production, resolution and trace evidence"))
	}
	return gates
}

func gate(id, area, state, requirement string) api.ReadinessGate {
	return api.ReadinessGate{ID: id, Area: area, State: state, Requirement: requirement}
}

func readinessSummary(gates []api.ReadinessGate) api.ReadinessSummary {
	result := api.ReadinessSummary{Total: len(gates)}
	for _, item := range gates {
		switch item.State {
		case gateVerified:
			result.Verified++
		case gateInProgress:
			result.InProgress++
		default:
			result.Pending++
		}
	}
	return result
}
