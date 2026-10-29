package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ishanjainn/superopen/internal/cli"
	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/ishanjainn/superopen/internal/graph/client"
	"github.com/ishanjainn/superopen/internal/graph/format"
)

// graphNativeReadCommands exposes the native graph's read/query operations.
func graphNativeReadCommands() []*cobra.Command {
	status := nativeGraphLeaf("status", "Show native graph build status", api.OpStatus, func(*cobra.Command, []string) any {
		return api.StatusRequest{RepoRoot: repoRoot()}
	})
	schema := nativeGraphLeaf("schema", "Show graph labels, relationships, properties, and patterns", api.OpSchema, func(*cobra.Command, []string) any {
		return api.SchemaRequest{RepoRoot: repoRoot()}
	})
	search := nativeGraphLeaf("search <query>", "Search indexed graph symbols", api.OpSearch, func(cmd *cobra.Command, args []string) any {
		limit, _ := cmd.Flags().GetInt("limit")
		return api.SearchRequest{RepoRoot: repoRoot(), Query: strings.Join(args, " "), Limit: limit}
	})
	search.Args = cobra.MinimumNArgs(1)
	search.Flags().Int("limit", 10, "Maximum results")
	cypher := nativeGraphLeaf("cypher <query>", "Execute the supported read-only Cypher subset", api.OpCypher, func(cmd *cobra.Command, args []string) any {
		maxRows, _ := cmd.Flags().GetInt("max-rows")
		graph, _ := cmd.Flags().GetString("graph")
		return api.CypherRequest{RepoRoot: repoRoot(), Query: strings.Join(args, " "), MaxRows: maxRows, Graph: graph}
	})
	cypher.Args = cobra.MinimumNArgs(1)
	cypher.Flags().Int("max-rows", 0, "Maximum result rows")
	cypher.Flags().String("graph", "code", "Graph to query (code or missed)")
	trace := nativeGraphLeaf("trace <start> [target]", "Trace callers, callees, or a path", api.OpTrace, func(cmd *cobra.Command, args []string) any {
		direction, _ := cmd.Flags().GetString("direction")
		depth, _ := cmd.Flags().GetInt("depth")
		limit, _ := cmd.Flags().GetInt("limit")
		uncertain, _ := cmd.Flags().GetBool("uncertain")
		tests, _ := cmd.Flags().GetBool("tests")
		request := api.TraceRequest{RepoRoot: repoRoot(), Start: args[0], Direction: direction, Depth: depth, Limit: limit, Uncertain: uncertain, Tests: tests}
		if len(args) > 1 {
			request.Target = args[1]
		}
		return request
	})
	trace.Args = cobra.RangeArgs(1, 2)
	trace.Flags().String("direction", "outgoing", "Traversal direction")
	trace.Flags().Int("depth", 3, "Maximum traversal depth")
	trace.Flags().Int("limit", 100, "Maximum visited results")
	trace.Flags().Bool("uncertain", false, "Also walk edges the resolver guessed from a shared method name")
	trace.Flags().Bool("tests", false, "Also list and walk symbols in test files")
	snippet := nativeGraphLeaf("snippet <qualified-name>", "Read source for an indexed symbol", api.OpSnippet, func(cmd *cobra.Command, args []string) any {
		contextLines, _ := cmd.Flags().GetInt("context")
		fromLine, _ := cmd.Flags().GetInt("from")
		maxLines, _ := cmd.Flags().GetInt("lines")
		return api.SnippetRequest{RepoRoot: repoRoot(), QualifiedName: args[0], ContextLines: contextLines, StartLine: fromLine, MaxLines: maxLines}
	})
	snippet.Args = cobra.ExactArgs(1)
	snippet.Flags().Int("context", 0, "Neighboring source lines")
	snippet.Flags().Int("from", 0, "First source line to read; use the omitted line from a clipped snippet")
	snippet.Flags().Int("lines", 0, "Maximum source lines (use with a file path and --from after code-search)")
	architecture := nativeGraphLeaf("architecture", "Summarize repository architecture", api.OpArchitecture, func(cmd *cobra.Command, _ []string) any {
		path, _ := cmd.Flags().GetString("path")
		aspects, _ := cmd.Flags().GetStringSlice("aspect")
		return api.ArchitectureRequest{RepoRoot: repoRoot(), Path: path, Aspects: aspects}
	})
	architecture.Flags().String("path", "", "Directory scope")
	architecture.Flags().StringSlice("aspect", nil, "Architecture aspect")
	layout := nativeGraphLeaf("layout", "Emit a render-ready subgraph with server-computed coordinates", api.OpLayout, func(cmd *cobra.Command, _ []string) any {
		maxNodes, _ := cmd.Flags().GetInt("max-nodes")
		return api.LayoutRequest{RepoRoot: repoRoot(), MaxNodes: maxNodes}
	})
	layout.Flags().Int("max-nodes", 5000, "Node budget (highest degree first)")
	impact := nativeGraphLeaf("impact [symbol...]", "Analyze change or symbol impact", api.OpImpact, func(cmd *cobra.Command, args []string) any {
		base, _ := cmd.Flags().GetString("base")
		files, _ := cmd.Flags().GetStringSlice("files")
		depth, _ := cmd.Flags().GetInt("depth")
		return api.ImpactRequest{RepoRoot: repoRoot(), Base: base, Symbols: args, Files: files, Depth: depth}
	})
	impact.Args = cobra.ArbitraryArgs
	impact.Flags().String("base", "", "Git base revision (seeds symbols on changed lines)")
	impact.Flags().StringSlice("files", nil, "Source files to analyze (whole file)")
	impact.Flags().Int("depth", 3, "Maximum impact depth")
	coverage := nativeGraphLeaf("coverage", "Show indexing coverage and missed files", api.OpCoverage, func(*cobra.Command, []string) any { return api.CoverageRequest{RepoRoot: repoRoot()} })
	projects := nativeGraphLeaf("projects", "List indexed graph projects", api.OpProjects, func(*cobra.Command, []string) any { return api.StatusRequest{RepoRoot: repoRoot()} })
	deleteProject := nativeGraphLeaf("delete <project>", "Delete an indexed graph project", api.OpProjectDelete, func(_ *cobra.Command, args []string) any {
		return api.ProjectDeleteRequest{RepoRoot: repoRoot(), Project: args[0]}
	})
	deleteProject.Args = cobra.ExactArgs(1)
	projects.AddCommand(deleteProject)
	artifact := &cobra.Command{Use: "artifact", Short: "Export, import, or verify a content-addressed graph artifact"}
	artifact.AddCommand(
		nativeArtifactLeaf("export <path>", "Write this repository's graph to a content-addressed artifact file", api.OpArtifactExport),
		nativeArtifactLeaf("import <path>", "Load a graph from an artifact file, after verifying it", api.OpArtifactImport),
		nativeArtifactLeaf("verify <path>", "Check an artifact file's integrity without importing it", api.OpArtifactVerify),
	)
	diagnostics := nativeGraphLeaf("diagnostics", "Verify graph database integrity", api.OpDiagnostics, func(*cobra.Command, []string) any { return api.DiagnosticsRequest{RepoRoot: repoRoot()} })
	return []*cobra.Command{status, schema, search, cypher, trace, snippet, architecture, layout, impact, coverage, projects, artifact, diagnostics}
}

