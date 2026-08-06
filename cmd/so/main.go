package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/superopen/so/internal/axi"
	"github.com/superopen/so/internal/checkpoint"
	"github.com/superopen/so/internal/coding"
	codinguninstall "github.com/superopen/so/internal/coding/uninstall"
	"github.com/superopen/so/internal/config"
	"github.com/superopen/so/internal/discover"
	"github.com/superopen/so/internal/doctor"
	"github.com/superopen/so/internal/eval"
	"github.com/superopen/so/internal/guardrails"
	"github.com/superopen/so/internal/graph"
	"github.com/superopen/so/internal/harness"
	"github.com/superopen/so/internal/harnessvalid"
	"github.com/superopen/so/internal/harvest"
	initcmd "github.com/superopen/so/internal/initcmd"
	"github.com/superopen/so/internal/inject"
	"github.com/superopen/so/internal/llm"
	"github.com/superopen/so/internal/otlp"
	"github.com/superopen/so/internal/projects"
	"github.com/superopen/so/internal/recommend"
	"github.com/superopen/so/internal/seed"
	"github.com/superopen/so/internal/session"
	"github.com/superopen/so/internal/sync"
	"github.com/superopen/so/internal/tracestore"
	"github.com/superopen/so/internal/version"
	"github.com/superopen/so/internal/viz"
)

var axiFlags axi.Flags

func out() *axi.Out { return axi.New(axiFlags) }

func main() {
	root := &cobra.Command{
		Use:   "so",
		Short: "Superopen - Open source Agent Harness Engineering",
		Long: `Superopen CLI (AXI: compact text by default; --json / --full for agents).

  go install ./cmd/so && so install   # CLI + /so skill for coding agents
  so init                            # bootstrap .so/ in this repo
  so                                 # status snapshot (content-first)
  so --help                          # full command list`,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE:          runRoot,
	}
	axi.Bind(root, &axiFlags)
	root.Version = version.Display()
	root.SetVersionTemplate("so {{.Version}}\n")
	root.AddCommand(
		cmdInstall(), cmdUninstall(), cmdInit(), cmdApplyUpgrade(), cmdSync(), cmdRefresh(), cmdDev(), cmdGraph(), cmdSessions(),
		cmdEval(), cmdRecommend(), cmdDoctor(), cmdSkill(), cmdGuard(), cmdKnowledge(), cmdHarvest(),
		cmdQuery(), coding.NewCmd(),
		cmdProjects(), cmdStatus(), cmdCheckpoint(), cmdBlame(), cmdWhy(),
		cmdGitHook(), cmdLogin(), cmdLogout(), cmdImport(),
		cmdMemory(), cmdLearn(), cmdAudit(), cmdOpen(), cmdRetrieve(),
		version.NewCmd(),
	)
	if err := root.Execute(); err != nil {
		out().WriteError(err)
		os.Exit(axi.ExitCode(err))
	}
}


func runRoot(cmd *cobra.Command, args []string) error {
	o := out()
	root := repoRoot()
	paths := harness.Resolve(root)
	if !paths.Exists() {
		o.Next("so init", "so install")
		o.Empty("harness")
		return nil
	}
	st := session.NewStateStore(paths)
	active, _ := st.ListActive()
	backend := session.NewLocalMulti(root, paths)
	list, _ := backend.List(cmd.Context(), session.Filter{ProjectID: "all"})
	_, graphOK := os.Stat(paths.GraphJSON)
	_, guardOK := os.Stat(paths.GuardrailsFile)
	data := map[string]any{
		"repo":            root,
		"harness":         paths.Root,
		"sessions":        len(list),
		"active_sessions": len(active),
		"graph":           graphOK == nil,
		"guardrails":      guardOK == nil,
	}
	o.Next("so sessions list", `so graph query "how does X work?"`, "so doctor")
	return o.HumanOrJSON("status", func() {
		fmt.Fprintf(o.W, "harness  %s\n", paths.Root)
		fmt.Fprintf(o.W, "sessions  %d  active=%d\n", len(list), len(active))
		fmt.Fprintf(o.W, "graph  %v  guardrails  %v\n", graphOK == nil, guardOK == nil)
	}, data)
}

