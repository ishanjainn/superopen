package main

import (
	"bytes"
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

	"github.com/ishanjainn/superopen/internal/axi"
	"github.com/ishanjainn/superopen/internal/checkpoint"
	"github.com/ishanjainn/superopen/internal/coding"
	codinguninstall "github.com/ishanjainn/superopen/internal/coding/uninstall"
	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/discover"
	"github.com/ishanjainn/superopen/internal/doctor"
	"github.com/ishanjainn/superopen/internal/eval"
	"github.com/ishanjainn/superopen/internal/execx"
	"github.com/ishanjainn/superopen/internal/gitruntime"
	"github.com/ishanjainn/superopen/internal/graph"
	"github.com/ishanjainn/superopen/internal/guardrails"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/harnessvalid"
	"github.com/ishanjainn/superopen/internal/harvest"
	initcmd "github.com/ishanjainn/superopen/internal/initcmd"
	"github.com/ishanjainn/superopen/internal/inject"
	"github.com/ishanjainn/superopen/internal/llm"
	"github.com/ishanjainn/superopen/internal/nativedocs"
	"github.com/ishanjainn/superopen/internal/otlp"
	"github.com/ishanjainn/superopen/internal/projects"
	"github.com/ishanjainn/superopen/internal/recommend"
	"github.com/ishanjainn/superopen/internal/seed"
	"github.com/ishanjainn/superopen/internal/session"
	"github.com/ishanjainn/superopen/internal/sync"
	"github.com/ishanjainn/superopen/internal/tracestore"
	"github.com/ishanjainn/superopen/internal/version"
	"github.com/ishanjainn/superopen/internal/viz"
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
		cmdGitHook(), cmdLogin(), cmdLogout(),
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
		Short: "Apply assistant-produced harness JSON to AGENTS.md, guardrails, and evals",
		Long:  "Reads upgrade JSON (from an AI assistant) and writes AGENTS.md plus .so/guardrails and .so/evals. Pass a file path OR stdin — never both (a path with a heredoc would ignore the heredoc). Use after `so init --no-llm` when running inside Cursor/Claude.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := repoRoot()
			paths := harness.Resolve(root)
			if !paths.Exists() {
				return fmt.Errorf("run so init first")
			}
			useFile := len(args) == 1 && args[0] != "-"
			if useFile && stdinRedirected() {
				return fmt.Errorf("apply-upgrade: pass a file OR stdin/heredoc, not both (got %q with redirected stdin)", args[0])
			}
			var raw []byte
			var err error
			if !useFile {
				raw, err = io.ReadAll(os.Stdin)
			} else {
				raw, err = os.ReadFile(args[0])
			}
			if err != nil {
				return err
			}
			if len(bytes.TrimSpace(raw)) == 0 {
				return fmt.Errorf("apply-upgrade: empty JSON (write a file and pass its path, or pipe/heredoc JSON on stdin)")
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
			fmt.Println("Applied harness upgrade → AGENTS.md, .so/guardrails/, .so/evals/")
			return nil
		},
	}
}

