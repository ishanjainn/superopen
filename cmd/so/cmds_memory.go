package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ishanjainn/superopen/internal/audit"
	"github.com/ishanjainn/superopen/internal/axi"
	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/guardrails"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/learn"
	"github.com/ishanjainn/superopen/internal/llm"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/retrieve"
)

func cmdMemory() *cobra.Command {
	c := &cobra.Command{Use: "memory", Short: "Workspace memory (prefs, lessons, search, active-context)"}
	show := &cobra.Command{
		Use:   "show",
		Short: "Show memory summary",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := harness.Resolve(repoRoot())
			s := memory.NewStore(paths)
			_ = s.Ensure()
			lessons, _ := s.ListLessons()
			sem, _ := s.ListSemantic()
			eps, _ := s.ListEpisodic()
			data := map[string]any{
				"lessons": len(lessons), "semantic": len(sem), "episodic": len(eps),
				"active": s.ActivePath(), "dir": paths.MemoryDir,
			}
			return out().HumanOrJSON("result", func() {
				fmt.Printf("memory dir: %s\nlessons: %d  semantic: %d  episodic: %d\nactive: %s\n",
					paths.MemoryDir, len(lessons), len(sem), len(eps), s.ActivePath())
			}, data)
		},
	}
	search := &cobra.Command{
		Use:   "search [query]",
		Short: "Search memory + harness corpus (hybrid keyword)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := harness.Resolve(repoRoot())
			s := memory.NewStore(paths)
			hits, err := s.HybridSearch(strings.Join(args, " "), 20)
			if err != nil {
				return axi.Err(err)
			}
			return out().HumanOrJSON("result", func() {
				for _, h := range hits {
					fmt.Printf("[%s] %s  %s\n", h.Kind, h.ID, h.Snippet)
				}
			}, hits)
		},
	}
	add := &cobra.Command{
		Use:   "add [text...]",
		Short: "Add a lesson",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := harness.Resolve(repoRoot())
			s := memory.NewStore(paths)
			text := strings.Join(args, " ")
			l := memory.Lesson{Text: text, Scope: "workspace", Confidence: 1}
			if err := s.AddLesson(l, memory.ModePersistent); err != nil {
				return axi.Err(err)
			}
			list, _ := s.ListLessons()
			var payload any = map[string]any{"text": text, "count": len(list)}
			if len(list) > 0 {
				payload = list[len(list)-1]
			}
			return out().HumanOrJSON("result", func() { fmt.Println("added lesson") }, payload)
		},
	}

	rm := &cobra.Command{
		Use:   "rm [lesson-id]",
		Short: "Delete a lesson by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := harness.Resolve(repoRoot())
			s := memory.NewStore(paths)
			if err := s.DeleteLesson(args[0]); err != nil {
				return axi.Err(err)
			}
			return out().HumanOrJSON("result", func() { fmt.Println("deleted", args[0]) }, map[string]any{"id": args[0], "ok": true})
		},
	}

	update := &cobra.Command{
		Use:   "update [lesson-id] [text...]",
		Short: "Update a lesson by id",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := harness.Resolve(repoRoot())
			s := memory.NewStore(paths)
			id := args[0]
			text := strings.Join(args[1:], " ")
			if err := s.UpdateLesson(id, text); err != nil {
				return axi.Err(err)
			}
			list, _ := s.ListLessons()
			var lesson any
			for _, l := range list {
				if l.ID == id {
					lesson = l
					break
				}
			}
			return out().HumanOrJSON("result", func() { fmt.Println("updated", id) }, map[string]any{"ok": true, "lesson": lesson})
		},
	}

	ctxCmd := &cobra.Command{
		Use:     "active-context",
		Aliases: []string{"context"},
		Short:   "Preview SessionStart Active Context pack",
		RunE: func(cmd *cobra.Command, args []string) error {
			q, _ := cmd.Flags().GetString("query")
			modeStr, _ := cmd.Flags().GetString("mode")
			mode := memory.Mode(modeStr)
			if mode == "" {
				mode = memory.ModePersistent
			}
			paths := harness.Resolve(repoRoot())
			cfg, _ := config.Load(paths.Config)
			if !cfg.MemoryEnabled() {
				return axi.Err(fmt.Errorf("memory.enabled is false"))
			}
			s := memory.NewStore(paths)
			pack, err := s.BuildSessionContext(12000, q, mode)
			if err != nil {
				return axi.Err(err)
			}
			return out().HumanOrJSON("result", func() { fmt.Print(pack.Text) }, pack)
		},
	}
	ctxCmd.Flags().String("query", "", "Optional retrieval query for episodic/semantic")
	ctxCmd.Flags().String("mode", "persistent", "persistent|incognito|temporary")

	refresh := &cobra.Command{
		Use:   "refresh",
		Short: "Rebuild context.md inject pack",
		RunE: func(cmd *cobra.Command, args []string) error {
			q, _ := cmd.Flags().GetString("query")
			paths := harness.Resolve(repoRoot())
			pack, err := memory.NewStore(paths).RefreshActive(q)
			if err != nil {
				return axi.Err(err)
			}
			return out().HumanOrJSON("result", func() {
				fmt.Printf("refreshed %s (%d chars)\n", pack.ActivePath, pack.CharCount)
			}, pack)
		},
	}
	refresh.Flags().String("query", "", "Optional query bias for episodic/semantic")

	cons := &cobra.Command{
		Use:   "consolidate [summary...]",
		Short: "Consolidate session summary into prefs/projects/history",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := harness.Resolve(repoRoot())
			cfg, _ := config.Load(paths.Config)
			s := memory.NewStore(paths)
			summary := strings.Join(args, " ")
			if summary == "" {
				summary = "manual consolidate"
			}
			hint, err := s.Consolidate(summary, llm.NewMemoryCompleter(cfg))
			if err != nil {
				return axi.Err(err)
			}
			return out().HumanOrJSON("result", func() {
				fmt.Println("consolidated")
				if hint != "" {
					fmt.Println(hint)
				}
			}, map[string]any{"ok": true, "hint": hint})
		},
	}

	c.AddCommand(show, search, add, rm, update, ctxCmd, refresh, cons)
	return c
}