func repoRoot() string {
	wd, _ := os.Getwd()
	root, err := harness.FindRoot(wd)
	if err != nil {
		return wd
	}
	return root
}


func applyTracesDir(root string, paths *harness.Paths, cfg config.Config) {
	if paths == nil {
		return
	}
	paths.TracesDir = cfg.LocalTracesDir(root)
}

func cmdInstall() *cobra.Command {
	var global, project bool
	c := &cobra.Command{
		Use:   "install",
		Short: "Register /so skill with coding agents (run after installing the CLI)",
		Long: `Registers the /so skill for Claude Code, Cursor, Codex, Gemini, OpenCode,
Copilot CLI, Pi, and shared Agent Skills - same role as graphify install after
installing the graphify CLI.

Default: install globally into your home directory AND into the current git project.
This does NOT create .so/ or build a graph - run so init for that.

Also best-effort ensures the graphify CLI is on PATH (used later by so init / so graph).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !global && !project {
				global, project = true, true
			}
			opts := inject.InstallOptions{Global: global}
			if project {
				if root := repoRoot(); root != "" {
					opts.ProjectRoot = root
				}
			}
			// Force refresh so upgrades rewrite skill files
			if global && project {
				res, err := inject.EnsureSkills(true)
				if err != nil {
					return err
				}
				fmt.Println("Registered /so skill:")
				for _, p := range res.Paths {
					fmt.Println(" ", p)
				}
			} else {
				res, err := inject.InstallSkills(opts)
				if err != nil {
					return err
				}
				fmt.Println("Registered /so skill:")
				for _, p := range res.Paths {
					fmt.Println(" ", p)
				}
			}
			ensureGraphifyy()
			fmt.Println()
			fmt.Println("Reload Cursor / your agent if /so is not listed yet.")
			fmt.Println("Then try:  /so")
			fmt.Println("Bootstrap: /so init")
			return nil
		},
	}
	c.Flags().BoolVar(&global, "global", false, "Install into ~/.agents, ~/.claude, ~/.cursor, ~/.codex")
	c.Flags().BoolVar(&project, "project", false, "Install into the current repository only")
	return c
}

func cmdUninstall() *cobra.Command {
	var dryRun, keepHarness, keepBinary bool
	c := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove Superopen skills, hooks, injectors, harness, and the CLI",
		Long: `Full teardown of what so install / so init / coding hooks put on this machine.

Removes:
  - /so skills (Cursor, Claude, Codex, Agent Skills) - user-global and this repo
  - Project injectors (AGENTS.md / CLAUDE.md markers, .cursor/rules/superopen.mdc)
  - Coding-agent OTLP hooks (all vendors, purged)
  - .so/ harness in the current repo (unless --keep-harness)
  - The so binary itself (unless --keep-binary)

Use so coding uninstall if you only want to strip hooks.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			o := out()
			root := repoRoot()
			var removed []string
			o.Next("go install ./cmd/so && so install", "so init")

			if dryRun {
				_, _ = codinguninstall.RemoveAll(true, true, o.W, o.ErrW)
				fmt.Fprintf(o.W, "[dry-run] would remove skills + injectors under %s and ~\n", root)
				if !keepHarness {
					fmt.Fprintf(o.W, "[dry-run] would remove %s\n", filepath.Join(root, ".so"))
				}
				if !keepBinary {
					if exe, err := os.Executable(); err == nil {
						exe, _ = filepath.EvalSymlinks(exe)
						fmt.Fprintf(o.W, "[dry-run] would remove binary %s\n", exe)
					}
				}
				return nil
			}

			res, err := inject.Uninstall(root)
			if err != nil {
				return axi.Err(err)
			}
			removed = append(removed, res.Removed...)

			hookPaths, _ := codinguninstall.RemoveAll(true, false, o.W, o.ErrW)
			removed = append(removed, hookPaths...)

			if !keepHarness {
				soDir := filepath.Join(root, ".so")
				if info, err := os.Stat(soDir); err == nil && info.IsDir() {
					if err := os.RemoveAll(soDir); err != nil {
						return axi.Err(err)
					}
					removed = append(removed, soDir)
				}
			}

			rows := make([]map[string]any, 0, len(removed))
			for _, p := range removed {
				rows = append(rows, map[string]any{"path": p})
			}
			if len(rows) == 0 {
				o.Empty("uninstall_paths")
			} else {
				o.Rows("uninstall_paths", []string{"path"}, rows)
			}

			if !keepBinary {
				exe, err := os.Executable()
				if err == nil {
					exe, _ = filepath.EvalSymlinks(exe)
					fmt.Fprintf(o.W, "removing binary  %s\n", exe)
					_ = os.Remove(exe)
				}
			}
			return nil
		},
	}
	c.Flags().BoolVar(&dryRun, "dry-run", false, "Print plan without changing files")
	c.Flags().BoolVar(&keepHarness, "keep-harness", false, "Leave .so/ in place")
	c.Flags().BoolVar(&keepBinary, "keep-binary", false, "Leave the so executable in place")
	return c
}


