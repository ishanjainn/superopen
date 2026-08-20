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
	codinguninstall "github.com/ishanjainn/superopen/internal/agent/uninstall"
	"github.com/ishanjainn/superopen/internal/cli"
	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/ishanjainn/superopen/internal/graph/client"
	"github.com/ishanjainn/superopen/internal/graph/watch"
	"github.com/ishanjainn/superopen/internal/memory"
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
		cmdMemory(),
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
	command.AddCommand(&cobra.Command{
		Use:   "prune",
		Short: "Remove missing, non-git, home, and scratch entries from the project index",
		RunE: func(cmd *cobra.Command, _ []string) error {
			removed, err := projects.PruneInvalid(false)
			if err != nil {
				return err
			}
			return out().HumanOrJSON("projects_prune", func() {
				if len(removed) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "Nothing to prune.")
					return
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Removed %d project(s):\n", len(removed))
				for _, res := range removed {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s\n", res.Project.ID, res.Project.RepoRoot)
				}
			}, map[string]any{"pruned": len(removed), "results": removed})
		},
	})
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

// hookRepoAndSession prefers the editor workspace from hook stdin.
// Cursor runs ~/.cursor/hooks.json with cwd ~/.cursor, which is not the repo
// (and may even have a stray .so/ from a previous detached refresh).
func hookRepoAndSession() (root, sessionID string) {
	root = repoRoot()
	if strings.TrimSpace(cliFlags.Root) != "" || strings.TrimSpace(os.Getenv("SUPEROPEN_ROOT")) != "" {
		return root, ""
	}
	hook := cli.ReadHookStdin(os.Stdin)
	if hook.Workspace == "" {
		return root, hook.SessionID
	}
	found, err := paths.FindRoot(hook.Workspace)
	if err == nil && found != "" {
		root = found
	} else {
		root = absPath(hook.Workspace)
	}
	return root, hook.SessionID
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
			_ = memory.FetchModels()
			fmt.Println("Installed user-global /so skill, hooks, graph-first guidance, and MCP.")
			if findWebDir("") == "" {
				fmt.Printf("note: web UI is not in %s; re-run sh scripts/install.sh (or brew) so so dev works from any repo.\n", expectedWebDir())
			}
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
		Short: "Remove Superopen agent wiring and machine-local data",
		Long: `Remove Superopen from this machine. Works from any directory on
macOS, Linux, and Windows. No source checkout is required.

Removes:
  - hooks, /so skill, MCP, durable guidance, and subagents for every
    supported coding agent (Claude Code, Cursor, Codex, Gemini CLI,
    OpenCode, Copilot CLI, Pi)
  - project index (config dir)
  - marketplace copy (data dir)
  - session-state caches
  - registered repositories' .so data (unless --keep-data)
  - release-installer prefix (~/.superopen), including that channel's binary

Does not remove a package-managed so binary (Homebrew, Scoop, WinGet,
Chocolatey). Use that manager's uninstall after this command.
`,
		RunE: func(*cobra.Command, []string) error {
			_, warnings := codinguninstall.RemoveAll(true, keepData, false, os.Stdout, os.Stderr)
			for _, w := range warnings {
				fmt.Fprintf(os.Stderr, "so uninstall: %s\n", w)
			}
			if exe, err := os.Executable(); err == nil {
				if hint := paths.PackageManagerUninstallHint(exe); hint != "" {
					fmt.Printf("The so binary is still provided by a package manager. Remove it with: %s\n", hint)
				}
			}
			fmt.Println("Restart your coding agent so it drops in-memory hooks and MCP.")
			return nil
		},
	}
	command.Flags().BoolVar(&keepData, "keep-data", false, "Keep per-repo .so session/graph data")
	return command
}

func cmdInit() *cobra.Command {
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

Does not re-run user-global so install (hooks/MCP). Does not rebuild an
existing graph unless --force is passed.

Does not start so dev by default. Pass --dev to start the UI after init.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			root := initRoot()
			fmt.Fprintf(out, "so init: %s\n", root)
			if f, ok := out.(*os.File); ok {
				_ = f.Sync()
			}
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
			_, dbErr := os.Stat(layout.Database)
			already := dbErr == nil
			if already && !force {
				fmt.Fprintf(out, "Already initialized. Graph DB: %s\n", layout.Database)
				fmt.Fprintf(out, "Session data: %s\n", layout.SessionsDir)
				fmt.Fprintf(out, "Rebuild with: so init --force\n")
				_ = projects.TouchInit(root)
				return nil
			}
			client, err := client.Resolve()
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Building native graph...\n")
			if f, ok := out.(*os.File); ok {
				_ = f.Sync()
			}
			var result api.BuildResult
			if err := client.Call(cmd.Context(), api.OpBuild, api.BuildRequest{RepoRoot: root, Mode: "full", Force: force}, &result); err != nil {
				return err
			}
			watch.RecordSignature(root)
			_ = projects.TouchInit(root)
			_ = projects.TouchGraphRefresh(root)
			if result.Status != "" && result.Status != "ok" {
				fmt.Fprintf(out, "Initialized native graph: %d nodes, %d edges (%s)\n", result.NodeCount, result.EdgeCount, result.Status)
			} else {
				fmt.Fprintf(out, "Initialized native graph: %d nodes, %d edges\n", result.NodeCount, result.EdgeCount)
			}
			fmt.Fprintf(out, "Repo root:    %s\n", root)
			if top := gitTopLevel(root); top != "" && filepath.Clean(top) != filepath.Clean(root) {
				fmt.Fprintf(out, "Package-scoped graph (git top-level is %s)\n", top)
			}
			fmt.Fprintf(out, "Session data: %s\n", layout.SessionsDir)
			fmt.Fprintf(out, "Shared DB:    %s\n", layout.Database)
			if withDev {
				fmt.Fprintln(out, "Starting so dev -d (UI + live watcher; MCP is user-global)...")
				prev := cliFlags.Root
				cliFlags.Root = root
				defer func() { cliFlags.Root = prev }()
				return runDev(4444, true, true, false)
			}
			fmt.Fprintln(out, "Optional: so dev -d  (UI + live watcher)")
			return nil
		},
	}
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
