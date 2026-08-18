package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ishanjainn/superopen/internal/agent"
	agentinstall "github.com/ishanjainn/superopen/internal/agent/install"
	"github.com/ishanjainn/superopen/internal/agent/skills"
	"github.com/ishanjainn/superopen/internal/agent/steer"
	codinguninstall "github.com/ishanjainn/superopen/internal/agent/uninstall"
	"github.com/ishanjainn/superopen/internal/cli"
	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/ishanjainn/superopen/internal/graph/client"
	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/projects"
	"github.com/ishanjainn/superopen/internal/session"
	"github.com/ishanjainn/superopen/internal/version"
)

var cliFlags cli.Flags

func out() *cli.Out { return cli.New(cliFlags) }

func main() {
	root := newRootCommand()
	if err := root.Execute(); err != nil {
		out().WriteError(err)
		os.Exit(cli.ExitCode(err))
	}
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "so",
		Short:         "One CLI to rule them all",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE:          runRoot,
	}
	cli.Bind(root, &cliFlags)
	root.Version = version.Display()
	root.SetVersionTemplate("so {{.Version}}\n")
	root.AddCommand(
		cmdInstall(),
		cmdUninstall(),
		cmdInit(),
		cmdGraph(),
		cmdQuery(),
		cmdProjects(),
		cmdSessions(),
		cmdDev(),
		cmdOpen(),
		cmdStatus(),
		agent.NewCmd(),
		version.NewCmd(),
	)
	return root
}

func cmdProjects() *cobra.Command {
	command := &cobra.Command{
		Use:   "projects",
		Short: "List repositories registered with Superopen",
		RunE: func(cmd *cobra.Command, _ []string) error {
			list, err := projects.List()
			if err != nil {
				return err
			}
			return out().HumanOrJSON("projects", func() {
				if len(list) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "No projects registered. Run so init in a repository.")
					return
				}
				for _, p := range list {
					fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  (%s)\n", p.ID, p.Name, p.RepoRoot)
				}
			}, map[string]any{"projects": list})
		},
	}
	return command
}

func runRoot(cmd *cobra.Command, _ []string) error {
	root := repoRoot()
	paths := paths.Resolve(root)
	sessions, _ := session.NewLocalMulti(root, paths).List(cmd.Context(), session.Filter{ProjectID: root})
	active := 0
	for _, item := range sessions {
		if item.Status == session.StatusActive {
			active++
		}
	}
	var graphStatus api.Status
	client, graphErr := client.Resolve()
	if graphErr == nil {
		graphErr = client.Call(cmd.Context(), api.OpStatus, api.StatusRequest{RepoRoot: root}, &graphStatus)
	}
	graphReady := graphErr == nil && graphStatus.State != "missing"
	return out().HumanOrJSON("status", func() {
		fmt.Fprintf(out().W, "sessions  %d  active=%d\n", len(sessions), active)
		fmt.Fprintf(out().W, "graph  %v  engine=%s nodes=%d edges=%d\n", graphReady, api.EngineName, graphStatus.NodeCount, graphStatus.EdgeCount)
	}, map[string]any{
		"repo": root, "sessions": len(sessions), "active_sessions": active,
		"graph": graphReady, "graph_status": graphStatus,
	})
}

func repoRoot() string {
	if configured := strings.TrimSpace(cliFlags.Root); configured != "" {
		return absPath(configured)
	}
	if configured := strings.TrimSpace(os.Getenv("SUPEROPEN_ROOT")); configured != "" {
		return absPath(configured)
	}
	wd, _ := os.Getwd()
	root, err := paths.FindRoot(wd)
	if err != nil {
		return wd
	}
	return root
}

// initRoot is the repository root used by `so init`.
// Default preference: existing .so / git top-level via FindRoot (one graph per
// repo). Explicit package graphs use --root or SUPEROPEN_ROOT.
func initRoot() string {
	if configured := strings.TrimSpace(cliFlags.Root); configured != "" {
		return absPath(configured)
	}
	if configured := strings.TrimSpace(os.Getenv("SUPEROPEN_ROOT")); configured != "" {
		return absPath(configured)
	}
	wd, _ := os.Getwd()
	root, err := paths.FindRoot(wd)
	if err != nil {
		return absPath(wd)
	}
	return root
}

func absPath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