func ensureGraphifyy() {
	if err := graph.EnsureTool(); err != nil {
		fmt.Printf("graphify: %v\n", err)
	}
}

func cmdInit() *cobra.Command {
	var codeOnly, force, useLLM, noLLM bool
	c := &cobra.Command{
		Use:   "init",
		Short: "Bootstrap .so/ harness, graph, o11y hooks, and injectors",
		RunE: func(cmd *cobra.Command, args []string) error {
			if useLLM && noLLM {
				return fmt.Errorf("cannot combine --llm and --no-llm")
			}
			if useLLM {
				cfg, _ := config.Load(harness.Resolve(repoRoot()).Config)
				if !cfg.HasLLM() {
					fmt.Print(config.LLMSetupGuide())
					return fmt.Errorf("LLM required (--llm) but no API key / gateway configured")
				}
			}
			rep, err := initcmd.Run(initcmd.Options{
				RepoRoot: repoRoot(),
				CodeOnly: codeOnly,
				Force:    force,
				UseLLM:   useLLM,
				NoLLM:    noLLM,
			})
			if err != nil {
				return err
			}
			fmt.Printf("Superopen initialized at %s\n", rep.Paths.Root)
			html := "missing graph.html"
			if rep.Graph.HasHTML {
				html = "UI graph.html ready"
			}
			fmt.Printf("Graph: %d nodes, %d edges (%s · %s)\n", rep.Graph.NodeCount, rep.Graph.EdgeCount, rep.Graph.Source, html)
			fmt.Printf("Agent sources: %d · derived rules: %d → guardrails/evals\n", rep.Agents, rep.Rules)
			if rep.LLM.Used {
				fmt.Printf("LLM upgrade: %d guardrails, %d checks\n", rep.LLM.Rules, rep.LLM.Checks)
				fmt.Println("Next: run `so dev`, use your coding agent")
			} else {
				fmt.Printf("Harness upgrade: deferred (%s) - use /so init in an assistant, or so apply-upgrade / so init --llm\n", rep.LLM.Reason)
			}
			fmt.Println("Vendors: coding-agent o11y hooks installed")
			return nil
		},
	}
	c.Flags().BoolVar(&codeOnly, "code-only", false, "Skip Graphify semantic/docs pass")
	c.Flags().BoolVar(&force, "force", false, "Overwrite existing docs/guardrails/evals with fresh heuristic seed")
	c.Flags().BoolVar(&useLLM, "llm", false, "Require headless API-key LLM upgrade (fails without a configured LLM)")
	c.Flags().BoolVar(&noLLM, "no-llm", false, "Skip API-key LLM upgrade (default path for /so init in assistants)")
	return c
}

func cmdApplyUpgrade() *cobra.Command {
	return &cobra.Command{
		Use:   "apply-upgrade [file|-]",
		Short: "Apply assistant-produced harness JSON into .so/ (Graphify-style)",
		Long:  "Reads upgrade JSON (from an AI assistant) and writes .so/knowledge, guardrails, and evals. Use after `so init --no-llm` when running inside Cursor/Claude.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := repoRoot()
			paths := harness.Resolve(root)
			if !paths.Exists() {
				return fmt.Errorf("run so init first")
			}
			var raw []byte
			var err error
			if len(args) == 0 || args[0] == "-" {
				raw, err = io.ReadAll(os.Stdin)
			} else {
				raw, err = os.ReadFile(args[0])
			}
			if err != nil {
				return err
			}
			data, err := os.ReadFile(filepath.Join(paths.Root, "discovery.json"))
			if err != nil {
				return fmt.Errorf("discovery.json: %w", err)
			}
			var profile discover.Profile
			if err := json.Unmarshal(data, &profile); err != nil {
				return err
			}
			if err := seed.ApplyUpgradeJSON(paths, profile, string(raw)); err != nil {
				return err
			}
			fmt.Println("Applied harness upgrade into .so/")
			return nil
		},
	}
}