func cmdLearn() *cobra.Command {
	c := &cobra.Command{Use: "learn", Short: "Capture corrections as lessons"}
	c.AddCommand(&cobra.Command{
		Use:   "add [text...]",
		Short: "Add an explicit lesson",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := harness.Resolve(repoRoot())
			s := memory.NewStore(paths)
			text := strings.Join(args, " ")
			l := memory.Lesson{Text: text, Scope: "workspace", Confidence: 1}
			if err := s.AddLesson(l, memory.ModePersistent); err != nil {
				return axi.Err(err)
			}
			list, _ := s.ListLessons()
			var payload any = map[string]any{"text": text, "count": len(list)}
			if len(list) > 0 {
				payload = list[len(list)-1]
			}
			return out().HumanOrJSON("result", func() { fmt.Println("learned:", text) }, payload)
		},
	})
	c.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List lessons",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := harness.Resolve(repoRoot())
			s := memory.NewStore(paths)
			list, err := s.ListLessons()
			if err != nil {
				return axi.Err(err)
			}
			return out().HumanOrJSON("result", func() {
				for _, l := range list {
					fmt.Printf("%s  %s\n", l.ID, l.Text)
				}
			}, list)
		},
	})
	return c
}

func cmdAudit() *cobra.Command {
	c := &cobra.Command{Use: "audit", Short: "SEL-style audit trail"}
	c.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List recent audit events",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := harness.Resolve(repoRoot())
			evs, err := audit.List(paths, 100)
			if err != nil {
				return axi.Err(err)
			}
			return out().HumanOrJSON("result", func() {
				for _, e := range evs {
					fmt.Printf("%s  %-28s  %-10s  %s\n", e.At.Format(time.RFC3339), e.Action, e.Type, e.Detail)
				}
			}, evs)
		},
	})
	c.AddCommand(&cobra.Command{
		Use:   "verify",
		Short: "Verify audit log is readable",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := harness.Resolve(repoRoot())
			evs, err := audit.List(paths, 1)
			if err != nil {
				return axi.Err(err)
			}
			return out().HumanOrJSON("result", func() {
				fmt.Printf("ok (%d recent)\n", len(evs))
			}, map[string]any{"ok": true, "count": len(evs)})
		},
	})
	return c
}

func registerGuardSubcommands(c *cobra.Command) {
	c.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show effective guardrails policy",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := harness.Resolve(repoRoot())
			_ = guardrails.EnsureDefaults(paths)
			eng, err := guardrails.Load(paths)
			if err != nil {
				return axi.Err(err)
			}
			data := eng.Explain()
			return out().HumanOrJSON("result", func() {
				raw, _ := json.MarshalIndent(data, "", "  ")
				fmt.Println(string(raw))
			}, data)
		},
	})
	check := &cobra.Command{
		Use:   "check",
		Short: "Check a command or path against policy",
		RunE: func(cmd *cobra.Command, args []string) error {
			command, _ := cmd.Flags().GetString("command")
			path, _ := cmd.Flags().GetString("path")
			paths := harness.Resolve(repoRoot())
			eng, _ := guardrails.Load(paths)
			var dec guardrails.Decision
			switch {
			case command != "":
				dec = eng.CheckCommand(command)
			case path != "":
				dec = eng.CheckPath(path)
			default:
				return fmt.Errorf("pass --command or --path")
			}
			return out().HumanOrJSON("result", func() {
				fmt.Println(guardrails.FormatDecision(dec))
			}, dec)
		},
	}
	check.Flags().String("command", "", "Shell command to check")
	check.Flags().String("path", "", "Filesystem path to check")
	c.AddCommand(check)
	deny := &cobra.Command{Use: "deny", Short: "Denied command helpers"}
	deny.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List denied command patterns",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := harness.Resolve(repoRoot())
			eng, _ := guardrails.Load(paths)
			data := eng.Explain()["denied_commands"]
			return out().HumanOrJSON("result", func() {
				for _, p := range data.([]string) {
					fmt.Println(p)
				}
			}, data)
		},
	})
	c.AddCommand(deny)
	c.AddCommand(&cobra.Command{
		Use:   "explain",
		Short: "Explain effective guardrails",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := harness.Resolve(repoRoot())
			eng, _ := guardrails.Load(paths)
			data := eng.Explain()
			return out().HumanOrJSON("result", func() {
				raw, _ := json.MarshalIndent(data, "", "  ")
				fmt.Println(string(raw))
			}, data)
		},
	})
}