func nativeArtifactLeaf(use, short string, operation api.Operation) *cobra.Command {
	command := nativeGraphLeaf(use, short, operation, func(_ *cobra.Command, args []string) any {
		path, _ := filepath.Abs(args[0])
		return api.ArtifactRequest{RepoRoot: repoRoot(), Path: path}
	})
	command.Args = cobra.ExactArgs(1)
	return command
}

func nativeGraphLeaf(use, short string, operation api.Operation, params func(*cobra.Command, []string) any) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		root := repoRoot()
		if operation == api.OpBuild {
			if len(args) == 1 {
				root = args[0]
			}
		}
		if err := failIfUnmanaged(root); err != nil {
			return err
		}
		stale := ""
		switch operation {
		case api.OpQuery, api.OpSearch, api.OpSnippet, api.OpTrace, api.OpImpact, api.OpArchitecture, api.OpCodeSearch, api.OpCypher:
			var freshErr error
			stale, freshErr = ensureFreshGraph(cmd, root)
			if freshErr != nil {
				return freshErr
			}
		}
		client, err := client.Resolve()
		if err != nil {
			return err
		}
		request := params(cmd, args)
		var result json.RawMessage
		if err := client.Call(cmd.Context(), operation, request, &result); err != nil {
			return graphCallError(err)
		}
		text, hints, payload, ok := renderGraphResult(operation, request, result)
		if !ok {
			var display any
			if err := json.Unmarshal(result, &display); err != nil {
				return err
			}
			payload = display
			text = compactGraphText(operation, result)
		}
		if out().Flags.Full {
			var full any
			if err := json.Unmarshal(result, &full); err == nil {
				payload = full
			}
		}
		out().Next(hints...)
		return out().HumanOrJSON("graph_"+strings.ReplaceAll(string(operation), "_", "-"), func() {
			text = applyStale(stale, text)
			fmt.Fprint(cmd.OutOrStdout(), text)
			if !strings.HasSuffix(text, "\n") {
				fmt.Fprintln(cmd.OutOrStdout())
			}
		}, payload)
	}}
}

// graphCallError gives an unknown symbol AXI's not-found shape: the engine
// message (with any nearby names) plus the command that finds the right qn.
func graphCallError(err error) error {
	msg := err.Error()
	if strings.Contains(msg, "graph database was built by another version") {
		return cli.Fail(cli.ExitFail, msg, "so graph build --force")
	}
	i := strings.Index(msg, "symbol not found:")
	if i < 0 {
		return err
	}
	msg = msg[i:]
	name := strings.TrimSpace(strings.SplitN(strings.TrimPrefix(msg, "symbol not found:"), "\n", 2)[0])
	return cli.NotFound(msg, "so graph search "+name)
}