func cmdSync() *cobra.Command {
	var semantic bool
	var skipGraph bool
	c := &cobra.Command{
		Use:   "sync",
		Short: "Refresh injectors, hooks, graph, and citymap after harness edits",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := sync.Run(sync.Options{RepoRoot: repoRoot(), Semantic: semantic, SkipGraph: skipGraph}); err != nil {
				return err
			}
			return out().HumanOrJSON("result", func() {
				fmt.Println("Synced.")
			}, map[string]any{"ok": true, "skip_graph": skipGraph})
		},
	}
	c.Flags().BoolVar(&semantic, "semantic", false, "Force docs/semantic graph rebuild")
	c.Flags().BoolVar(&skipGraph, "skip-graph", false, "Skip graph rebuild (still refreshes retrieve index + inject)")
	return c
}

func cmdRefresh() *cobra.Command {
	var skipGraph bool
	var force bool
	c := &cobra.Command{
		Use:   "refresh",
		Short: "Lite refresh after git pull (memory, retrieve, optional graph)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := sync.Refresh(sync.RefreshOptions{RepoRoot: repoRoot(), SkipGraph: skipGraph, Force: force}); err != nil {
				return err
			}
			return out().HumanOrJSON("result", func() {
				fmt.Println("Refreshed.")
			}, map[string]any{"ok": true, "skip_graph": skipGraph})
		},
	}
	c.Flags().BoolVar(&skipGraph, "skip-graph", false, "Skip graph rebuild")
	c.Flags().BoolVar(&force, "force", false, "Force graph rebuild even if SHA unchanged")
	return c
}

func cmdGraph() *cobra.Command {
	c := &cobra.Command{Use: "graph", Short: "Repository graph operations"}
	var codeOnly bool
	rebuild := &cobra.Command{
		Use:   "rebuild",
		Short: "Rebuild graph (JSON + graph.html for the UI)",
		RunE: func(cmd *cobra.Command, args []string) error {
			root := repoRoot()
			paths := harness.Resolve(root)
			cfg, _ := config.Load(paths.Config)
			_ = graph.EnsureTool()
			onlyCode := codeOnly || !cfg.Graph.Semantic
			res, err := graph.Build(root, paths, onlyCode, cfg.Graph.SemanticBackend)
			if err != nil {
				return err
			}
			html := "no html"
			if res.HasHTML {
				html = "graph.html"
			}
			fmt.Printf("Wrote %s (%d nodes, %d edges, %s, %s)\n", res.Path, res.NodeCount, res.EdgeCount, res.Source, html)
			return viz.BuildCitymap(root, paths)
		},
	}
	rebuild.Flags().BoolVar(&codeOnly, "code-only", false, "AST-only extract (skip semantic LLM pass)")
	c.AddCommand(rebuild)
	c.AddCommand(queryCmd())
	return c
}

func cmdQuery() *cobra.Command {
	c := queryCmd()
	c.Use = "query [question]"
	c.Short = "Query the local graph (alias for `so graph query`)"
	return c
}

func queryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "query [question]",
		Short: "Query the local graph",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o := out()
			answer, err := graph.Query(repoRoot(), strings.Join(args, " "))
			if err != nil {
				return axi.Err(err)
			}
			preview := answer
			if !o.Flags.Full {
				preview = o.Truncate(answer, 500)
			}
			payload := map[string]any{"answer": preview, "truncated": preview != answer}
			o.Next("so graph rebuild", "so retrieve ...")
			return o.HumanOrJSON("graph_query", func() {
				fmt.Fprintln(o.W, preview)
			}, payload)
		},
	}
}

