// Package api defines the native graph protocol used by `so`.
package api

import (
	"encoding/json"
	"time"
)

const (
	ProtocolVersion = 1
	EngineName      = "so-graph"
	SchemaVersion   = 3
)

type Operation string

const (
	OpCapabilities   Operation = "capabilities"
	OpBuild          Operation = "build"
	OpStatus         Operation = "status"
	OpSchema         Operation = "schema"
	OpSearch         Operation = "search"
	OpCodeSearch     Operation = "code_search"
	OpQuery          Operation = "query"
	OpCypher         Operation = "cypher"
	OpTrace          Operation = "trace"
	OpSnippet        Operation = "snippet"
	OpArchitecture   Operation = "architecture"
	OpLayout         Operation = "layout"
	OpImpact         Operation = "impact"
	OpCoverage       Operation = "coverage"
	OpProjects       Operation = "projects"
	OpProjectDelete  Operation = "projects_delete"
	OpTraceIngest    Operation = "traces_ingest"
	OpArtifactExport Operation = "artifact_export"
	OpArtifactImport Operation = "artifact_import"
	OpArtifactVerify Operation = "artifact_verify"
	OpDiagnostics    Operation = "diagnostics"
	OpIncremental    Operation = "incremental"
)

// Request is the single JSON request envelope understood by so-graph. Params
// is decoded according to Operation, which lets the protocol add operations
// without coupling the main so binary to engine internals.
type Request struct {
	Protocol  int             `json:"protocol"`
	RequestID string          `json:"request_id,omitempty"`
	Operation Operation       `json:"operation"`
	Params    json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	Protocol  int             `json:"protocol"`
	RequestID string          `json:"request_id,omitempty"`
	OK        bool            `json:"ok"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *Error          `json:"error,omitempty"`
}

type Error struct {
	Code       string         `json:"code"`
	Message    string         `json:"message"`
	Retryable  bool           `json:"retryable,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
	Suggestion string         `json:"suggestion,omitempty"`
}

type Page struct {
	Limit      int    `json:"limit"`
	Cursor     string `json:"cursor,omitempty"`
	NextCursor string `json:"next_cursor,omitempty"`
	Total      int    `json:"total,omitempty"`
	Truncated  bool   `json:"truncated"`
}

type Budget struct {
	RequestedTokens int  `json:"requested_tokens,omitempty"`
	ReturnedTokens  int  `json:"returned_tokens,omitempty"`
	Truncated       bool `json:"truncated"`
}

type Location struct {
	File        string `json:"file"`
	StartLine   int    `json:"start_line,omitempty"`
	StartColumn int    `json:"start_column,omitempty"`
	EndLine     int    `json:"end_line,omitempty"`
	EndColumn   int    `json:"end_column,omitempty"`
}

type Evidence struct {
	Strategy     string     `json:"strategy"`
	Confidence   float64    `json:"confidence"`
	Location     *Location  `json:"location,omitempty"`
	Excerpt      string     `json:"excerpt,omitempty"`
	Ambiguous    bool       `json:"ambiguous,omitempty"`
	Alternatives []string   `json:"alternatives,omitempty"`
	Unresolved   string     `json:"unresolved_reason,omitempty"`
	Properties   Properties `json:"properties,omitempty"`
}

type Properties map[string]any

type Node struct {
	ID            int64      `json:"id"`
	Project       string     `json:"project"`
	Label         string     `json:"label"`
	Name          string     `json:"name"`
	QualifiedName string     `json:"qualified_name"`
	Location      Location   `json:"location"`
	Properties    Properties `json:"properties,omitempty"`
}

type Edge struct {
	ID         int64      `json:"id"`
	Project    string     `json:"project"`
	SourceID   int64      `json:"source_id"`
	TargetID   int64      `json:"target_id"`
	Type       string     `json:"type"`
	Properties Properties `json:"properties,omitempty"`
	Evidence   *Evidence  `json:"evidence,omitempty"`
}

type RankedNode struct {
	Node
	Score     float64    `json:"score"`
	Signals   Properties `json:"signals,omitempty"`
	Connected []string   `json:"connected,omitempty"`
}

