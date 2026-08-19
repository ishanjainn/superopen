package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

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
		request := api.TraceRequest{RepoRoot: repoRoot(), Start: args[0], Direction: direction, Depth: depth, Limit: limit}
		if len(args) > 1 {
			request.Target = args[1]
		}
		return request
	})
	trace.Args = cobra.RangeArgs(1, 2)
	trace.Flags().String("direction", "outgoing", "Traversal direction")
	trace.Flags().Int("depth", 3, "Maximum traversal depth")
	trace.Flags().Int("limit", 100, "Maximum visited results")
	snippet := nativeGraphLeaf("snippet <qualified-name>", "Read source for an indexed symbol", api.OpSnippet, func(cmd *cobra.Command, args []string) any {
		contextLines, _ := cmd.Flags().GetInt("context")
		return api.SnippetRequest{RepoRoot: repoRoot(), QualifiedName: args[0], ContextLines: contextLines}
	})
	snippet.Args = cobra.ExactArgs(1)
	snippet.Flags().Int("context", 0, "Neighboring source lines")
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
		depth, _ := cmd.Flags().GetInt("depth")
		return api.ImpactRequest{RepoRoot: repoRoot(), Base: base, Symbols: args, Depth: depth}
	})
	impact.Args = cobra.ArbitraryArgs
	impact.Flags().String("base", "", "Git base revision")
	impact.Flags().Int("depth", 3, "Maximum impact depth")
	coverage := nativeGraphLeaf("coverage", "Show indexing coverage and missed files", api.OpCoverage, func(*cobra.Command, []string) any { return api.CoverageRequest{RepoRoot: repoRoot()} })
	projects := nativeGraphLeaf("projects", "List indexed graph projects", api.OpProjects, func(*cobra.Command, []string) any { return api.StatusRequest{RepoRoot: repoRoot()} })
	deleteProject := nativeGraphLeaf("delete <project>", "Delete an indexed graph project", api.OpProjectDelete, func(_ *cobra.Command, args []string) any {
		return api.ProjectDeleteRequest{RepoRoot: repoRoot(), Project: args[0]}
	})
	deleteProject.Args = cobra.ExactArgs(1)
	projects.AddCommand(deleteProject)
	artifact := &cobra.Command{Use: "artifact", Short: "Export, import, or verify a content-addressed graph artifact"}
	artifact.AddCommand(nativeArtifactLeaf("export <path>", api.OpArtifactExport), nativeArtifactLeaf("import <path>", api.OpArtifactImport), nativeArtifactLeaf("verify <path>", api.OpArtifactVerify))
	diagnostics := nativeGraphLeaf("diagnostics", "Verify graph database integrity", api.OpDiagnostics, func(*cobra.Command, []string) any { return api.DiagnosticsRequest{RepoRoot: repoRoot()} })
	return []*cobra.Command{status, schema, search, cypher, trace, snippet, architecture, layout, impact, coverage, projects, artifact, diagnostics}
}

func nativeArtifactLeaf(use string, operation api.Operation) *cobra.Command {
	command := nativeGraphLeaf(use, "Operate on a verified Zstandard graph artifact", operation, func(_ *cobra.Command, args []string) any {
		path, _ := filepath.Abs(args[0])
		return api.ArtifactRequest{RepoRoot: repoRoot(), Path: path}
	})
	command.Args = cobra.ExactArgs(1)
	return command
}

func nativeGraphLeaf(use, short string, operation api.Operation, params func(*cobra.Command, []string) any) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		client, err := client.Resolve()
		if err != nil {
			return err
		}
		var result json.RawMessage
		if err := client.Call(cmd.Context(), operation, params(cmd, args), &result); err != nil {
			return err
		}
		if operation == api.OpQuery {
			var query api.QueryResult
			if err := json.Unmarshal(result, &query); err != nil {
				return err
			}
			return out().HumanOrJSON("graph_query", func() {
				fmt.Fprint(cmd.OutOrStdout(), query.Text)
				if !strings.HasSuffix(query.Text, "\n") {
					fmt.Fprintln(cmd.OutOrStdout())
				}
			}, query)
		}
		var display any
		if err := json.Unmarshal(result, &display); err != nil {
			return err
		}
		return out().HumanOrJSON("graph_"+strings.ReplaceAll(string(operation), "_", "-"), func() {
			text := compactGraphText(operation, result)
			fmt.Fprint(cmd.OutOrStdout(), text)
			if !strings.HasSuffix(text, "\n") {
				fmt.Fprintln(cmd.OutOrStdout())
			}
		}, display)
	}}
}

func compactGraphText(operation api.Operation, result json.RawMessage) string {
	switch operation {
	case api.OpSearch:
		var search api.SearchResult
		if json.Unmarshal(result, &search) == nil {
			return format.SearchCompact(search)
		}
	case api.OpTrace:
		var trace api.TraceResult
		if json.Unmarshal(result, &trace) == nil {
			return format.TraceCompact(trace)
		}
	case api.OpSnippet:
		var snippet api.SnippetResult
		if json.Unmarshal(result, &snippet) == nil {
			return format.SnippetCompact(snippet)
		}
	case api.OpArchitecture:
		var architecture api.ArchitectureResult
		if json.Unmarshal(result, &architecture) == nil {
			return format.ArchitectureCompact(architecture)
		}
	}
	var pretty any
	_ = json.Unmarshal(result, &pretty)
	body, _ := json.MarshalIndent(pretty, "", "  ")
	return string(body) + "\n"
}
