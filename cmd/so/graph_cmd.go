package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ishanjainn/superopen/internal/cli"
	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/ishanjainn/superopen/internal/graph/buildpool"
	"github.com/ishanjainn/superopen/internal/graph/client"
	"github.com/ishanjainn/superopen/internal/graph/engine"
	"github.com/ishanjainn/superopen/internal/graph/format"
	"github.com/ishanjainn/superopen/internal/projects"
)

// newGraphCommand exposes the native graph engine. There is no
// provider selection or compatibility path: Superopen has one graph.
func newGraphCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "graph",
		Short: "Build and query the native repository graph",
		// With no subcommand the group shows live state (AXI §8), not help.
		Args: cobra.NoArgs,
	}
	command.RunE = func(cmd *cobra.Command, _ []string) error {
		for _, c := range command.Commands() {
			if c.Name() == "status" {
				c.SetContext(cmd.Context())
				return c.RunE(c, nil)
			}
		}
		return cmd.Help()
	}
	command.PersistentFlags().Bool("no-refresh", false, "Skip query-path graph freshness (CI)")
	command.AddCommand(graphNativeCommands()...)
	return command
}

func runGraphRefresh(cmd *cobra.Command, root string, force, fromProbe bool) error {
	if err := failIfUnmanaged(root); err != nil {
		return err
	}
	c, err := client.Resolve()
	if err != nil {
		return err
	}
	var result api.BuildResult
	req := api.BuildRequest{RepoRoot: root, Mode: "full", Force: force, Incremental: !force, FromProbe: fromProbe}
	if err := c.Call(cmd.Context(), api.OpBuild, req, &result); err != nil {
		return err
	}
	if result.Status == "pool_full" || result.Status == "refresh_in_progress" {
		reason := "already in progress"
		if result.Status == "pool_full" {
			n := buildpool.SlotCount()
			reason = fmt.Sprintf("build pool full (%d/%d)", n, n)
		}
		out().Next("so graph status")
		return out().HumanOrJSON("graph_refresh", func() {
			fmt.Fprintf(cmd.OutOrStdout(), "refresh: skipped\nreason: %s\n", reason)
		}, result)
	}
	_ = engine.WriteFingerprint(cmd.Context(), root, nil)
	_ = projects.TouchGraphRefresh(root)
	out().Next(format.HelpForBuild(result)...)
	return out().HumanOrJSON("graph_refresh", func() {
		fmt.Fprint(cmd.OutOrStdout(), format.BuildCompact(result))
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
	build.Flags().Bool("force", false, "Rebuild even when the source revision is unchanged")
	build.Flags().String("mode", "full", "Index mode")

	refresh := &cobra.Command{
		Use:   "refresh",
		Short: "Incrementally refresh the native repository graph",
		RunE: func(cmd *cobra.Command, _ []string) error {
			detach, _ := cmd.Flags().GetBool("detach")
			force, _ := cmd.Flags().GetBool("force")
			fromProbe, _ := cmd.Flags().GetBool("probe")
			root, _ := hookRepoAndSession()
			if err := failIfUnmanaged(root); err != nil {
				return err
			}
			if detach {
				if engine.BuildBusy(root) || engine.BuildPoolFull() {
					return nil
				}
				args := []string{"--root", root, "graph", "refresh"}
				if force {
					args = append(args, "--force")
				}
				if fromProbe {
					args = append(args, "--probe")
				}
				cli.SpawnSO(root, args...)
				return nil
			}
			return runGraphRefresh(cmd, root, force, fromProbe)
		},
	}
	refresh.Flags().Bool("detach", false, "Run refresh in a detached background process")
	refresh.Flags().Bool("force", false, "Force a full rebuild")
	refresh.Flags().Bool("probe", false, "Use the size+mtime fingerprint instead of hashing every file")

	query := nativeGraphLeaf("query <question>", "Retrieve focused graph context", api.OpQuery, func(cmd *cobra.Command, args []string) any {
		depth, _ := cmd.Flags().GetInt("depth")
		terms, _ := cmd.Flags().GetStringSlice("term")
		limit, _ := cmd.Flags().GetInt("limit")
		offset, _ := cmd.Flags().GetInt("offset")
		return api.QueryRequest{
			RepoRoot: repoRoot(),
			Question: args[0],
			Terms:    terms,
			Depth:    depth,
			Limit:    limit,
			Offset:   offset,
		}
	})
	query.Args = cobra.ExactArgs(1)
	query.Flags().Int("depth", 2, "Traversal depth")
	query.Flags().StringSlice("term", nil, "Additional exact graph seed")
	query.Flags().Int("limit", 0, "Ranked rows per page (default 15)")
	query.Flags().Int("offset", 0, "Skip this many ranked rows (from a previous page's hint)")

	codeSearch := nativeGraphLeaf("code-search <pattern> [path...]", "Search indexed source code", api.OpCodeSearch, func(cmd *cobra.Command, args []string) any {
		limit, _ := cmd.Flags().GetInt("limit")
		regex, _ := cmd.Flags().GetBool("regex")
		filePattern, _ := cmd.Flags().GetString("file-pattern")
		if glob, _ := cmd.Flags().GetString("glob"); glob != "" {
			filePattern = glob
		}
		path, _ := cmd.Flags().GetString("path")
		var paths []string
		if len(args) > 1 {
			paths = args[1:]
			path = strings.Join(paths, ", ")
		}
		return api.CodeSearchRequest{
			RepoRoot:   repoRoot(),
			Pattern:    args[0],
			Regex:      regex,
			FileGlob:   filePattern,
			PathFilter: path,
			Paths:      paths,
			Limit:      limit,
		}
	})
	codeSearch.Args = cobra.MinimumNArgs(1)
	codeSearch.Flags().Int("limit", 20, "Maximum results")
	codeSearch.Flags().Bool("regex", false, "Interpret pattern as a regular expression")
	codeSearch.Flags().String("file-pattern", "", "Restrict source files by glob (e.g. *.py)")
	codeSearch.Flags().String("glob", "", "Same as --file-pattern")
	codeSearch.Flags().String("path", "", "Restrict to a repo-relative file or directory (same as the [path] argument)")

	commands := []*cobra.Command{build, refresh, query, codeSearch, graphBuildsCommand()}
	commands = append(commands, graphNativeReadCommands()...)
	return commands
}

func graphBuildsCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "builds",
		Short: "Show the global graph build pool",
		Args:  cobra.NoArgs,
	}
	command.RunE = func(cmd *cobra.Command, _ []string) error {
		for _, c := range command.Commands() {
			if c.Name() == "status" {
				c.SetContext(cmd.Context())
				return c.RunE(c, nil)
			}
		}
		return cmd.Help()
	}
	status := &cobra.Command{
		Use:   "status",
		Short: "List active graph builds across repositories",
		RunE: func(cmd *cobra.Command, _ []string) error {
			slots, err := buildpool.List()
			if err != nil {
				return err
			}
			active := slots[:0:0]
			for _, sl := range slots {
				if sl.PID != 0 || sl.Repo != "" {
					active = append(active, sl)
				}
			}
			slots = active
			n := buildpool.SlotCount()
			return out().HumanOrJSON("graph_builds", func() {
				w := cmd.OutOrStdout()
				if n == 0 {
					fmt.Fprintln(w, "pool: unlimited")
				} else {
					fmt.Fprintf(w, "pool: %d of %d slots in use\n", len(slots), n)
				}
				if len(slots) == 0 {
					fmt.Fprintln(w, "builds: none running")
					return
				}
				rows := make([][]string, 0, len(slots))
				for _, sl := range slots {
					started := ""
					if !sl.StartedAt.IsZero() {
						started = sl.StartedAt.Format("15:04:05")
					}
					rows = append(rows, []string{fmt.Sprint(sl.Index), fmt.Sprint(sl.PID), sl.Repo, started})
				}
				var b strings.Builder
				format.Table(&b, "", "builds", []string{"slot", "pid", "repo", "started"}, rows)
				fmt.Fprint(w, b.String())
			}, map[string]any{"slots": n, "active": slots})
		},
	}
	command.AddCommand(status)
	return command
}