type BuildRequest struct {
	RepoRoot       string   `json:"repo_root"`
	Project        string   `json:"project,omitempty"`
	Mode           string   `json:"mode,omitempty"`
	Force          bool     `json:"force,omitempty"`
	Incremental    bool     `json:"incremental,omitempty"`
	Exclude        []string `json:"exclude,omitempty"`
	TokenBudget    int      `json:"token_budget,omitempty"`
	ExpectedSource string   `json:"expected_source_revision,omitempty"`
}

type BuildResult struct {
	Status         string        `json:"status"`
	Project        string        `json:"project"`
	Database       string        `json:"database"`
	SourceRevision string        `json:"source_revision,omitempty"`
	Generation     string        `json:"generation"`
	NodeCount      int           `json:"node_count"`
	EdgeCount      int           `json:"edge_count"`
	FileCount      int           `json:"file_count"`
	Duration       time.Duration `json:"duration_ns"`
	Coverage       Coverage      `json:"coverage"`
	Changes        *ChangeSet    `json:"changes,omitempty"`
}

type ChangeSet struct {
	Added           []FileChange `json:"added,omitempty"`
	Modified        []FileChange `json:"modified,omitempty"`
	Deleted         []FileChange `json:"deleted,omitempty"`
	Renamed         []FileChange `json:"renamed,omitempty"`
	Unchanged       int          `json:"unchanged"`
	SourceRevision  string       `json:"source_revision,omitempty"`
	RevisionChanged bool         `json:"revision_changed,omitempty"`
	RequiresFull    bool         `json:"requires_full,omitempty"`
	Reason          string       `json:"reason,omitempty"`
}

