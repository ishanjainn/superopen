package graph

// FlagSchema is the reviewed Graphify 0.9.45 façade contract. It is kept in
// compiled code so dependency upgrades cannot silently add executable surface.
type FlagSchema struct {
	Name          string
	Type          string // bool|string|int|stringSlice
	Default       any
	Usage         string
	RequiredExtra string
	ConflictsWith string
}

type CommandSchemaEntry struct {
	Native         string
	Alias          string
	Flags          []FlagSchema
	RequiredExtra  string
	ExcludedReason string
}

var CommandSchema = map[string]CommandSchemaEntry{
	"extract": {Native: "extract", Flags: []FlagSchema{
		{"backend", "string", "", "Configured semantic backend", "", ""}, {"model", "string", "", "Backend model", "", ""}, {"mode", "string", "standard", "standard or deep extraction", "", ""},
		{"force", "bool", false, "Bypass manifest and semantic cache", "", ""}, {"max-workers", "int", 0, "AST worker count", "", ""}, {"token-budget", "int", 60000, "Per-chunk token cap", "", ""},
		{"max-concurrency", "int", 4, "Parallel semantic chunks", "", ""}, {"api-timeout", "string", "600", "Provider timeout seconds", "", ""}, {"directed", "bool", false, "Preserve directed edges", "", ""},
		{"no-cluster", "bool", false, "Skip clustering", "", ""}, {"exclude", "stringSlice", nil, "Additional source exclusions", "", ""}, {"resolution", "string", "", "Clustering resolution", "leiden", ""},
		{"exclude-hubs", "string", "", "Hub exclusion percentile", "", ""}, {"google-workspace", "bool", false, "Resolve Workspace shortcuts", "google", ""}, {"no-gitignore", "bool", false, "Ignore gitignore rules", "", ""},
		{"code-only", "bool", false, "Local AST only", "", ""}, {"postgres", "string", "", "PostgreSQL schema DSN", "postgres", ""}, {"cargo", "bool", false, "Cargo workspace metadata", "", ""},
	}},
	"update":            {Native: "update", Flags: []FlagSchema{{"force", "bool", false, "Allow intentional graph shrink", "", ""}, {"no-cluster", "bool", false, "Skip clustering", "", ""}}},
	"query":             {Native: "query", Flags: []FlagSchema{{"dfs", "bool", false, "Depth-first traversal", "", ""}, {"depth", "int", 2, "Traversal depth (1-6)", "", ""}, {"context", "stringSlice", nil, "Edge context filter", "", ""}, {"budget", "int", 1200, "Output token budget", "", ""}, {"term", "stringSlice", nil, "Optional vocabulary expansion term", "", ""}, {"original-question", "string", "", "Superopen AXI question metadata", "", ""}}},
	"stats":             {Alias: "superopen-state"},
	"serve":             {Alias: "graphify-mcp", RequiredExtra: "mcp"},
	"watch":             {Native: "watch", RequiredExtra: "watch"},
	"check-update":      {Native: "check-update"},
	"path":              {Native: "path"},
	"explain":           {Native: "explain"},
	"affected":          {Native: "affected", Flags: []FlagSchema{{"relation", "stringSlice", nil, "Relation to traverse in reverse (repeatable)", "", ""}, {"depth", "int", 2, "Reverse traversal depth", "", ""}}},
	"god-nodes":         {Native: "god-nodes", Flags: []FlagSchema{{"top", "int", 10, "Number of hubs to show", "", ""}, {"json", "bool", false, "Emit Graphify JSON", "", ""}}},
	"diagnose":          {Native: "diagnose", Flags: []FlagSchema{{"json", "bool", false, "Emit Graphify JSON", "", ""}, {"max-examples", "int", 5, "Maximum duplicate-edge examples", "", ""}, {"directed", "bool", false, "Force directed simulation", "", "undirected"}, {"undirected", "bool", false, "Force undirected simulation", "", "directed"}, {"extract-path", "string", "", "Extractor source for suppression scan", "", ""}}},
	"cluster":           {Native: "cluster-only", Flags: []FlagSchema{{"no-viz", "bool", false, "Skip graph.html", "", ""}, {"no-label", "bool", false, "Keep placeholder community labels", "", ""}, {"backend", "string", "", "Configured labeling backend", "", ""}, {"model", "string", "", "Labeling model", "", ""}, {"max-concurrency", "int", 4, "Parallel labeling calls", "", ""}, {"batch-size", "int", 100, "Communities per labeling call", "", ""}}},
	"label":             {Native: "label", Flags: []FlagSchema{{"missing-only", "bool", false, "Only label missing communities", "", ""}, {"backend", "string", "", "Configured labeling backend", "", ""}, {"model", "string", "", "Labeling model", "", ""}, {"max-concurrency", "int", 4, "Parallel labeling calls", "", ""}, {"batch-size", "int", 100, "Communities per labeling call", "", ""}}},
	"add":               {Native: "add", Flags: []FlagSchema{{"author", "string", "", "Content author", "", ""}, {"contributor", "string", "", "Corpus contributor", "", ""}, {"dir", "string", "", "Repository-local target directory", "", ""}}},
	"clone":             {Native: "clone", Flags: []FlagSchema{{"branch", "string", "", "Branch to clone", "", ""}, {"out", "string", "", "Clone destination", "", ""}}},
	"merge":             {Native: "merge-graphs", Flags: []FlagSchema{{"out", "string", "", "Merged graph output", "", ""}}},
	"global":            {Native: "global"},
	"prs":               {Native: "prs", Flags: []FlagSchema{{"triage", "bool", false, "Triage pull requests", "", "conflicts"}, {"conflicts", "bool", false, "Analyze conflicts", "", "triage"}, {"worktrees", "bool", false, "Show worktree to PR mapping", "", ""}, {"wrong-base", "bool", false, "Include pull requests targeting another base", "", ""}, {"base", "string", "", "Target base branch", "", ""}, {"repo", "string", "", "GitHub owner/repository", "", ""}}},
	"benchmark":         {Native: "benchmark"},
	"tree":              {Native: "tree", Flags: []FlagSchema{{"output", "string", "", "Tree HTML output path", "", ""}, {"root", "string", "", "Filesystem hierarchy root", "", ""}, {"max-children", "int", 200, "Tree children per node", "", ""}, {"top-k-edges", "int", 12, "Inspector outbound edges", "", ""}, {"label", "string", "", "Project label", "", ""}}},
	"export":            {Native: "export", Flags: []FlagSchema{{"output", "string", "", "Artifact output path", "", "dir"}, {"dir", "string", "", "Artifact output directory", "", "output"}, {"root", "string", "", "Filesystem hierarchy root", "", ""}, {"max-children", "int", 200, "Tree children per node", "", ""}, {"top-k-edges", "int", 12, "Tree inspector outbound edges", "", ""}, {"label", "string", "", "Project label", "", ""}, {"push", "string", "", "Explicit database destination URI", "", ""}}},
	"export:html":       {Alias: "export html", Flags: []FlagSchema{{"node-limit", "int", 5000, "Maximum nodes in the full HTML view", "", ""}, {"no-viz", "bool", false, "Remove/skip graph.html", "", ""}}},
	"export:wiki":       {Alias: "export wiki"},
	"export:obsidian":   {Alias: "export obsidian", Flags: []FlagSchema{{"dir", "string", "", "Obsidian vault destination", "", ""}}},
	"export:svg":        {Alias: "export svg", RequiredExtra: "svg"},
	"export:graphml":    {Alias: "export graphml"},
	"export:canvas":     {Alias: "graphify.export.to_canvas"},
	"export:cypher":     {Alias: "export neo4j (without --push)"},
	"export:callflow":   {Alias: "export callflow-html", Flags: []FlagSchema{{"sections", "string", "", "JSON section definitions", "", ""}, {"output", "string", "", "Callflow HTML destination", "", ""}, {"lang", "string", "auto", "Diagram language", "", ""}, {"max-sections", "int", 15, "Maximum auto-derived sections", "", ""}, {"diagram-scale", "string", "1.0", "Mermaid scale", "", ""}, {"max-diagram-nodes", "int", 18, "Representative nodes per section", "", ""}, {"max-diagram-edges", "int", 24, "Representative edges per section", "", ""}}},
	"export:neo4j":      {Alias: "export neo4j", RequiredExtra: "neo4j", Flags: []FlagSchema{{"push", "string", "", "Explicit Neo4j URI", "", ""}, {"user", "string", "neo4j", "Database user", "", ""}}},
	"export:falkordb":   {Alias: "export falkordb", RequiredExtra: "falkordb", Flags: []FlagSchema{{"push", "string", "", "Explicit FalkorDB URI", "", ""}, {"user", "string", "", "Database user", "", ""}}},
	"provider":          {Native: "provider", Alias: "provider add|list|show|remove"},
	"vendor-installers": {ExcludedReason: "Superopen owns coding-agent skills and hooks"},
	"git-hooks":         {ExcludedReason: "Superopen owns lifecycle hooks"},
	"merge-driver":      {ExcludedReason: "Superopen owns repository integration and does not install Graphify git drivers"},
	"save-result":       {Alias: "result save", ExcludedReason: "Superopen memory is the authoritative outcome store"},
	"reflect":           {Alias: "reflect", ExcludedReason: "Superopen materializes a temporary Graphify-compatible memory projection"},
}

func CapabilityState() map[string]map[string]string {
	out := make(map[string]map[string]string, len(CommandSchema))
	for name, command := range CommandSchema {
		status := "available"
		if command.ExcludedReason != "" {
			status = "intentionally_excluded"
			if command.Alias != "" {
				status = "superopen_owned"
			}
		}
		out[name] = map[string]string{"status": status}
		if command.RequiredExtra != "" {
			out[name]["required_extra"] = command.RequiredExtra
		}
		if command.ExcludedReason != "" {
			out[name]["ownership_reason"] = command.ExcludedReason
		}
	}
	return out
}