// renderGraphResult turns a graph result into TOON text, AXI next steps and
// the --json payload. Operations it does not know fall back to JSON text.
func renderGraphResult(operation api.Operation, request any, result json.RawMessage) (string, []string, any, bool) {
	switch operation {
	case api.OpQuery:
		var query api.QueryResult
		if json.Unmarshal(result, &query) != nil {
			return "", nil, nil, false
		}
		req, _ := request.(api.QueryRequest)
		return format.QueryCompact(query), format.HelpForQuery(query, req.Question, req.Limit), format.QueryAgentJSON(query), true
	case api.OpSearch:
		var search api.SearchResult
		if json.Unmarshal(result, &search) != nil {
			return "", nil, nil, false
		}
		req, _ := request.(api.SearchRequest)
		return format.SearchCompact(search, req.Query), format.HelpForSearch(search, req.Query, req.Limit), search, true
	case api.OpTrace:
		var trace api.TraceResult
		if json.Unmarshal(result, &trace) != nil {
			return "", nil, nil, false
		}
		return format.TraceCompact(trace), format.HelpForTrace(trace), trace, true
	case api.OpSnippet:
		var snippet api.SnippetResult
		if json.Unmarshal(result, &snippet) != nil {
			return "", nil, nil, false
		}
		snippet.Coverage = api.Coverage{}
		return format.SnippetCompact(snippet), format.HelpForSnippet(snippet), snippet, true
	case api.OpImpact:
		var impact api.ImpactResult
		if json.Unmarshal(result, &impact) != nil {
			return "", nil, nil, false
		}
		return format.ImpactCompact(impact), format.HelpForImpact(impact), impact, true
	case api.OpArchitecture:
		var architecture api.ArchitectureResult
		if json.Unmarshal(result, &architecture) != nil {
			return "", nil, nil, false
		}
		var help []string
		if len(architecture.Packages) > 20 && !out().Flags.Full {
			help = []string{"so graph architecture --full"}
		}
		return format.ArchitectureCompact(architecture, out().Flags.Full), help, architecture, true
	case api.OpStatus:
		var status api.Status
		if json.Unmarshal(result, &status) != nil {
			return "", nil, nil, false
		}
		return format.StatusCompact(status), format.HelpForStatus(status), status, true
	case api.OpBuild:
		var build api.BuildResult
		if json.Unmarshal(result, &build) != nil {
			return "", nil, nil, false
		}
		return format.BuildCompact(build), format.HelpForBuild(build), build, true
	case api.OpProjects:
		var list api.ProjectsResult
		if json.Unmarshal(result, &list) != nil {
			return "", nil, nil, false
		}
		return format.ProjectsCompact(list), nil, list, true
	case api.OpCoverage:
		var coverage api.Coverage
		if json.Unmarshal(result, &coverage) != nil {
			return "", nil, nil, false
		}
		full := out().Flags.Full
		var hints []string
		if !full {
			hints = format.HelpForCoverage(coverage)
		}
		return format.CoverageCompact(coverage, full), hints, coverage, true
	case api.OpSchema:
		var schema api.SchemaResult
		if json.Unmarshal(result, &schema) != nil {
			return "", nil, nil, false
		}
		return format.SchemaCompact(schema), nil, schema, true
	case api.OpDiagnostics:
		var diagnostics api.DiagnosticsResult
		if json.Unmarshal(result, &diagnostics) != nil {
			return "", nil, nil, false
		}
		return format.DiagnosticsCompact(diagnostics), format.HelpForDiagnostics(diagnostics), diagnostics, true
	case api.OpCypher:
		var cypher api.CypherResult
		if json.Unmarshal(result, &cypher) != nil {
			return "", nil, nil, false
		}
		return format.CypherCompact(cypher), format.HelpForCypher(cypher), cypher, true
	case api.OpCodeSearch:
		var search api.CodeSearchResult
		if json.Unmarshal(result, &search) != nil {
			return "", nil, nil, false
		}
		req, _ := request.(api.CodeSearchRequest)
		return format.CodeSearchCompact(search, req.Pattern, req.PathFilter, out().Flags.Full), format.HelpForCodeSearch(search, req.Pattern, req.Limit), search, true
	}
	return "", nil, nil, false
}

// applyStale prepends the freshness state as TOON fields: the answer is from
// the previous index, and these edited files are not in it.
func applyStale(stale, text string) string {
	if stale == "" {
		return text
	}
	var b strings.Builder
	b.WriteString("index: stale; answer is from the previous index and may miss recent edits\n")
	var files []string
	for _, line := range strings.Split(stale, "\n") {
		if strings.HasPrefix(line, "  ") && strings.TrimSpace(line) != "" {
			files = append(files, strings.TrimSpace(line))
		}
	}
	if len(files) > 0 {
		var list strings.Builder
		format.List(&list, "", "stale_files", files)
		b.WriteString(list.String())
	}
	return b.String() + text
}

// compactGraphText renders an operation without a dedicated view as generic
// TOON, so no command falls back to raw JSON on the default path.
func compactGraphText(_ api.Operation, result json.RawMessage) string {
	var value any
	_ = json.Unmarshal(result, &value)
	return cli.Toon(value, out().Flags.Full)
}