type FileChange struct {
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	OldPath string `json:"old_path,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
}

type StatusRequest struct {
	RepoRoot string `json:"repo_root,omitempty"`
	Project  string `json:"project,omitempty"`
}

type Status struct {
	Engine        string         `json:"engine"`
	EngineVersion string         `json:"engine_version"`
	Protocol      int            `json:"protocol"`
	Schema        int            `json:"schema"`
	State         string         `json:"state"`
	Project       string         `json:"project,omitempty"`
	Database      string         `json:"database,omitempty"`
	Generation    string         `json:"generation,omitempty"`
	IndexedAt     *time.Time     `json:"indexed_at,omitempty"`
	NodeCount     int            `json:"node_count,omitempty"`
	EdgeCount     int            `json:"edge_count,omitempty"`
	FileCount     int            `json:"file_count,omitempty"`
	Diagnostics   []Diagnostic   `json:"diagnostics,omitempty"`
	Capabilities  CapabilitySet  `json:"capabilities"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

type SchemaRequest struct {
	RepoRoot string `json:"repo_root,omitempty"`
	Project  string `json:"project,omitempty"`
}

type SchemaCount struct {
	Name       string   `json:"name"`
	Count      int      `json:"count"`
	Properties []string `json:"properties,omitempty"`
}

type SchemaPattern struct {
	Source string `json:"source"`
	Edge   string `json:"edge"`
	Target string `json:"target"`
	Count  int    `json:"count"`
}

type SchemaResult struct {
	Project    string          `json:"project"`
	NodeLabels []SchemaCount   `json:"node_labels"`
	EdgeTypes  []SchemaCount   `json:"edge_types"`
	Patterns   []SchemaPattern `json:"patterns"`
	NodeCount  int             `json:"node_count"`
	EdgeCount  int             `json:"edge_count"`
}

type CodeSearchRequest struct {
	RepoRoot   string `json:"repo_root,omitempty"`
	Project    string `json:"project,omitempty"`
	Pattern    string `json:"pattern"`
	FileGlob   string `json:"file_pattern,omitempty"`
	PathFilter string `json:"path_filter,omitempty"`
	Mode       string `json:"mode,omitempty"`
	Context    int    `json:"context,omitempty"`
	Regex      bool   `json:"regex,omitempty"`
	Debug      bool   `json:"debug,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

type CodeSearchMatch struct {
	Node       *Node    `json:"node,omitempty"`
	Location   Location `json:"location"`
	Signature  string   `json:"signature,omitempty"`
	Source     string   `json:"source,omitempty"`
	MatchLines []int    `json:"match_lines,omitempty"`
	Score      float64  `json:"score,omitempty"`
	Truncated  bool     `json:"source_truncated,omitempty"`
}

type CodeSearchResult struct {
	Matches          []CodeSearchMatch `json:"matches"`
	Files            []string          `json:"files,omitempty"`
	TotalGrepMatches int               `json:"total_grep_matches"`
	TotalResults     int               `json:"total_results"`
	Page             Page              `json:"page"`
}

type SearchRequest struct {
	RepoRoot             string   `json:"repo_root,omitempty"`
	Project              string   `json:"project,omitempty"`
	Query                string   `json:"query,omitempty"`
	NamePattern          string   `json:"name_pattern,omitempty"`
	QualifiedNamePattern string   `json:"qualified_name_pattern,omitempty"`
	FilePattern          string   `json:"file_pattern,omitempty"`
	Labels               []string `json:"labels,omitempty"`
	Languages            []string `json:"languages,omitempty"`
	PathPrefix           string   `json:"path_prefix,omitempty"`
	Relationship         string   `json:"relationship,omitempty"`
	MinDegree            *int     `json:"min_degree,omitempty"`
	MaxDegree            *int     `json:"max_degree,omitempty"`
	ExcludeEntryPoints   bool     `json:"exclude_entry_points,omitempty"`
	IncludeConnected     bool     `json:"include_connected,omitempty"`
	SemanticQuery        []string `json:"semantic_query,omitempty"`
	Fields               []string `json:"fields,omitempty"`
	Detail               string   `json:"detail,omitempty"`
	Mode                 string   `json:"mode,omitempty"`
	Limit                int      `json:"limit,omitempty"`
	Cursor               string   `json:"cursor,omitempty"`
	Budget               int      `json:"budget,omitempty"`
}

type SearchResult struct {
	Matches  []RankedNode `json:"matches"`
	Semantic []RankedNode `json:"semantic,omitempty"`
	Page     Page         `json:"page"`
	Budget   Budget       `json:"budget"`
}

type QueryRequest struct {
	RepoRoot string   `json:"repo_root,omitempty"`
	Project  string   `json:"project,omitempty"`
	Question string   `json:"question"`
	Terms    []string `json:"terms,omitempty"`
	Depth    int      `json:"depth,omitempty"`
	Budget   int      `json:"budget,omitempty"`
	Cursor   string   `json:"cursor,omitempty"`
}

type QueryResult struct {
	Text      string       `json:"text"`
	Seeds     []RankedNode `json:"seeds,omitempty"`
	Nodes     []Node       `json:"nodes,omitempty"`
	Edges     []Edge       `json:"edges,omitempty"`
	Page      Page         `json:"page"`
	Budget    Budget       `json:"budget"`
	Uncertain []Evidence   `json:"uncertain,omitempty"`
}

type CypherRequest struct {
	RepoRoot string         `json:"repo_root,omitempty"`
	Project  string         `json:"project,omitempty"`
	Query    string         `json:"query"`
	Params   map[string]any `json:"params,omitempty"`
	MaxRows  int            `json:"max_rows,omitempty"`
	Graph    string         `json:"graph,omitempty"`
}

type CypherResult struct {
	Columns   []string         `json:"columns"`
	Rows      []map[string]any `json:"rows"`
	Page      Page             `json:"page"`
	Plan      []string         `json:"plan,omitempty"`
	ElapsedNS int64            `json:"elapsed_ns"`
}

type TraceRequest struct {
	RepoRoot  string   `json:"repo_root,omitempty"`
	Project   string   `json:"project,omitempty"`
	Start     string   `json:"start"`
	Target    string   `json:"target,omitempty"`
	Direction string   `json:"direction,omitempty"`
	EdgeTypes []string `json:"edge_types,omitempty"`
	Depth     int      `json:"depth,omitempty"`
	Limit     int      `json:"limit,omitempty"`
	Parameter string   `json:"parameter,omitempty"`
}

type TraceStep struct {
	Node     Node      `json:"node"`
	Via      *Edge     `json:"via,omitempty"`
	Evidence *Evidence `json:"evidence,omitempty"`
	Hop      int       `json:"hop"`
}

type TraceResult struct {
	Paths       [][]TraceStep            `json:"paths"`
	Unresolved  []UnresolvedRelationship `json:"unresolved,omitempty"`
	Visited     int                      `json:"visited"`
	Truncated   bool                     `json:"truncated"`
	Coverage    Coverage                 `json:"coverage,omitempty"`
	Status      string                   `json:"status,omitempty"`
	Message     string                   `json:"message,omitempty"`
	Suggestions []Node                   `json:"suggestions,omitempty"`
}

// UnresolvedRelationship preserves a source-grounded relationship attempt
// without inventing a target node that would pollute graph analysis/search.
type UnresolvedRelationship struct {
	Project    string     `json:"project"`
	Source     string     `json:"source"`
	TargetText string     `json:"target_text"`
	Type       string     `json:"type"`
	Properties Properties `json:"properties,omitempty"`
	Evidence   *Evidence  `json:"evidence,omitempty"`
}

type SnippetRequest struct {
	RepoRoot      string `json:"repo_root,omitempty"`
	Project       string `json:"project,omitempty"`
	QualifiedName string `json:"qualified_name,omitempty"`
	File          string `json:"file,omitempty"`
	StartLine     int    `json:"start_line,omitempty"`
	EndLine       int    `json:"end_line,omitempty"`
	ContextLines  int    `json:"context_lines,omitempty"`
}

type SnippetResult struct {
	Location      Location `json:"location"`
	Language      string   `json:"language,omitempty"`
	Code          string   `json:"code"`
	QualifiedName string   `json:"qualified_name,omitempty"`
	Name          string   `json:"name,omitempty"`
	Label         string   `json:"label,omitempty"`
	Callers       int      `json:"callers,omitempty"`
	Callees       int      `json:"callees,omitempty"`
	Coverage      Coverage `json:"coverage,omitempty"`
}

type ArchitectureRequest struct {
	RepoRoot string   `json:"repo_root,omitempty"`
	Project  string   `json:"project,omitempty"`
	Path     string   `json:"path,omitempty"`
	Aspects  []string `json:"aspects,omitempty"`
	Limit    int      `json:"limit,omitempty"`
}

type ArchitectureResult struct {
	Summary        string           `json:"summary"`
	Aspects        map[string]any   `json:"aspects"`
	Path           string           `json:"path,omitempty"`
	TotalNodes     int              `json:"total_nodes"`
	TotalEdges     int              `json:"total_edges"`
	RootTotalNodes int              `json:"root_total_nodes,omitempty"`
	RootTotalEdges int              `json:"root_total_edges,omitempty"`
	Languages      []LanguageCount  `json:"languages,omitempty"`
	Packages       []PackageSummary `json:"packages,omitempty"`
	EntryPoints    []Node           `json:"entry_points,omitempty"`
	Routes         []Route          `json:"routes,omitempty"`
	Hotspots       []RankedNode     `json:"hotspots,omitempty"`
	Boundaries     []Boundary       `json:"boundaries,omitempty"`
	Layers         []PackageLayer   `json:"layers,omitempty"`
	FileTree       []FileTreeEntry  `json:"file_tree,omitempty"`
	Cycles         [][]Node         `json:"cycles,omitempty"`
	Communities    []Community      `json:"communities,omitempty"`
	Coverage       Coverage         `json:"coverage"`
}

// LayoutRequest asks for a render-ready subgraph with server-computed 3D
// coordinates. MaxNodes is a rendering budget, not a graph correctness limit.
type LayoutRequest struct {
	RepoRoot string `json:"repo_root,omitempty"`
	Project  string `json:"project,omitempty"`
	MaxNodes int    `json:"max_nodes,omitempty"`
}

type LayoutNode struct {
	ID            int64   `json:"id"`
	X             float64 `json:"x"`
	Y             float64 `json:"y"`
	Z             float64 `json:"z"`
	Label         string  `json:"label"`
	Name          string  `json:"name"`
	QualifiedName string  `json:"qualified_name"`
	FilePath      string  `json:"file_path,omitempty"`
	StartLine     int     `json:"start_line,omitempty"`
	EndLine       int     `json:"end_line,omitempty"`
	Degree        int     `json:"degree"`
	Size          float64 `json:"size"`
	Color         string  `json:"color"`
	Community     string  `json:"community,omitempty"`
}

type LayoutEdge struct {
	Source int64  `json:"source"`
	Target int64  `json:"target"`
	Type   string `json:"type"`
}

type LayoutResult struct {
	Nodes      []LayoutNode `json:"nodes"`
	Edges      []LayoutEdge `json:"edges"`
	TotalNodes int          `json:"total_nodes"`
	TotalEdges int          `json:"total_edges"`
	Project    string       `json:"project"`
}

type PackageSummary struct {
	Name      string `json:"name"`
	NodeCount int    `json:"node_count"`
	FanIn     int    `json:"fan_in"`
	FanOut    int    `json:"fan_out"`
}

type LanguageCount struct {
	Language  string `json:"language"`
	FileCount int    `json:"file_count"`
}

type Route struct {
	Method  string `json:"method"`
	Path    string `json:"path"`
	Handler string `json:"handler"`
}

type Boundary struct {
	From      string `json:"from"`
	To        string `json:"to"`
	CallCount int    `json:"call_count"`
}

type PackageLayer struct {
	Name   string `json:"name"`
	Layer  string `json:"layer"`
	Reason string `json:"reason"`
}

type FileTreeEntry struct {
	Path     string `json:"path"`
	Type     string `json:"type"`
	Children int    `json:"children"`
}

type Community struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	Members   int      `json:"members"`
	Cohesion  float64  `json:"cohesion"`
	TopNodes  []string `json:"top_nodes,omitempty"`
	Packages  []string `json:"packages,omitempty"`
	EdgeTypes []string `json:"edge_types,omitempty"`
	Hub       *Node    `json:"hub,omitempty"`
}

type ImpactRequest struct {
	RepoRoot  string   `json:"repo_root,omitempty"`
	Project   string   `json:"project,omitempty"`
	Base      string   `json:"base,omitempty"`
	Symbols   []string `json:"symbols,omitempty"`
	Files     []string `json:"files,omitempty"`
	Direction string   `json:"direction,omitempty"`
	EdgeTypes []string `json:"edge_types,omitempty"`
	Depth     int      `json:"depth,omitempty"`
	Limit     int      `json:"limit,omitempty"`
}

type ImpactedNode struct {
	Node
	Hop int `json:"hop"`
}

type ImpactResult struct {
	Base            string         `json:"base,omitempty"`
	MergeBase       string         `json:"merge_base,omitempty"`
	ChangedFiles    []string       `json:"changed_files,omitempty"`
	Impacted        []ImpactedNode `json:"impacted"`
	ImpactedModules map[string]int `json:"impacted_modules"`
	Total           int            `json:"impacted_total"`
	Truncated       bool           `json:"truncated"`
	Coverage        Coverage       `json:"coverage"`
}

type CoverageRequest struct {
	RepoRoot string   `json:"repo_root,omitempty"`
	Project  string   `json:"project,omitempty"`
	Paths    []string `json:"paths,omitempty"`
	Scope    string   `json:"scope,omitempty"`
}

type Coverage struct {
	Status              string        `json:"status"`
	Generation          string        `json:"generation,omitempty"`
	IndexMode           string        `json:"index_mode,omitempty"`
	RecordedAt          *time.Time    `json:"recorded_at,omitempty"`
	RecordingStatus     string        `json:"recording_status,omitempty"`
	HashRecordsComplete bool          `json:"hash_records_complete,omitempty"`
	Rows                []CoverageRow `json:"rows,omitempty"`
	Total               int           `json:"total,omitempty"`
	Truncated           bool          `json:"truncated,omitempty"`
}

type CoverageRow struct {
	Path       string     `json:"path"`
	Kind       string     `json:"kind"`
	Detail     string     `json:"detail,omitempty"`
	Freshness  string     `json:"freshness,omitempty"`
	Location   *Location  `json:"location,omitempty"`
	Properties Properties `json:"properties,omitempty"`
}

type Project struct {
	Name       string    `json:"name"`
	RootPath   string    `json:"root_path"`
	Database   string    `json:"database"`
	IndexedAt  time.Time `json:"indexed_at"`
	Generation string    `json:"generation"`
	NodeCount  int       `json:"node_count"`
	EdgeCount  int       `json:"edge_count"`
	State      string    `json:"state"`
}

type ProjectsResult struct {
	Projects []Project `json:"projects"`
	Page     Page      `json:"page"`
}

type ProjectDeleteRequest struct {
	RepoRoot string `json:"repo_root"`
	Project  string `json:"project"`
}

type ProjectDeleteResult struct {
	Project string `json:"project"`
	Deleted bool   `json:"deleted"`
}

type RuntimeTrace struct {
	TraceID    string     `json:"trace_id,omitempty"`
	Source     string     `json:"source"`
	Target     string     `json:"target"`
	Type       string     `json:"type,omitempty"`
	Timestamp  *time.Time `json:"timestamp,omitempty"`
	DurationNS int64      `json:"duration_ns,omitempty"`
	Properties Properties `json:"properties,omitempty"`
}

type TraceIngestRequest struct {
	RepoRoot string         `json:"repo_root"`
	Project  string         `json:"project,omitempty"`
	Format   string         `json:"format,omitempty"`
	Traces   []RuntimeTrace `json:"traces"`
}

type TraceIngestResult struct {
	Accepted int          `json:"accepted"`
	Rejected int          `json:"rejected"`
	Edges    []Edge       `json:"edges,omitempty"`
	Errors   []Diagnostic `json:"errors,omitempty"`
}

type ArtifactRequest struct {
	RepoRoot string `json:"repo_root"`
	Path     string `json:"path"`
	Project  string `json:"project,omitempty"`
}

type ArtifactResult struct {
	Path           string `json:"path"`
	Project        string `json:"project"`
	Generation     string `json:"generation"`
	DatabaseSHA256 string `json:"database_sha256"`
	DatabaseSize   int64  `json:"database_size"`
	Verified       bool   `json:"verified"`
}

type IncrementalRequest struct {
	BuildRequest
	Changes ChangeSet `json:"changes"`
	BatchID string    `json:"batch_id,omitempty"`
}

type DiagnosticsRequest struct {
	RepoRoot string `json:"repo_root"`
	Project  string `json:"project,omitempty"`
}

type DiagnosticsResult struct {
	Healthy     bool         `json:"healthy"`
	Diagnostics []Diagnostic `json:"diagnostics"`
	Recovered   bool         `json:"recovered,omitempty"`
	Database    string       `json:"database,omitempty"`
}

type Diagnostic struct {
	Severity   string     `json:"severity"`
	Code       string     `json:"code"`
	Message    string     `json:"message"`
	Location   *Location  `json:"location,omitempty"`
	Properties Properties `json:"properties,omitempty"`
}

type Capability struct {
	Name      string   `json:"name"`
	State     string   `json:"state"`
	Languages []string `json:"languages,omitempty"`
	Notes     string   `json:"notes,omitempty"`
}

// ReadinessGate is an observable engine behavior that must be verified before
// the native engine can advertise completeness. A gate is deliberately more
// granular than a capability so an aggregate cannot hide an unsupported
// language, resolver, query feature, or operational failure mode.
type ReadinessGate struct {
	ID          string   `json:"id"`
	Area        string   `json:"area"`
	State       string   `json:"state"`
	Requirement string   `json:"requirement"`
	Languages   []string `json:"languages,omitempty"`
}

type ReadinessSummary struct {
	Total      int `json:"total"`
	Verified   int `json:"verified"`
	InProgress int `json:"in_progress"`
	Pending    int `json:"pending"`
}

type CapabilitySet struct {
	AssetRevision string           `json:"asset_revision"`
	Complete      bool             `json:"complete"`
	Capabilities  []Capability     `json:"capabilities"`
	Languages     []string         `json:"languages"`
	Readiness     ReadinessSummary `json:"readiness"`
	Gates         []ReadinessGate  `json:"gates,omitempty"`
}