func cmdOpen() *cobra.Command {
	return &cobra.Command{
		Use:   "open [path]",
		Short: "Open Superopen UI deep-link when so dev is running",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			port := 4444
			path := "/sessions"
			if len(args) > 0 {
				p := args[0]
				if !strings.HasPrefix(p, "/") {
					p = "/" + p
				}
				path = p
			}
			url := fmt.Sprintf("http://127.0.0.1:%d%s", port, path)
			client := &http.Client{Timeout: 800 * time.Millisecond}
			if resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/meta", port)); err != nil {
				return fmt.Errorf("UI not reachable at :%d - run `so dev` first (%w)", port, err)
			} else {
				resp.Body.Close()
			}
			return openBrowser(url)
		},
	}
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		// Avoid `start` eating the first quoted arg; rundll32 is reliable.
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		fmt.Println(url)
		return nil
	}
	if err := cmd.Start(); err != nil {
		fmt.Println(url)
		return err
	}
	return nil
}

func cmdRetrieve() *cobra.Command {
	c := &cobra.Command{
		Use:   "retrieve [query]",
		Short: "Search harness corpus index",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := repoRoot()
			paths := harness.Resolve(root)
			hits, err := retrieve.Search(paths, strings.Join(args, " "), 20)
			if err != nil || len(hits) == 0 {
				_, _ = retrieve.Rebuild(root, paths)
				hits, err = retrieve.Search(paths, strings.Join(args, " "), 20)
			}
			if err != nil {
				return axi.Err(err)
			}
			return out().HumanOrJSON("result", func() {
				for _, h := range hits {
					fmt.Printf("[%s] %s\n  %s\n", h.Kind, h.Path, h.Snippet)
				}
			}, hits)
		},
	}
	return c
}

// attachSessionsStart adds `so sessions start` and enriches resume.
func attachSessionsStart(sessions *cobra.Command) {
	var vendor, query, mode string
	var fromMemory, noLaunch bool
	start := &cobra.Command{
		Use:   "start",
		Short: "Start a coding agent session from shared .so/memory",
		RunE: func(cmd *cobra.Command, args []string) error {
			if vendor == "" {
				return fmt.Errorf("--vendor required (claude|cursor|codex)")
			}
			root := repoRoot()
			paths := harness.Resolve(root)
			s := memory.NewStore(paths)
			m := memory.Mode(mode)
			if m == "" {
				m = memory.ModePersistent
			}
			pack, err := s.BuildSessionContext(12000, query, m)
			if err != nil {
				return axi.Err(err)
			}
			_ = audit.Append(paths, audit.Event{
				Action: "session.start_from_memory", Type: "session", Vendor: vendor,
				Detail: fmt.Sprintf("chars=%d launch=%v", pack.CharCount, !noLaunch),
			})
			data := map[string]any{
				"vendor": vendor, "active_path": pack.ActivePath, "char_count": pack.CharCount,
				"mode": m, "launched": !noLaunch,
			}
			if err := out().HumanOrJSON("session_start", func() {
				fmt.Printf("Wrote memory pack (%d chars) → %s\n", pack.CharCount, pack.ActivePath)
			}, data); err != nil {
				return err
			}
			if noLaunch {
				return nil
			}
			bin := "claude"
			switch strings.ToLower(vendor) {
			case "cursor":
				bin = "cursor"
			case "codex":
				bin = "codex"
			case "claude", "claude-code", "cc":
				bin = "claude"
			}
			fmt.Fprintf(os.Stderr, "Launching %s with memory pack ready. Agents should read %s\n", bin, pack.ActivePath)
			ex := exec.Command(bin)
			ex.Stdin, ex.Stdout, ex.Stderr = os.Stdin, os.Stdout, os.Stderr
			return ex.Run()
		},
	}
	start.Flags().StringVar(&vendor, "vendor", "", "claude|cursor|codex")
	start.Flags().StringVar(&query, "query", "", "Optional memory retrieval query")
	start.Flags().StringVar(&mode, "mode", "persistent", "persistent|incognito|temporary")
	start.Flags().BoolVar(&fromMemory, "from-memory", true, "Build memory pack (always true)")
	start.Flags().BoolVar(&noLaunch, "no-launch", false, "Only write context.md")
	_ = fromMemory
	sessions.AddCommand(start)
}

func mineOnFinalize(paths harness.Paths, sessionID string, cfg config.Config) {
	_, _, _ = learn.MineSessionFile(paths, sessionID, llm.NewMemoryCompleter(cfg))
}