func cmdSessions() *cobra.Command {
	c := &cobra.Command{Use: "sessions", Short: "List and manage sessions"}
	c.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			o := out()
			root := repoRoot()
			paths := harness.Resolve(root)
			backend := session.NewLocalMulti(root, paths)
			list, err := backend.List(cmd.Context(), session.Filter{ProjectID: "all"})
			if err != nil {
				return axi.Err(err)
			}
			rows := make([]map[string]any, 0, len(list))
			for _, s := range list {
				title := session.DisplayName(s.Meta)
				rows = append(rows, map[string]any{
					"id": s.ID, "vendor": s.Vendor, "commits": len(s.Commits), "title": title,
				})
			}
			if len(rows) == 0 {
				o.Next("so sessions demo", "so dev")
				o.Empty("sessions")
				return nil
			}
			o.Rows("sessions", []string{"id", "vendor", "commits", "title"}, rows)
			return nil
		},
	})
	attachSessionsStart(c)
	// rest of sessions cmds added below / via extendSessionsCmd
	c.AddCommand(&cobra.Command{
		Use:   "show [id]",
		Args:  cobra.ExactArgs(1),
		Short: "Show session meta",
		RunE: func(cmd *cobra.Command, args []string) error {
			ss := session.NewStore(harness.Resolve(repoRoot()))
			m, err := ss.Get(args[0])
			if err != nil {
				return err
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(m)
		},
	})
	c.AddCommand(&cobra.Command{
		Use:   "finalize [session-id]",
		Short: "Materialize traces into a session (post-session pipeline)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := ""
			if len(args) > 0 {
				id = strings.TrimSpace(args[0])
			}
			return finalizeSession(repoRoot(), id)
		},
	})
	c.AddCommand(&cobra.Command{
		Use:   "demo",
		Short: "Write a demo session from synthetic telemetry (for UI smoke)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return demoSession(repoRoot())
		},
	})
	extendSessionsCmd(c)
	c.RunE = c.Commands()[0].RunE // default to list
	return c
}

func cmdEval() *cobra.Command {
	return &cobra.Command{
		Use:   "eval [session-id]",
		Short: "Run evaluations for a session",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := repoRoot()
			paths := harness.Resolve(root)
			cfg, _ := config.Load(paths.Config)
			applyTracesDir(root, &paths, cfg)
			id := ""
			if len(args) > 0 {
				id = args[0]
			} else {
				list, _ := session.NewStore(paths).List()
				if len(list) == 0 {
					return fmt.Errorf("no sessions")
				}
				id = list[0].ID
			}
			store := tracestore.NewLocalJSONL(paths.TracesDir)
			spans, _ := store.Query(tracestore.QueryFilter{SessionID: id})
			client := llm.NewBestCompleter(cfg)
			res, err := eval.Run(paths, cfg, id, spans, client)
			if err != nil {
				return err
			}
			fmt.Printf("Session %s score=%.2f badge=%s\n", id, res.Score, res.Badge)
			if client != nil && client.Available() {
				fmt.Printf("Model backend: %s\n", client.Backend())
			}
			if cfg.Recommendations.Auto {
				recs, _ := recommend.Generate(paths, id, res, client)
				fmt.Printf("Generated %d recommendations\n", len(recs))
			}
			return nil
		},
	}
}

func cmdRecommend() *cobra.Command {
	c := &cobra.Command{Use: "recommend", Short: "Manage recommendations"}
	c.AddCommand(&cobra.Command{
		Use: "list",
		RunE: func(cmd *cobra.Command, args []string) error {
			recs, err := recommend.LoadPending(harness.Resolve(repoRoot()))
			if err != nil {
				return err
			}
			return out().HumanOrJSON("recommendations", func() {
				for _, r := range recs {
					fmt.Printf("%s  [%s] %s\n", r.ID, r.Type, r.Title)
				}
			}, recs)
		},
	})
	c.AddCommand(&cobra.Command{
		Use:  "apply [id]",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := harness.Resolve(repoRoot())
			if err := recommend.Apply(paths, args[0]); err != nil {
				return err
			}
			if err := sync.Run(sync.Options{RepoRoot: repoRoot(), SkipGraph: true}); err != nil {
				return err
			}
			return out().HumanOrJSON("result", func() {
				fmt.Println("Applied.")
			}, map[string]any{"ok": true, "id": args[0], "status": "applied"})
		},
	})
	c.AddCommand(&cobra.Command{
		Use:  "dismiss [id]",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := recommend.Dismiss(harness.Resolve(repoRoot()), args[0]); err != nil {
				return err
			}
			return out().HumanOrJSON("result", func() {
				fmt.Println("Dismissed.")
			}, map[string]any{"ok": true, "id": args[0], "status": "dismissed"})
		},
	})
	c.AddCommand(&cobra.Command{
		Use:  "revert [id]",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := harness.Resolve(repoRoot())
			if err := recommend.Revert(paths, args[0]); err != nil {
				return err
			}
			_ = sync.Run(sync.Options{RepoRoot: repoRoot(), SkipGraph: true})
			return out().HumanOrJSON("result", func() {
				fmt.Println("Reverted.")
			}, map[string]any{"ok": true, "id": args[0], "status": "reverted"})
		},
	})
	c.RunE = c.Commands()[0].RunE
	return c
}