func stdinRedirected() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice == 0
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
	listCmd := &cobra.Command{
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
	}
	c.AddCommand(listCmd)
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
	finalizeCmd := &cobra.Command{
		Use:   "finalize [session-id]",
		Short: "Materialize traces into a session (post-session pipeline)",
		Long: `Runs eval → recommendations → optional auto-apply → harvest for a session.

Prefer agent SessionEnd / the coding hook to invoke this. Pass --detach so the
caller (agent hooks) returns immediately while work continues in the background.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := ""
			if len(args) > 0 {
				id = strings.TrimSpace(args[0])
			}
			detach, _ := cmd.Flags().GetBool("detach")
			if detach {
				root := repoRoot()
				if id == "" {
					execx.SpawnSO(root, "sessions", "finalize")
				} else {
					execx.SpawnSO(root, "sessions", "finalize", id)
				}
				return nil
			}
			// Fail-soft for agent companions: never fail the hook hard.
			if err := finalizeSession(repoRoot(), id); err != nil {
				fmt.Fprintf(os.Stderr, "finalize: %v\n", err)
			}
			return nil
		},
	}
	finalizeCmd.Flags().Bool("detach", false, "Return immediately; run finalize in a background process")
	c.AddCommand(finalizeCmd)
	c.AddCommand(&cobra.Command{
		Use:   "refresh [session-id]",
		Short: "Materialize current traces while keeping the session active",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := ""
			if len(args) > 0 {
				id = strings.TrimSpace(args[0])
			}
			return refreshSession(repoRoot(), id)
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
	c.RunE = listCmd.RunE // default to list; Commands() is alphabetically sorted.
	return c
}

func cmdEval() *cobra.Command {
	var force bool
	c := &cobra.Command{
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
			meta, _ := session.NewStore(paths).Get(id)
			scope := "snapshot"
			if meta.Status == session.StatusEnded {
				scope = "complete"
			}
			if decision := eval.DecideSkip(paths, cfg, meta, force); decision.Skip {
				if decision.Scope != "" {
					scope = decision.Scope
				}
				return out().HumanOrJSON("evaluation", func() {
					fmt.Println(eval.SkipMessage(id, decision))
				}, map[string]any{
					"result": decision.Prior, "reused": true, "scope": scope,
					"skip_reason": decision.Reason,
				})
			}
			store := tracestore.NewLocalJSONL(paths.TracesDir)
			spans, _ := store.Query(tracestore.QueryFilter{SessionID: id})
			client := llm.NewBestCompleter(cfg)
			res, err := eval.Run(paths, cfg, id, spans, client)
			if err != nil {
				return err
			}
			backend := "heuristics"
			if client != nil && client.Available() {
				backend = client.Backend()
			}
			generated := 0
			if cfg.Recommendations.Auto {
				recs, _ := recommend.Generate(paths, id, res, client)
				generated = len(recs)
			}
			return out().HumanOrJSON("evaluation", func() {
				fmt.Printf("Session %s score=%.2f badge=%s\n", id, res.Score, res.Badge)
				fmt.Printf("Model backend: %s\n", backend)
				fmt.Printf("Generated %d recommendations\n", generated)
			}, map[string]any{
				"result": res, "backend": backend, "recommendations_generated": generated,
				"reused": false, "scope": scope,
			})
		},
	}
	c.Flags().BoolVar(&force, "force", false, "Re-run even when a closed chat already has a final evaluation, or an open chat is still inside the active cooldown")
	return c
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
	var applyReason, applyActor string
	applyCmd := &cobra.Command{
		Use:  "apply [id]",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := harness.Resolve(repoRoot())
			if err := recommend.Apply(paths, args[0], recommend.Decision{Reason: applyReason, Actor: applyActor}); err != nil {
				return err
			}
			if err := sync.Run(sync.Options{RepoRoot: repoRoot(), SkipGraph: true}); err != nil {
				return err
			}
			return out().HumanOrJSON("result", func() {
				fmt.Println("Applied.")
			}, map[string]any{"ok": true, "id": args[0], "status": "applied", "decision_reason": strings.TrimSpace(applyReason), "decision_actor": strings.ToLower(strings.TrimSpace(applyActor))})
		},
	}
	applyCmd.Flags().StringVar(&applyReason, "reason", "", "why this recommendation resolves the issue")
	applyCmd.Flags().StringVar(&applyActor, "actor", "agent", "decision actor: human, agent, or system")
	_ = applyCmd.MarkFlagRequired("reason")
	c.AddCommand(applyCmd)

	var dismissReason, dismissActor string
	dismissCmd := &cobra.Command{
		Use:  "dismiss [id]",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := recommend.Dismiss(harness.Resolve(repoRoot()), args[0], recommend.Decision{Reason: dismissReason, Actor: dismissActor}); err != nil {
				return err
			}
			return out().HumanOrJSON("result", func() {
				fmt.Println("Dismissed.")
			}, map[string]any{"ok": true, "id": args[0], "status": "dismissed", "decision_reason": strings.TrimSpace(dismissReason), "decision_actor": strings.ToLower(strings.TrimSpace(dismissActor))})
		},
	}
	dismissCmd.Flags().StringVar(&dismissReason, "reason", "", "why this recommendation is being dismissed")
	dismissCmd.Flags().StringVar(&dismissActor, "actor", "agent", "decision actor: human, agent, or system")
	_ = dismissCmd.MarkFlagRequired("reason")
	c.AddCommand(dismissCmd)

	var revertReason, revertActor string
	revertCmd := &cobra.Command{
		Use:  "revert [id]",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := harness.Resolve(repoRoot())
			if err := recommend.Revert(paths, args[0], recommend.Decision{Reason: revertReason, Actor: revertActor}); err != nil {
				return err
			}
			_ = sync.Run(sync.Options{RepoRoot: repoRoot(), SkipGraph: true})
			return out().HumanOrJSON("result", func() {
				fmt.Println("Reverted.")
			}, map[string]any{"ok": true, "id": args[0], "status": "reverted", "decision_reason": strings.TrimSpace(revertReason), "decision_actor": strings.ToLower(strings.TrimSpace(revertActor))})
		},
	}
	revertCmd.Flags().StringVar(&revertReason, "reason", "", "why the applied recommendation is being reverted")
	revertCmd.Flags().StringVar(&revertActor, "actor", "agent", "decision actor: human, agent, or system")
	_ = revertCmd.MarkFlagRequired("reason")
	c.AddCommand(revertCmd)
	c.RunE = c.Commands()[0].RunE
	return c
}

func cmdDoctor() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check harness, hooks, injectors, graph, and local health",
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
			body := fmt.Sprintf("# %s\n\nDescribe the workflow steps here.\n", args[0])
			if err := os.MkdirAll(filepath.Join(paths.SkillsDir, args[0]), 0o755); err != nil {
				return err
			}
			return os.WriteFile(paths.SkillSKILL(args[0]), []byte(body), 0o644)
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
	c := &cobra.Command{Use: "knowledge", Short: "Show native knowledge roots (AGENTS.md)"}
	c.AddCommand(&cobra.Command{
		Use:   "generate",
		Short: "Append graph report into AGENTS.md learned section when useful",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := harness.Resolve(repoRoot())
			report, err := os.ReadFile(paths.GraphReport)
			if err != nil || len(report) == 0 {
				return fmt.Errorf("no graph report - run so graph first")
			}
			return nativedocs.AppendLearned(paths, "## Graph snapshot\n\n"+string(report))
		},
	})
	c.RunE = func(cmd *cobra.Command, args []string) error {
		paths := harness.Resolve(repoRoot())
		roots := nativedocs.DiscoverRoots(paths.RepoRoot)
		fmt.Println("AGENTS.md:", paths.AgentsMD)
		for _, p := range paths.AgentsPaths() {
			if p != paths.AgentsMD {
				fmt.Println("AGENTS.md (nested):", p)
			}
		}
		fmt.Printf("Rules (%s): %s\n", roots.RulesKind, paths.RulesDir)
		fmt.Printf("Skills (%s): %s\n", roots.SkillsKind, paths.SkillsDir)
		return nil
	}
	return c
}

type finalizeOpts struct {
	// SkipTrackedMutations keeps the working tree clean after git hooks:
	// generate evals/recs and memory only — no auto-apply, inject sync, or
	// harvest writes into AGENTS.md / rules / skills.
	SkipTrackedMutations bool
}

func finalizeSession(root, sessionID string, opts ...finalizeOpts) error {
	var o finalizeOpts
	if len(opts) > 0 {
		o = opts[0]
	}
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
	if existing, existingErr := sess.Get(latestID); existingErr == nil && existing.Status == session.StatusEnded && existing.EndedAt != nil {
		latestSpan := time.Time{}
		for _, sp := range ss {
			at := time.Unix(0, sp.EndTimeUnixN).UTC()
			if sp.EndTimeUnixN == 0 {
				at = time.Unix(0, sp.StartTimeUnixN).UTC()
			}
			if at.After(latestSpan) {
				latestSpan = at
			}
		}
		if !latestSpan.After(*existing.EndedAt) {
			fmt.Printf("Session %s is already finalized; no new evaluation was created\n", latestID)
			return nil
		}
	}
	if !session.SpansHaveActivity(ss) {
		_ = sess.Delete(latestID)
		fmt.Printf("Skipped empty session %s (no turns/work)\n", latestID)
		return nil
	}
	_ = sess.Start(session.Meta{ID: latestID, Vendor: session.VendorFromSpans(ss), StartedAt: time.Unix(0, ss[0].StartTimeUnixN).UTC()})
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
			if !o.SkipTrackedMutations {
				appliedAny := false
				for _, r := range recs {
					tier := harnessvalid.Tier(r.Type)
					if !cfg.AllowsAutoApplyTier(tier) {
						continue
					}
					if err := recommend.Apply(paths, r.ID, recommend.Decision{
						Reason: "Automatically applied because its recommendation tier is enabled in auto_apply_tiers.",
						Actor:  "system",
					}); err == nil {
						appliedAny = true
					}
				}
				if appliedAny {
					_ = sync.Run(sync.Options{RepoRoot: root, SkipGraph: true, SkipInject: true})
				}
			}
		}
	}
	mineOnFinalize(paths, latestID, cfg)
	_, _ = harvest.Run(paths, cfg, latestID, harvest.TriggerFinalize, harvest.RunOpts{
		SkipNativeDocs: o.SkipTrackedMutations,
	})
	_, _ = gitruntime.SnapshotSessionDir(root, paths.SessionDir(latestID), latestID)
	fmt.Printf("Finalized session %s (%s)\n", meta.ID, meta.EvalBadge)
	return nil
}

// refreshSession materializes the latest trace snapshot without declaring the
// chat complete. Codex Stop means "assistant turn ended", not "chat closed",
// so its hook uses this path and reserves finalizeSession for an explicit close.
func refreshSession(root, sessionID string) error {
	paths := harness.Resolve(root)
	_, _ = projects.Register(root, paths.Root, "")
	cfg, _ := config.Load(paths.Config)
	applyTracesDir(root, &paths, cfg)
	store := tracestore.NewLocalJSONL(paths.TracesDir)
	spans, err := store.Query(tracestore.QueryFilter{Limit: 5000})
	if err != nil {
		return err
	}
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
	id := strings.TrimSpace(sessionID)
	if id == "" {
		var latest int64
		for candidate, candidateSpans := range bySession {
			for _, sp := range candidateSpans {
				if sp.StartTimeUnixN > latest {
					latest = sp.StartTimeUnixN
					id = candidate
				}
			}
		}
	}
	ss := bySession[id]
	if len(ss) == 0 {
		return fmt.Errorf("no spans for session %s", id)
	}
	if !session.SpansHaveActivity(ss) {
		return nil
	}
	sess := session.NewStore(paths)
	_ = sess.Start(session.Meta{ID: id, Vendor: session.VendorFromSpans(ss), StartedAt: time.Unix(0, ss[0].StartTimeUnixN).UTC()})
	tokens, cost, _ := store.SessionCost(id)
	meta, err := sess.MaterializeFromSpans(id, ss, tokens, cost)
	if err != nil {
		return err
	}
	meta.Status = session.StatusActive
	meta.EndedAt = nil
	meta.DurationMs = time.Since(meta.StartedAt).Milliseconds()
	meta.RepoRoot = root
	if p, projectErr := projects.Get(root); projectErr == nil {
		meta.ProjectID = p.ID
	}
	if err := sess.UpdateMeta(meta); err != nil {
		return err
	}
	_ = session.NewStateStore(paths).Save(session.State{
		SessionID: id,
		Vendor:    meta.Vendor,
		Phase:     session.PhaseActive,
		RepoRoot:  root,
	})
	_, _ = viz.BuildReplayFromSpans(paths, id, ss)
	fmt.Printf("Refreshed active session %s\n", id)
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
	soBin, _ := os.Executable()
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("SUPEROPEN_ROOT=%s", repoRoot),
		fmt.Sprintf("SUPEROPEN_SO_BIN=%s", soBin),
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
