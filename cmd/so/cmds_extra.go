package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ishanjainn/superopen/internal/axi"
	"github.com/ishanjainn/superopen/internal/blame"
	"github.com/ishanjainn/superopen/internal/checkpoint"
	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/entitlement"
	"github.com/ishanjainn/superopen/internal/githooks"
	"github.com/ishanjainn/superopen/internal/gitruntime"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/llm"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/port"
	"github.com/ishanjainn/superopen/internal/projects"
	"github.com/ishanjainn/superopen/internal/retention"
	"github.com/ishanjainn/superopen/internal/session"
	syncpkg "github.com/ishanjainn/superopen/internal/sync"
)

func cmdProjects() *cobra.Command {
	c := &cobra.Command{Use: "projects", Short: "Manage registered Superopen projects"}
	c.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List registered projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			list, err := projects.List()
			if err != nil {
				return err
			}
			for _, p := range list {
				missing := ""
				if _, err := os.Stat(p.RepoRoot); err != nil {
					missing = "  (missing)"
				}
				fmt.Printf("%s  %s  %s%s\n", p.ID, p.Name, p.RepoRoot, missing)
			}
			return nil
		},
	})
	c.AddCommand(&cobra.Command{
		Use:   "add [path]",
		Short: "Register a repo path",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := repoRoot()
			if len(args) > 0 {
				root = args[0]
			}
			paths := harness.Resolve(root)
			p, err := projects.Register(root, paths.Root, "")
			if err != nil {
				return err
			}
			fmt.Printf("registered %s (%s)\n", p.Name, p.ID)
			return nil
		},
	})
	var removePurge bool
	removeCmd := &cobra.Command{
		Use:   "remove [id-or-path]",
		Short: "Unregister a project (optionally delete its .so data)",
		Long: `Remove a project from ~/.config/superopen/projects.json.

With --purge, also deletes that project's .so directory (sessions, traces, graph).
Works even when the repo path no longer exists on disk.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := projects.Remove(args[0], projects.RemoveOptions{PurgeSO: removePurge})
			if err != nil {
				return err
			}
			msg := fmt.Sprintf("removed %s (%s) from projects.json", res.Project.Name, res.Project.ID)
			if res.RepoMissing {
				msg += " - repo path was missing"
			}
			if res.PurgedSO {
				msg += fmt.Sprintf("; deleted %s", res.SOPath)
			}
			fmt.Println(msg)
			return nil
		},
	}
	removeCmd.Flags().BoolVar(&removePurge, "purge", false, "also delete the project's .so directory")
	c.AddCommand(removeCmd)

	var prunePurge bool
	pruneCmd := &cobra.Command{
		Use:   "prune",
		Short: "Unregister projects whose repo path no longer exists",
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := projects.PruneMissing(prunePurge)
			if err != nil {
				return err
			}
			if len(res) == 0 {
				fmt.Println("no missing projects")
				return nil
			}
			for _, r := range res {
				extra := ""
				if r.PurgedSO {
					extra = "; purged " + r.SOPath
				}
				fmt.Printf("pruned %s (%s)%s\n", r.Project.Name, r.Project.RepoRoot, extra)
			}
			return nil
		},
	}
	pruneCmd.Flags().BoolVar(&prunePurge, "purge", false, "also delete leftover .so directories")
	c.AddCommand(pruneCmd)

	c.AddCommand(&cobra.Command{
		Use:  "use [id-or-path]",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := projects.Use(args[0])
			if err != nil {
				return err
			}
			fmt.Printf("active: %s\n", p.RepoRoot)
			return nil
		},
	})
	c.RunE = c.Commands()[0].RunE
	return c
}

func cmdStatus() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show active session state for this repo",
		RunE: func(cmd *cobra.Command, args []string) error {
			o := out()
			root := repoRoot()
			paths := harness.Resolve(root)
			st := session.NewStateStore(paths)
			active, err := st.ListActive()
			if err != nil {
				return axi.Err(err)
			}
			rows := make([]map[string]any, 0, len(active))
			for _, s := range active {
				row := map[string]any{
					"id": s.SessionID, "phase": s.Phase, "branch": s.Branch,
					"head": short(s.HeadSHA), "vendor": s.Vendor,
				}
				if w := st.WarnConcurrent(s.SessionID, s.WorktreeID); w != "" {
					row["warn"] = w
				}
				rows = append(rows, row)
			}
			if len(rows) == 0 {
				o.Next("so sessions list", "so dev")
				o.Empty("active_sessions")
				return nil
			}
			o.Rows("active_sessions", []string{"id", "phase", "branch", "head", "vendor"}, rows)
			return nil
		},
	}
}

func cmdCheckpoint() *cobra.Command {
	c := &cobra.Command{Use: "checkpoint", Short: "Manage restorable session checkpoints"}
	c.AddCommand(&cobra.Command{
		Use:  "list [session-id]",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cs := checkpoint.NewStore(harness.Resolve(repoRoot()))
			list, err := cs.List(args[0])
			if err != nil {
				return err
			}
			return out().HumanOrJSON("checkpoints", func() {
				for _, m := range list {
					fmt.Printf("%d  %s  %s  files=%d\n", m.ID, m.CreatedAt.Format(time.RFC3339), m.Label, len(m.Files))
				}
			}, list)
		},
	})
	c.AddCommand(&cobra.Command{
		Use:   "create [session-id] [label]",
		Args:  cobra.RangeArgs(1, 2),
		Short: "Snapshot edited files from session footprint (also auto on git commit / finalize)",
		RunE: func(cmd *cobra.Command, args []string) error {
			root := repoRoot()
			label := "checkpoint"
			if len(args) > 1 {
				label = args[1]
			}
			cs := checkpoint.NewStore(harness.Resolve(root))
			m, err := cs.CreateFromFootprint(args[0], root, label)
			if err != nil {
				return fmt.Errorf("%w (checkpoints are created automatically on git commit when the session has a footprint)", err)
			}
			return out().HumanOrJSON("result", func() {
				fmt.Printf("created checkpoint %d (%d files)\n", m.ID, len(m.Files))
			}, m)
		},
	})
	c.AddCommand(&cobra.Command{
		Use:  "show [session-id] [checkpoint-id]",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := checkpoint.NewStore(harness.Resolve(repoRoot())).Get(args[0], args[1])
			if err != nil {
				return err
			}
			return out().HumanOrJSON("checkpoint", func() {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				_ = enc.Encode(m)
			}, m)
		},
	})
	c.AddCommand(&cobra.Command{
		Use:  "restore [session-id] [checkpoint-id]",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := repoRoot()
			if err := checkpoint.NewStore(harness.Resolve(root)).Restore(args[0], args[1], root); err != nil {
				return err
			}
			return out().HumanOrJSON("result", func() {
				fmt.Println("Restored.")
			}, map[string]any{"ok": true, "session_id": args[0], "checkpoint_id": args[1]})
		},
	})
	return c
}

func cmdBlame() *cobra.Command {
	return &cobra.Command{
		Use:   "blame [file]",
		Args:  cobra.ExactArgs(1),
		Short: "Annotate file lines with SO session ids",
		RunE: func(cmd *cobra.Command, args []string) error {
			root := repoRoot()
			lines, err := blame.File(root, args[0], harness.Resolve(root))
			if err != nil {
				return err
			}
			for _, li := range lines {
				sid := li.SessionID
				if sid == "" {
					sid = "-"
				}
				fmt.Printf("%4d  %s  %-12s  %s\n", li.Line, short(li.CommitSHA), sid, truncate(li.Content, 60))
			}
			return nil
		},
	}
}

func cmdWhy() *cobra.Command {
	return &cobra.Command{
		Use:   "why [file:line]",
		Args:  cobra.ExactArgs(1),
		Short: "Explain which session produced a line",
		RunE: func(cmd *cobra.Command, args []string) error {
			file, line, err := parseFileLine(args[0])
			if err != nil {
				return err
			}
			root := repoRoot()
			res, err := blame.Why(root, file, line, harness.Resolve(root))
			if err != nil {
				return err
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(res)
		},
	}
}

func parseFileLine(s string) (string, int, error) {
	i := strings.LastIndex(s, ":")
	if i <= 0 {
		return "", 0, fmt.Errorf("expected file:line")
	}
	line, err := strconv.Atoi(s[i+1:])
	if err != nil {
		return "", 0, err
	}
	return s[:i], line, nil
}

func cmdGitHook() *cobra.Command {
	c := &cobra.Command{Use: "githook", Short: "Internal git hook entrypoints", Hidden: true}
	c.AddCommand(&cobra.Command{
		Use:  "prepare-commit-msg",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			msgPath := args[0]
			root := repoRoot()
			paths := harness.Resolve(root)
			st := session.NewStateStore(paths)
			active, _ := st.ListActive()
			sid := ""
			if len(active) > 0 {
				sid = active[0].SessionID
			} else {
				list, _ := session.NewStore(paths).List()
				if len(list) == 0 || list[0].Status != session.StatusActive {
					return nil
				}
				sid = list[0].ID
			}
			if err := githooks.AppendTrailer(msgPath, githooks.TrailerSession, sid); err != nil {
				return err
			}
			// Best-effort attribution trailer from footprint vs HEAD.
			ss := session.NewStore(paths)
			meta, _ := ss.Get(sid)
			fp, _ := ss.GetFootprint(sid)
			var files []string
			for _, f := range fp.Files {
				if f.State == "edited" {
					files = append(files, f.Path)
				}
			}
			shaOut, _ := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
			head := strings.TrimSpace(string(shaOut))
			base := meta.BaseSHA
			if base == "" {
				base = head
			}
			sum := session.ComputeAttribution(root, base, head, files)
			if sum.TotalChanged > 0 && sum.Display != "" {
				_ = githooks.AppendTrailer(msgPath, githooks.TrailerAttribution, sum.Display)
			}
			return nil
		},
	})
	c.AddCommand(&cobra.Command{
		Use:   "post-commit",
		Short: "Finalize session into untracked store; never rewrite tracked Superopen caches",
		RunE: func(cmd *cobra.Command, args []string) error {
			root := repoRoot()
			paths := harness.Resolve(root)
			_ = finalizeLatestSession(root)
			// Attribution + trailer already on message; refresh meta from HEAD.
			shaOut, _ := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
			sha := strings.TrimSpace(string(shaOut))
			msgOut, _ := exec.Command("git", "-C", root, "log", "-1", "--format=%B").Output()
			sid, attr := githooks.ParseTrailers(string(msgOut))
			if sid == "" {
				return nil
			}
			ss := session.NewStore(paths)
			meta, err := ss.Get(sid)
			if err != nil {
				return nil
			}
			session.MergeTrailerSession(&meta, sha, firstLine(string(msgOut)), time.Now().UTC())
			if attr != "" {
				meta.Attribution = &session.AttributionSummary{Display: attr}
			} else {
				fp, _ := ss.GetFootprint(sid)
				var files []string
				for _, f := range fp.Files {
					if f.State == "edited" {
						files = append(files, f.Path)
					}
				}
				base := meta.BaseSHA
				if base == "" {
					base = meta.HeadSHA
				}
				sum := session.ComputeAttribution(root, base, sha, files)
				if sum.TotalChanged > 0 {
					meta.Attribution = &sum
					// Amend is dangerous; skip rewriting commit. Attribution lives on meta.
				}
			}
			meta.HeadSHA = sha
			_ = ss.UpdateMeta(meta)
			// Auto checkpoint on commit.
			_, _ = checkpoint.NewStore(paths).CreateFromFootprint(sid, root, "post-commit")
			_, _ = gitruntime.SnapshotSessionDir(root, paths.SessionDir(sid), sid)
			_ = session.NewStateStore(paths).End(sid)
			return nil
		},
	})
	refreshHook := func(cmd *cobra.Command, args []string) error {
		// After any SHA-changing checkout/merge: rebuild untracked graph/context only.
		root := repoRoot()
		return syncpkg.Refresh(syncpkg.RefreshOptions{RepoRoot: root})
	}
	c.AddCommand(&cobra.Command{Use: "post-merge", RunE: refreshHook})
	c.AddCommand(&cobra.Command{Use: "post-checkout", RunE: refreshHook})
	c.AddCommand(&cobra.Command{
		Use:   "pre-push",
		Short: "Fast-forward push refs/so/sessions/* (never force)",
		RunE: func(cmd *cobra.Command, args []string) error {
			remote := "origin"
			if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
				remote = args[0]
			}
			return gitruntime.PushSessionsFF(repoRoot(), remote)
		},
	})
	return c
}

func cmdLogin() *cobra.Command {
	var email, token, otlp, query string
	c := &cobra.Command{
		Use:   "login",
		Short: "Authenticate CLI with paid Superopen UI (unlocks cloud OTLP)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if email == "" || token == "" {
				return fmt.Errorf("paid login requires --email and --token from the Superopen paid UI")
			}
			if otlp == "" && query == "" {
				return fmt.Errorf("paid login requires --otlp-endpoint and/or --query-endpoint from the paid UI")
			}
			exp := time.Now().Add(30 * 24 * time.Hour)
			if err := entitlement.LoginPaid(email, token, otlp, query, exp); err != nil {
				return err
			}
			fmt.Println("logged in - cloud OTLP enabled")
			return nil
		},
	}
	c.Flags().StringVar(&email, "email", "", "account email")
	c.Flags().StringVar(&token, "token", "", "session token from paid UI")
	c.Flags().StringVar(&otlp, "otlp-endpoint", "", "OTLP export endpoint")
	c.Flags().StringVar(&query, "query-endpoint", "", "session query API endpoint")
	return c
}

func cmdLogout() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Clear paid authentication",
		RunE: func(cmd *cobra.Command, args []string) error {
			return entitlement.Clear()
		},
	}
}

func extendSessionsCmd(c *cobra.Command) {
	attachSessionsPort(c)
	resume := &cobra.Command{
		Use:   "resume [session-id]",
		Args:  cobra.ExactArgs(1),
		Short: "Print vendor resume command for a session",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := harness.Resolve(repoRoot())
			ss := session.NewStore(paths)
			m, err := ss.Get(args[0])
			if err != nil {
				return err
			}
			cfg, _ := config.Load(paths.Config)
			if cfg.MemoryEnabled() {
				_, _ = memory.NewStore(paths).BuildSessionContext(12000, "", memory.ModePersistent)
			}
			vendorFlag, _ := cmd.Flags().GetString("vendor")
			vendor := strings.ToLower(m.Vendor)
			if vendorFlag != "" {
				vendor = strings.ToLower(vendorFlag)
			}
			root := repoRoot()
			to := harnessIDFromVendor(vendor)
			if sess, err := portableSessionFromHub(paths, m, to); err == nil {
				_ = port.ArmResume(root, to, m.ID, sess)
				fmt.Printf("# resume armed: next %s SessionStart injects %s\n", to, m.ID)
			}
			switch {
			case strings.Contains(vendor, "claude"):
				fmt.Printf("claude --resume %s\n", m.ID)
			case strings.Contains(vendor, "codex"):
				fmt.Printf("codex resume %s\n", m.ID)
			case strings.Contains(vendor, "cursor"):
				portDir := filepath.Join(root, ".cursor", "so-port", m.ID)
				_ = os.MkdirAll(portDir, 0o755)
				if _, err := os.Stat(filepath.Join(portDir, "conversation.md")); err != nil {
					_ = materializeCursorResumePack(paths, root, m)
				}
				fmt.Printf("# Pack: .cursor/so-port/%s/\n", m.ID)
				fmt.Printf("# Start any coding agent in %s - SessionStart injects the conversation.\n", root)
			case strings.Contains(vendor, "opencode"):
				fmt.Printf("# OpenCode session dump: ~/.opencode/sessions/%s.json\n", m.ID)
			case vendor == "pi" || strings.Contains(vendor, "pi"):
				fmt.Printf("# Pi resume pack under ~/.pi/agent/sessions/so-port/%s.jsonl\n", m.ID)
				fmt.Printf("# Start Pi in the repo; Active Context injects on session_start when memory.enabled.\n")
			default:
				fmt.Printf("# resume session %s (%s)\n", m.ID, m.Vendor)
			}
			st := session.NewStateStore(paths)
			_ = st.Save(session.State{
				SessionID: m.ID,
				Vendor:    m.Vendor,
				Phase:     session.PhaseActive,
				Branch:    m.Branch,
				HeadSHA:   m.HeadSHA,
				RepoRoot:  repoRoot(),
			})
			return nil
		},
	}
	resume.Flags().String("vendor", "", "claude|cursor|codex|opencode (override meta vendor)")
	c.AddCommand(resume)
	c.AddCommand(&cobra.Command{
		Use:   "attach [session-id]",
		Args:  cobra.ExactArgs(1),
		Short: "Mark a session active in this worktree",
		RunE: func(cmd *cobra.Command, args []string) error {
			root := repoRoot()
			paths := harness.Resolve(root)
			ss := session.NewStore(paths)
			m, err := ss.Get(args[0])
			if err != nil {
				return err
			}
			branch, _ := exec.Command("git", "-C", root, "rev-parse", "--abbrev-ref", "HEAD").Output()
			sha, _ := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
			st := session.NewStateStore(paths)
			if w := st.WarnConcurrent(m.ID, ""); w != "" {
				fmt.Fprintln(os.Stderr, w)
			}
			return st.Save(session.State{
				SessionID: m.ID,
				Vendor:    m.Vendor,
				Phase:     session.PhaseActive,
				Branch:    strings.TrimSpace(string(branch)),
				HeadSHA:   strings.TrimSpace(string(sha)),
				BaseSHA:   strings.TrimSpace(string(sha)),
				RepoRoot:  root,
			})
		},
	})
	c.AddCommand(&cobra.Command{
		Use:   "clean",
		Short: "Remove ended session-state files",
		RunE: func(cmd *cobra.Command, args []string) error {
			st := session.NewStateStore(harness.Resolve(repoRoot()))
			all, err := st.List()
			if err != nil {
				return err
			}
			n := 0
			for _, s := range all {
				if s.Phase == session.PhaseEnded {
					_ = st.Delete(s.SessionID)
					n++
				}
			}
			fmt.Printf("cleaned %d state file(s)\n", n)
			return nil
		},
	})
	c.AddCommand(&cobra.Command{
		Use:   "prune",
		Short: "Delete empty sessions and artifacts older than retention.days (default 7)",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := harness.Resolve(repoRoot())
			cfg, err := config.Load(paths.Config)
			if err != nil {
				cfg = config.Default()
			}
			rep, err := retention.Prune(paths, cfg)
			if err != nil {
				return err
			}
			return out().HumanOrJSON("prune", func() {
				fmt.Printf(
					"pruned empty=%d expired_sessions=%d evals=%d audit=%d recs=%d traces=%d (retention %dd)\n",
					rep.EmptySessions, rep.ExpiredSessions, rep.EvalHistory, rep.AuditEvents,
					rep.Recommendations, rep.TraceFiles, cfg.RetentionDays(),
				)
			}, rep)
		},
	})
	c.AddCommand(&cobra.Command{
		Use:   "explain [session-id]",
		Args:  cobra.ExactArgs(1),
		Short: "Explain a session (files, commits, summary)",
		RunE: func(cmd *cobra.Command, args []string) error {
			root := repoRoot()
			paths := harness.Resolve(root)
			ss := session.NewStore(paths)
			m, err := ss.Get(args[0])
			if err != nil {
				return err
			}
			fp, _ := ss.GetFootprint(args[0])
			cfg, _ := config.Load(paths.Config)
			text, err := session.Explain(m, fp, llm.NewFromConfig(cfg))
			if err != nil {
				return err
			}
			fmt.Print(text)
			return nil
		},
	})
	c.AddCommand(&cobra.Command{
		Use:   "tokens [session-id]",
		Args:  cobra.MaximumNArgs(1),
		Short: "Show token/cost for a session",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := harness.Resolve(repoRoot())
			ss := session.NewStore(paths)
			id := ""
			if len(args) > 0 {
				id = args[0]
			} else {
				list, _ := ss.List()
				if len(list) == 0 {
					return fmt.Errorf("no sessions")
				}
				id = list[0].ID
			}
			m, err := ss.Get(id)
			if err != nil {
				return err
			}
			fmt.Printf("session %s\n  tokens=%d  cost=$%.4f\n", m.ID, m.Tokens, m.CostUSD)
			return nil
		},
	})
}

func materializeCursorResumePack(paths harness.Paths, repoRoot string, m session.Meta) error {
	portDir := filepath.Join(repoRoot, ".cursor", "so-port", m.ID)
	_ = os.MkdirAll(portDir, 0o755)
	src := filepath.Join(paths.SessionDir(m.ID), "transcript.jsonl")
	data, err := os.ReadFile(src)
	if err != nil {
		data = []byte{}
	}
	_ = os.WriteFile(filepath.Join(portDir, "transcript.jsonl"), data, 0o644)
	var conv strings.Builder
	conv.WriteString("# Ported conversation (Cursor resume pack)\n\n")
	if m.Title != "" {
		conv.WriteString("Title: " + m.Title + "\n\n")
	}
	if m.PromptPreview != "" {
		conv.WriteString("## user\n\n" + m.PromptPreview + "\n\n")
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row map[string]any
		if json.Unmarshal([]byte(line), &row) != nil {
			continue
		}
		role, _ := row["role"].(string)
		text, _ := row["text"].(string)
		if text == "" {
			continue
		}
		if role == "" {
			role = "turn"
		}
		conv.WriteString("## " + role + "\n\n" + text + "\n\n")
	}
	return os.WriteFile(filepath.Join(portDir, "conversation.md"), []byte(conv.String()), 0o644)
}

func harnessIDFromVendor(vendor string) port.HarnessID {
	v := strings.ToLower(strings.TrimSpace(vendor))
	switch {
	case strings.Contains(v, "claude"):
		return port.HarnessClaude
	case strings.Contains(v, "codex"):
		return port.HarnessCodex
	case strings.Contains(v, "opencode"):
		return port.HarnessOpenCode
	case strings.Contains(v, "cursor"):
		return port.HarnessCursor
	case v == "pi" || strings.Contains(v, "pi"):
		return port.HarnessPi
	default:
		return port.HarnessSOHub
	}
}

func portableSessionFromHub(paths harness.Paths, m session.Meta, to port.HarnessID) (port.PortableSession, error) {
	src := filepath.Join(paths.SessionDir(m.ID), "transcript.jsonl")
	data, err := os.ReadFile(src)
	if err != nil {
		data = []byte{}
	}
	sess := port.NewPortableSession(to, m.ID, src, repoRoot(), m.Title)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row map[string]any
		if json.Unmarshal([]byte(line), &row) != nil {
			continue
		}
		role, _ := row["role"].(string)
		text, _ := row["text"].(string)
		if text == "" {
			continue
		}
		if role == "" {
			role = "user"
		}
		sess.Turns = append(sess.Turns, port.PortableTurn{Role: role, Text: text})
	}
	if len(sess.Turns) == 0 && m.PromptPreview != "" {
		sess.Turns = append(sess.Turns, port.PortableTurn{Role: "user", Text: m.PromptPreview})
	}
	return sess, nil
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

// ensureAbsSoBin used when installing git hooks.
func ensureAbsSoBin() string {
	if exe, err := os.Executable(); err == nil {
		if abs, err := filepath.Abs(exe); err == nil {
			return abs
		}
	}
	return "so"
}