func cmdDoctor() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check harness, hooks, injectors, OTLP, graph, LLM",
		RunE: func(cmd *cobra.Command, args []string) error {
			o := out()
			checks := doctor.Run(repoRoot())
			rows := make([]map[string]any, 0, len(checks))
			failed := false
			for _, c := range checks {
				mark := "ok"
				if c.Warn {
					mark = "warn"
				} else if !c.OK {
					mark = "fail"
					failed = true
				}
				rows = append(rows, map[string]any{"mark": mark, "name": c.Name, "detail": c.Detail})
			}
			o.Rows("checks", []string{"mark", "name", "detail"}, rows)
			if failed {
				return axi.Fail(axi.ExitFail, "doctor found failures", "so sync")
			}
			return nil
		},
	}
}

func cmdSkill() *cobra.Command {
	c := &cobra.Command{Use: "skill", Short: "Manage skills"}
	c.AddCommand(&cobra.Command{
		Use:  "add [name]",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := harness.Resolve(repoRoot())
			path := filepath.Join(paths.SkillsDir, args[0]+".md")
			body := fmt.Sprintf("# %s\n\nDescribe the workflow steps here.\n", args[0])
			return os.WriteFile(path, []byte(body), 0o644)
		},
	})
	return c
}

func cmdGuard() *cobra.Command {
	c := &cobra.Command{
		Use:   "guard",
		Short: "Show .so/guardrails/guardrails.yaml (advisory + enforcement)",
		RunE: func(cmd *cobra.Command, args []string) error {
			o := out()
			paths := harness.Resolve(repoRoot())
			_ = guardrails.EnsureDefaults(paths)
			data, err := os.ReadFile(paths.GuardrailsFile)
			if err != nil {
				return axi.NotFound(err.Error(), "so sync")
			}
			eng, err := guardrails.Load(paths)
			if err != nil {
				return axi.Err(err)
			}
			o.Next("so guard check --command ...", "so guard show")
			if o.Flags.JSON {
				o.Object("guardrails", eng.Explain())
				return nil
			}
			body := string(data)
			if !o.Flags.Full {
				body = o.Truncate(body, 400)
			}
			return o.HumanOrJSON("guardrails", func() {
				fmt.Fprintln(o.W, body)
			}, eng.Explain())
		},
	}
	registerGuardSubcommands(c)
	return c
}

func cmdKnowledge() *cobra.Command {
	c := &cobra.Command{Use: "knowledge", Short: "Harness knowledge helpers (.so/knowledge)"}
	c.AddCommand(&cobra.Command{
		Use:   "generate",
		Short: "Seed architecture.md from the graph report when missing (non-destructive)",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := harness.Resolve(repoRoot())
			report, _ := os.ReadFile(paths.GraphReport)
			arch := filepath.Join(paths.KnowledgeDir, "architecture.md")
			existing, _ := os.ReadFile(arch)
			if len(existing) == 0 {
				return os.WriteFile(arch, append([]byte("# Architecture\n\n"), report...), 0o644)
			}
			fmt.Println("architecture.md exists - edit manually or delete to regenerate")
			return nil
		},
	})
	return c
}

