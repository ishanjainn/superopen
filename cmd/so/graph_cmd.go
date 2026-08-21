package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ishanjainn/superopen/internal/cli"
	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/ishanjainn/superopen/internal/graph/buildpool"
	"github.com/ishanjainn/superopen/internal/graph/client"
	"github.com/ishanjainn/superopen/internal/graph/engine"
	"github.com/ishanjainn/superopen/internal/graph/watch"
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
	if skipIfUnmanaged(cmd, root) {
		return nil
	}
	c, err := client.Resolve()
	if err != nil {
		return err
	}
	var result api.BuildResult
	req := api.BuildRequest{RepoRoot: root, Mode: "full", Force: force, Incremental: !force}
	if err := c.Call(cmd.Context(), api.OpBuild, req, &result); err != nil {
		return err
	}
	if result.Status == "pool_full" || result.Status == "refresh_in_progress" {
		return out().HumanOrJSON("graph_refresh", func() {
			if result.Status == "pool_full" {
				n := buildpool.SlotCount()
				fmt.Fprintf(cmd.OutOrStdout(), "graph refresh skipped: build pool full (%d/%d)\n", n, n)
				return
			}
			fmt.Fprintln(cmd.OutOrStdout(), "graph refresh skipped: already in progress")
		}, result)
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
			if skipIfUnmanaged(cmd, root) {
				return nil
			}
			if detach {
				// Silent on stdout: Claude attaches hook stdout as
				// model-visible content. Observability still records
				// via coding hooks; refresh just must not speak.
				if engine.BuildBusy(root) || engine.BuildPoolFull() {
					return nil
				}
				args := []string{"--root", root, "graph", "refresh"}
				if force {
					args = append(args, "--force")
				}
				cli.SpawnSO(root, args...)
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
	query.Flags().Int("budget", 2000, "Approximate output token budget")
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

	commands := []*cobra.Command{build, refresh, query, codeSearch, graphBuildsCommand()}
	commands = append(commands, graphNativeReadCommands()...)
	return commands
}

func graphBuildsCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "builds",
		Short: "Show the global graph build pool",
	}
	status := &cobra.Command{
		Use:   "status",
		Short: "List active graph builds across repositories",
		RunE: func(cmd *cobra.Command, _ []string) error {
			slots, err := buildpool.List()
			if err != nil {
				return err
			}
			n := buildpool.SlotCount()
			return out().HumanOrJSON("graph_builds", func() {
				if n == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "build pool unlimited")
					return
				}
				fmt.Fprintf(cmd.OutOrStdout(), "build pool %d/%d in use\n", len(slots), n)
				for _, s := range slots {
					started := ""
					if !s.StartedAt.IsZero() {
						started = " started=" + s.StartedAt.Format("15:04:05")
					}
					fmt.Fprintf(cmd.OutOrStdout(), "  slot %d  pid=%d  %s%s\n", s.Index, s.PID, s.Repo, started)
				}
			}, map[string]any{"slots": n, "active": slots})
		},
	}
	command.AddCommand(status)
	return command
}