func gitTopLevel(dir string) string {
	// Best-effort: used only for messaging when init creates a nested package graph.
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func cmdInstall() *cobra.Command {
	var vendors []string
	command := &cobra.Command{
		Use:   "install",
		Short: "Install /so skill and coding-agent observability hooks (user-global)",
		Long: `Install the Superopen skill and observability hooks for supported coding agents.

Run from any directory (not tied to a repository). After install, open your
agent and run /so init inside a repo to create .so/ and build the graph.

Installs the /so skill, observability hooks, durable graph-first guidance,
and user-global MCP entries (repo-neutral; no project files written).`,
		RunE: func(*cobra.Command, []string) error {
			if err := agent.Install(repoRoot(), vendors); err != nil {
				return err
			}
			fmt.Println("Installed user-global /so skill, hooks, graph-first guidance, and MCP.")
			fmt.Println("Next: open your coding agent and run /so init in a repository.")
			return nil
		},
	}
	command.Flags().StringSliceVar(&vendors, "vendor", nil, "Install selected vendor hooks (default: all supported)")
	return command
}

func cmdUninstall() *cobra.Command {
	var keepData bool
	command := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove coding-agent observability hooks, /so skills, and optional local session data",
		RunE: func(*cobra.Command, []string) error {
			_, warnings := codinguninstall.RemoveAll(true, false, os.Stdout, os.Stderr)
			if len(warnings) > 0 {
				return fmt.Errorf("uninstall hooks: %s", strings.Join(warnings, "; "))
			}
			for _, path := range skills.RemoveAll() {
				fmt.Printf("removed skill %s\n", path)
			}
			for _, path := range steer.RemoveAll() {
				fmt.Printf("removed guidance %s\n", path)
			}
			for _, path := range agentinstall.RemoveUserMCP() {
				fmt.Printf("removed mcp %s\n", path)
			}
			if !keepData {
				if err := os.RemoveAll(filepath.Join(repoRoot(), paths.DirName)); err != nil {
					return err
				}
			}
			return nil
		},
	}
	command.Flags().BoolVar(&keepData, "keep-data", false, "Keep .so session data")
	return command
}

func cmdInit() *cobra.Command {
	var vendors []string
	var force bool
	var withDev bool
	command := &cobra.Command{
		Use:   "init",
		Short: "Initialize .so/ session storage and the native graph for this repository",
		Long: `Per-repository setup. Prefer so install (once, user-global) before /so init.

Creates .so/sessions and .so/db, writes .so/.gitignore, builds the graph,
and registers the project in the user-wide Superopen index.

Init defaults to the repository root (nearest existing .so or git top-level).
Pass --root / SUPEROPEN_ROOT for an explicit nested package graph.

Does not start so dev by default. Pass --dev to start the UI after init.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root := initRoot()
			layout := paths.Resolve(root)
			if err := layout.EnsureDirs(); err != nil {
				return err
			}
			ignorePath := filepath.Join(layout.Root, ".gitignore")
			if _, err := os.Stat(ignorePath); os.IsNotExist(err) {
				if err := os.WriteFile(ignorePath, soGitignoreContents(), 0o644); err != nil {
					return err
				}
			}
			// Hooks/skills/MCP should already exist from `so install`; re-run is idempotent.
			if err := agent.Install(root, vendors); err != nil {
				return err
			}
			client, err := client.Resolve()
			if err != nil {
				return err
			}
			var result api.BuildResult
			if err := client.Call(cmd.Context(), api.OpBuild, api.BuildRequest{RepoRoot: root, Mode: "full", Force: force}, &result); err != nil {
				return err
			}
			_ = projects.TouchInit(root)
			_ = projects.TouchGraphRefresh(root)
			fmt.Printf("Initialized native graph: %d nodes, %d edges (%s)\n", result.NodeCount, result.EdgeCount, result.Status)
			fmt.Printf("Repo root:    %s\n", root)
			if top := gitTopLevel(root); top != "" && filepath.Clean(top) != filepath.Clean(root) {
				fmt.Printf("Package-scoped graph (git top-level is %s)\n", top)
			}
			fmt.Printf("Session data: %s\n", layout.SessionsDir)
			fmt.Printf("Shared DB:    %s\n", layout.Database)
			if withDev {
				fmt.Println("Starting so dev -d (UI + live watcher; MCP is user-global)...")
				prev := cliFlags.Root
				cliFlags.Root = root
				defer func() { cliFlags.Root = prev }()
				return runDev(4444, true, true)
			}
			fmt.Println("Optional: so dev -d  (UI + live watcher)")
			return nil
		},
	}
	command.Flags().StringSliceVar(&vendors, "vendor", nil, "Install selected vendor observability hooks")
	command.Flags().BoolVar(&force, "force", false, "Force native graph rebuild")
	command.Flags().BoolVar(&withDev, "dev", false, "After init, start so dev -d (idempotent if already running)")
	return command
}

func cmdGraph() *cobra.Command { return newGraphCommand() }

func cmdQuery() *cobra.Command {
	command := nativeGraphLeaf("query <question>", "Query the native repository graph", api.OpQuery, func(cmd *cobra.Command, args []string) any {
		depth, _ := cmd.Flags().GetInt("depth")
		budget, _ := cmd.Flags().GetInt("budget")
		return api.QueryRequest{RepoRoot: repoRoot(), Question: args[0], Depth: depth, Budget: budget}
	})
	command.Args = cobra.ExactArgs(1)
	command.Flags().Int("depth", 2, "Traversal depth")
	command.Flags().Int("budget", 1200, "Approximate output token budget")
	return command
}

// soGitignoreContents prefers templates/so.gitignore when present in a source
// checkout; installed binaries fall back to the same committed content.
func soGitignoreContents() []byte {
	candidates := []string{filepath.Join("templates", "so.gitignore")}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "templates", "so.gitignore"),
			filepath.Join(dir, "..", "templates", "so.gitignore"),
			filepath.Join(dir, "..", "..", "templates", "so.gitignore"),
		)
	}
	for _, candidate := range candidates {
		data, err := os.ReadFile(candidate)
		if err == nil && len(bytes.TrimSpace(data)) > 0 {
			return data
		}
	}
	return []byte("# Superopen machine-local data (do not commit).\nsessions/\ndb/\n")
}