func finalizeSession(root, sessionID string) error {
	paths := harness.Resolve(root)
	_, _ = projects.Register(root, paths.Root, "")
	cfg, _ := config.Load(paths.Config)
	applyTracesDir(root, &paths, cfg)
	store := tracestore.NewLocalJSONL(paths.TracesDir)
	spans, err := store.Query(tracestore.QueryFilter{Limit: 5000})
	if err != nil {
		return err
	}
	if len(spans) == 0 {
		return fmt.Errorf("no spans in TraceStore")
	}
	// group by session id
	bySession := map[string][]tracestore.Span{}
	for _, sp := range spans {
		sid := otlp.ResolveSessionID(sp.Attributes, "")
		if sid == "" {
			sid = sp.SessionID
		}
		if sid == "" {
			sid = sp.TraceID
		}
		bySession[sid] = append(bySession[sid], sp)
	}

	latestID := strings.TrimSpace(sessionID)
	if latestID == "" {
		latestID = harvest.ConsumeFinalizePending(paths)
	}
	if latestID == "" {
		var latestTime int64
		for id, ss := range bySession {
			for _, sp := range ss {
				if sp.StartTimeUnixN > latestTime {
					latestTime = sp.StartTimeUnixN
					latestID = id
				}
			}
		}
	}
	ss := bySession[latestID]
	if len(ss) == 0 {
		return fmt.Errorf("no spans for session %s", latestID)
	}
	sess := session.NewStore(paths)
	if !session.SpansHaveActivity(ss) {
		_ = sess.Delete(latestID)
		fmt.Printf("Skipped empty session %s (no turns/work)\n", latestID)
		return nil
	}
	_ = sess.Start(session.Meta{ID: latestID, Vendor: ss[0].Attributes["coding_agent.vendor"], StartedAt: time.Unix(0, ss[0].StartTimeUnixN).UTC()})
	tokens, cost, _ := store.SessionCost(latestID)
	meta, err := sess.MaterializeFromSpans(latestID, ss, tokens, cost)
	if err != nil {
		return err
	}
	session.BackfillFromGitLog(&meta, root, 50)
	meta.RepoRoot = root
	if p, err := projects.Get(root); err == nil {
		meta.ProjectID = p.ID
	}
	_ = sess.UpdateMeta(meta)
	// Snapshot edited files as a restorable checkpoint on finalize.
	_, _ = checkpoint.NewStore(paths).CreateFromFootprint(latestID, root, "finalize")
	_ = session.NewStateStore(paths).End(latestID)
	if _, err := viz.BuildReplayFromSpans(paths, latestID, ss); err != nil {
		return err
	}
	client := llm.NewBestCompleter(cfg)
	if cfg.Evals.OnSessionEnd || cfg.Evals.Auto {
		ev, err := eval.Run(paths, cfg, latestID, ss, client)
		if err != nil {
			return err
		}
		if cfg.Recommendations.Auto {
			recs, _ := recommend.Generate(paths, latestID, ev, client)
			appliedAny := false
			for _, r := range recs {
				tier := harnessvalid.Tier(r.Type)
				if !cfg.AllowsAutoApplyTier(tier) {
					continue
				}
				if err := recommend.Apply(paths, r.ID); err == nil {
					appliedAny = true
				}
			}
			if appliedAny {
				_ = sync.Run(sync.Options{RepoRoot: root, SkipGraph: true})
			}
		}
	}
	mineOnFinalize(paths, latestID, cfg)
	_, _ = harvest.Run(paths, cfg, latestID, harvest.TriggerFinalize)
	fmt.Printf("Finalized session %s (%s)\n", meta.ID, meta.EvalBadge)
	return nil
}

// finalizeLatestSession kept as alias for callers/tests.
func finalizeLatestSession(root string) error {
	return finalizeSession(root, "")
}

