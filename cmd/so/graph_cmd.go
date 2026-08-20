package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ishanjainn/superopen/internal/cli"
	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/ishanjainn/superopen/internal/graph/client"
	"github.com/ishanjainn/superopen/internal/graph/engine"
	"github.com/ishanjainn/superopen/internal/graph/mcp"
	"github.com/ishanjainn/superopen/internal/graph/watch"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/projects"
)

// newGraphCommand exposes the native graph engine. There is no
// provider selection or compatibility path: Superopen has one graph.
func newGraphCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "graph",
		Short: "Build and query the native repository graph",
	}
	command.AddCommand(graphNativeCommands()...)
	return command
}

func runGraphRefresh(cmd *cobra.Command, root string, force bool) error {
	c, err := client.Resolve()
	if err != nil {
		return err
	}
	var result api.BuildResult
	req := api.BuildRequest{RepoRoot: root, Mode: "full", Force: force, Incremental: !force}
	if err := c.Call(cmd.Context(), api.OpBuild, req, &result); err != nil {
		return err
	}
	watch.RecordSignature(root)
	_ = projects.TouchGraphRefresh(root)
	return out().HumanOrJSON("graph_refresh", func() {
		fmt.Fprintf(cmd.OutOrStdout(), "graph refresh: status=%s nodes=%d edges=%d\n", result.Status, result.NodeCount, result.EdgeCount)
	}, result)
}

func graphNativeCommands() []*cobra.Command {
	build := nativeGraphLeaf("build [path]", "Build the native repository graph", api.OpBuild, func(cmd *cobra.Command, args []string) any {
		root := repoRoot()
		if len(args) == 1 {
			root = args[0]
		}
		force, _ := cmd.Flags().GetBool("force")
		mode, _ := cmd.Flags().GetString("mode")
		return api.BuildRequest{RepoRoot: root, Force: force, Mode: mode}
	})
	build.Args = cobra.MaximumNArgs(1)
	build.Aliases = []string{"extract", "rebuild"}
	build.Flags().Bool("force", false, "Rebuild even when the source revision is unchanged")
	build.Flags().String("mode", "full", "Index mode")

	refresh := &cobra.Command{
		Use:   "refresh",
		Short: "Incrementally refresh the native repository graph",
		RunE: func(cmd *cobra.Command, _ []string) error {
			detach, _ := cmd.Flags().GetBool("detach")
			force, _ := cmd.Flags().GetBool("force")
			root, _ := hookRepoAndSession()
			if detach {
				if engine.BuildBusy(root) {
					fmt.Fprintln(cmd.OutOrStdout(), "graph refresh skipped: already in progress")
					return nil
				}
				args := []string{"--root", root, "graph", "refresh"}
				if force {
					args = append(args, "--force")
				}
				cli.SpawnSO(root, args...)
				fmt.Fprintln(cmd.OutOrStdout(), "graph refresh started in background")
				return nil
			}
			return runGraphRefresh(cmd, root, force)
		},
	}
	refresh.Flags().Bool("detach", false, "Run refresh in a detached background process")
	refresh.Flags().Bool("force", false, "Force a full rebuild")

	query := nativeGraphLeaf("query <question>", "Retrieve focused graph context", api.OpQuery, func(cmd *cobra.Command, args []string) any {
		depth, _ := cmd.Flags().GetInt("depth")
		budget, _ := cmd.Flags().GetInt("budget")
		terms, _ := cmd.Flags().GetStringSlice("term")
		return api.QueryRequest{
			RepoRoot: repoRoot(),
			Question: args[0],
			Terms:    terms,
			Depth:    depth,
			Budget:   budget,
		}
	})
	query.Args = cobra.ExactArgs(1)
	query.Flags().Int("depth", 2, "Traversal depth")
	query.Flags().Int("budget", 1200, "Approximate output token budget")
	query.Flags().StringSlice("term", nil, "Additional exact graph seed")

	codeSearch := nativeGraphLeaf("code-search <pattern>", "Search indexed source code", api.OpCodeSearch, func(cmd *cobra.Command, args []string) any {
		limit, _ := cmd.Flags().GetInt("limit")
		regex, _ := cmd.Flags().GetBool("regex")
		filePattern, _ := cmd.Flags().GetString("file-pattern")
		return api.CodeSearchRequest{
			RepoRoot: repoRoot(),
			Pattern:  args[0],
			Regex:    regex,
			FileGlob: filePattern,
			Limit:    limit,
		}
	})
	codeSearch.Args = cobra.ExactArgs(1)
	codeSearch.Flags().Int("limit", 20, "Maximum results")
	codeSearch.Flags().Bool("regex", false, "Interpret pattern as a regular expression")
	codeSearch.Flags().String("file-pattern", "", "Restrict source files")

	commands := []*cobra.Command{build, refresh, query, codeSearch}
	commands = append(commands, graphNativeReadCommands()...)
	commands = append(commands, graphMCPCommand())
	return commands
}

func graphMCPCommand() *cobra.Command {
	var root string
	command := &cobra.Command{Use: "mcp", Short: "Serve the native graph to coding agents"}
	serve := &cobra.Command{
		Use:   "serve",
		Short: "Serve native graph tools over MCP stdio",
		RunE: func(cmd *cobra.Command, _ []string) error {
			selected := strings.TrimSpace(root)
			if selected == "" || selected == "." {
				selected = repoRoot()
			}
			absolute, err := filepath.Abs(selected)
			if err != nil {
				return err
			}
			c, err := client.Resolve()
			if err != nil {
				return err
			}
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			go memory.EnsureEmbedWorker()
			return (mcp.Server{Root: absolute, Client: c}).Serve(cmd.Context(), os.Stdin, os.Stdout)
		},
	}
	serve.Flags().StringVar(&root, "root", "", "Repository root (default: cwd / SUPEROPEN_ROOT / nearest .so or git root)")
	config := &cobra.Command{
		Use:   "config",
		Short: "Print user-global MCP configuration snippet (diagnostic; so install wires this automatically)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			executable, err := os.Executable()
			if err != nil {
				return err
			}
			body, _ := json.MarshalIndent(map[string]any{
				"mcpServers": map[string]any{
					"superopen": map[string]any{
						"command": executable,
						"args":    []string{"graph", "mcp", "serve"},
					},
				},
			}, "", "  ")
			fmt.Fprintln(cmd.OutOrStdout(), string(body))
			return nil
		},
	}
	command.AddCommand(serve, config)
	return command
}