func demoSession(root string) error {
	paths := harness.Resolve(root)
	if !paths.Exists() {
		return fmt.Errorf("run so init first")
	}
	cfg, _ := config.Load(paths.Config)
	applyTracesDir(root, &paths, cfg)
	id := fmt.Sprintf("ses_demo_%d", time.Now().Unix())
	store := tracestore.NewLocalJSONL(paths.TracesDir)
	now := time.Now().UnixNano()
	spans := []tracestore.Span{
		{TraceID: id, SpanID: "1", Name: "coding_agent.prompt", StartTimeUnixN: now, EndTimeUnixN: now + 1e6, SessionID: id, Attributes: map[string]string{"coding_agent.vendor": "claude-code", "gen_ai.prompt": "Add health check endpoint", "gen_ai.request.model": "claude-sonnet", "gen_ai.usage.total_tokens": "1200"}},
		{TraceID: id, SpanID: "2", Name: "coding_agent.search", StartTimeUnixN: now + 2e6, SessionID: id, Attributes: map[string]string{"coding_agent.vendor": "claude-code", "coding_agent.file_path": "cmd/so/main.go"}},
		{TraceID: id, SpanID: "3", Name: "coding_agent.read", StartTimeUnixN: now + 3e6, SessionID: id, Attributes: map[string]string{"coding_agent.vendor": "claude-code", "coding_agent.file_path": "internal/otlp/receiver.go"}},
		{TraceID: id, SpanID: "4", Name: "coding_agent.edit", StartTimeUnixN: now + 4e6, SessionID: id, Attributes: map[string]string{"coding_agent.vendor": "claude-code", "coding_agent.file_path": "internal/otlp/receiver.go"}},
		{TraceID: id, SpanID: "5", Name: "coding_agent.grep", StartTimeUnixN: now + 5e6, SessionID: id, Attributes: map[string]string{"coding_agent.vendor": "claude-code", "coding_agent.file_path": "README.md"}},
	}
	if err := store.Write(spans); err != nil {
		return err
	}
	sess := session.NewStore(paths)
	_ = sess.Start(session.Meta{ID: id, Vendor: "claude-code", Model: "claude-sonnet", PromptPreview: "Add health check endpoint", StartedAt: time.Unix(0, now).UTC()})
	meta, err := sess.MaterializeFromSpans(id, spans, 1200, 0.02)
	if err != nil {
		return err
	}
	if _, err := viz.BuildReplayFromSpans(paths, id, spans); err != nil {
		return err
	}
	client := llm.NewBestCompleter(cfg)
	ev, _ := eval.Run(paths, cfg, id, spans, client)
	_, _ = recommend.Generate(paths, id, ev, client)
	fmt.Printf("Demo session %s created (badge=%s)\n", meta.ID, meta.EvalBadge)
	return nil
}

func findWebDir(repoRoot string) string {
	candidates := []string{
		filepath.Join(repoRoot, "superopen", "web"),
		filepath.Join(repoRoot, "web"),
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append([]string{
			filepath.Join(wd, "superopen", "web"),
			filepath.Join(wd, "web"),
		}, candidates...)
	}
	for _, p := range candidates {
		if _, err := os.Stat(filepath.Join(p, "package.json")); err == nil {
			return p
		}
	}
	return ""
}

func startNextUI(repoRoot string, uiPort int) (*exec.Cmd, string, error) {
	webDir := findWebDir(repoRoot)
	if webDir == "" {
		return nil, "", fmt.Errorf("superopen/web not found")
	}
	if _, err := exec.LookPath("npm"); err != nil {
		return nil, "", fmt.Errorf("npm not on PATH")
	}
	if _, err := os.Stat(filepath.Join(webDir, "node_modules")); err != nil {
		fmt.Println("Installing web UI dependencies (npm install --ignore-scripts)…")
		install := exec.Command("npm", "install", "--ignore-scripts")
		install.Dir = webDir
		install.Stdout = os.Stdout
		install.Stderr = os.Stderr
		if err := install.Run(); err != nil {
			return nil, "", fmt.Errorf("npm install: %w", err)
		}
	}

	url := fmt.Sprintf("http://127.0.0.1:%d", uiPort)
	// Light local path: Next.js Turbopack dev via `npm run dev` (package.json).
	cmd := exec.Command("npm", "run", "dev", "--", "-p", fmt.Sprintf("%d", uiPort), "-H", "127.0.0.1")
	cmd.Dir = webDir
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("SUPEROPEN_ROOT=%s", repoRoot),
		"NODE_ENV=development",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, "", err
	}

	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url + "/sessions")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < 500 {
				return cmd, url, nil
			}
		}
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			return nil, "", fmt.Errorf("next.js exited early")
		}
		time.Sleep(400 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	return nil, "", fmt.Errorf("timed out waiting for Next.js on %s", url)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
