package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ishanjainn/superopen/internal/cli"
	"github.com/ishanjainn/superopen/internal/memory"
)

func cmdMemory() *cobra.Command {
	command := &cobra.Command{
		Use:   "memory",
		Short: "Project diary over coding sessions (search, capture, teach, distill)",
	}
	command.AddCommand(
		memorySearchCmd(),
		memoryRecallCmd(),
		memoryTemporalRecallCmd(),
		memoryGetCmd(),
		memoryTimelineCmd(),
		memoryLastCmd(),
		memoryCaptureCmd(),
		memoryContradictCmd(),
		memoryIngestCmd(),
		memoryTeachCmd(),
		memoryUploadCmd(),
		memoryWatchCmd(),
		memoryPinCmd("pin", false),
		memoryPinCmd("fade", true),
		memoryRescueCmd(),
		memoryStatusCmd(),
		memoryDistillCmd(),
		memorySleepCmd(),
		memoryLayoutCmd(),
		memoryEmbedWorkerCmd(),
		memoryObserveCmd(),
	)
	return command
}

func memorySearchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Hybrid search index (IDs + type + title + tokens, no bodies)",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := memory.OpenRoot(repoRoot())
			if err != nil {
				return err
			}
			defer store.Close()
			limit, _ := cmd.Flags().GetInt("limit")
			kind, _ := cmd.Flags().GetString("kind")
			typ, _ := cmd.Flags().GetString("type")
			sessionID, _ := cmd.Flags().GetString("session")
			file, _ := cmd.Flags().GetString("file")
			hits, err := store.Search(memory.SearchFilter{
				Query:         strings.Join(args, " "),
				Kind:          kind,
				Type:          typ,
				SessionID:     sessionID,
				File:          file,
				Limit:         limit,
				RecordEconomy: true,
			})
			if err != nil {
				return err
			}
			out := out()
			out.Next(memory.HelpForSearch(hits)...)
			rows := make([]memory.IndexHit, 0, len(hits))
			for _, hit := range hits {
				rows = append(rows, memory.IndexFromHit(hit))
			}
			if len(rows) == 0 {
				out.Empty("memories")
				return nil
			}
			return out.HumanOrJSON("memories", func() {
				for _, hit := range hits {
					fmt.Fprintln(cmd.OutOrStdout(), memory.FormatHit(hit.Episode))
				}
			}, rows)
		},
	}
	cmd.Flags().Int("limit", 20, "Maximum results")
	cmd.Flags().String("kind", "", "Filter kind (prompt|session|pin|teaching|working|observation)")
	cmd.Flags().String("type", "", "Observation type (decision|bugfix|feature|refactor|discovery|change)")
	cmd.Flags().String("session", "", "Filter by session id")
	cmd.Flags().String("file", "", "Filter by file path substring")
	return cmd
}

func memoryRecallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recall [query]",
		Short: "Budgeted pack with hits and anti-hits (on-demand, not a hook)",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := memory.OpenRoot(repoRoot())
			if err != nil {
				return err
			}
			defer store.Close()
			budget, _ := cmd.Flags().GetInt("budget")
			structural, _ := cmd.Flags().GetBool("structural")
			query := strings.Join(args, " ")
			if structural {
				hits, err := store.RecallShape(query, 12)
				if err != nil {
					return err
				}
				for _, h := range hits {
					_ = store.Reinforce(h.ID)
				}
				out := out()
				out.Next(memory.HelpForSearch(hits)...)
				return out.HumanOrJSON("memory_recall", func() {
					for _, hit := range hits {
						fmt.Fprintln(cmd.OutOrStdout(), memory.FormatHit(hit.Episode))
					}
				}, hits)
			}
			res, err := store.Recall(query, budget)
			if err != nil {
				return err
			}
			for _, h := range res.Hits {
				_ = store.Reinforce(h.ID)
			}
			out := out()
			out.Next(memory.HelpForSearch(res.Hits)...)
			return out.HumanOrJSON("memory_recall", func() {
				fmt.Fprintf(cmd.OutOrStdout(), "hits: %d  anti_hits: %d  budget: %d\n", len(res.Hits), len(res.AntiHits), res.BudgetTokens)
				for _, hit := range res.Hits {
					fmt.Fprintln(cmd.OutOrStdout(), memory.FormatHit(hit.Episode))
				}
				for _, hit := range res.AntiHits {
					fmt.Fprintf(cmd.OutOrStdout(), "anti %s\n", memory.FormatHit(hit.Episode))
				}
			}, res)
		},
	}
	cmd.Flags().Int("budget", 1500, "Token budget")
	cmd.Flags().Bool("structural", false, "Shape/HD recall over embeddings")
	return cmd
}

func memoryTemporalRecallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "temporal-recall [query]",
		Short: "Time-bounded recall (--as-of, --changed-since)",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := memory.OpenRoot(repoRoot())
			if err != nil {
				return err
			}
			defer store.Close()
			asOf, _ := cmd.Flags().GetString("as-of")
			changed, _ := cmd.Flags().GetString("changed-since")
			budget, _ := cmd.Flags().GetInt("budget")
			res, err := store.TemporalRecall(strings.Join(args, " "), asOf, changed, budget)
			if err != nil {
				return err
			}
			return out().HumanOrJSON("memory_recall", func() {
				fmt.Fprintf(cmd.OutOrStdout(), "hits: %d  anti_hits: %d\n", len(res.Hits), len(res.AntiHits))
				for _, hit := range res.Hits {
					fmt.Fprintf(cmd.OutOrStdout(), "  #%d %s %s %s\n", hit.ID, hit.Kind, hit.Title, hit.CreatedAt)
				}
			}, res)
		},
	}
	cmd.Flags().String("as-of", "", "RFC3339 or YYYY-MM-DD valid window")
	cmd.Flags().String("changed-since", "", "Only memories updated since this time")
	cmd.Flags().Int("budget", 1500, "Token budget")
	return cmd
}

func memoryLastCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "last",
		Short: "Recent non-tool memories",
		RunE: func(cmd *cobra.Command, _ []string) error {
			store, err := memory.OpenRoot(repoRoot())
			if err != nil {
				return err
			}
			defer store.Close()
			limit, _ := cmd.Flags().GetInt("n")
			eps, err := store.Recent(limit)
			if err != nil {
				return err
			}
			return out().HumanOrJSON("memory_last", func() {
				for _, ep := range eps {
					fmt.Fprintf(cmd.OutOrStdout(), "#%d %s %s\n", ep.ID, ep.Kind, ep.Title)
				}
			}, eps)
		},
	}
	cmd.Flags().Int("n", 20, "Count")
	return cmd
}

func memoryGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id> [id…]",
		Short: "Show one or more memory episodes by id (bodies)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := memory.ParseIDs(args)
			if err != nil {
				return err
			}
			store, err := memory.OpenRoot(repoRoot())
			if err != nil {
				return err
			}
			defer store.Close()
			eps, err := store.GetMany(ids)
			if err != nil {
				return err
			}
			for _, ep := range eps {
				_ = store.Reinforce(ep.ID)
			}
			out := out()
			out.Next(memory.HelpForGet(eps)...)
			return out.HumanOrJSON("memory", func() {
				for _, ep := range eps {
					fmt.Fprintf(cmd.OutOrStdout(), "%s\n%s\n", memory.FormatHit(ep), ep.Text)
				}
			}, eps)
		},
	}
}

func memoryTimelineCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "timeline",
		Short: "WHEN folders, or --around <id> for neighboring episodes",
		RunE: func(cmd *cobra.Command, _ []string) error {
			store, err := memory.OpenRoot(repoRoot())
			if err != nil {
				return err
			}
			defer store.Close()
			around, _ := cmd.Flags().GetInt64("around")
			before, _ := cmd.Flags().GetInt("before")
			after, _ := cmd.Flags().GetInt("after")
			if around > 0 {
				eps, err := store.TimelineAround(around, before, after)
				if err != nil {
					return err
				}
				out := out()
				if len(eps) > 0 {
					out.Next(fmt.Sprintf("so memory get %d", around))
				}
				rows := make([]memory.IndexHit, 0, len(eps))
				for _, ep := range eps {
					rows = append(rows, memory.IndexFromEpisode(ep))
				}
				if len(rows) == 0 {
					out.Empty("memories")
					return nil
				}
				return out.HumanOrJSON("memory_timeline", func() {
					for _, ep := range eps {
						fmt.Fprintln(cmd.OutOrStdout(), memory.FormatHit(ep))
					}
				}, rows)
			}
			limit, _ := cmd.Flags().GetInt("limit")
			buckets, err := store.Timeline(limit)
			if err != nil {
				return err
			}
			return out().HumanOrJSON("memory_timeline", func() {
				for _, b := range buckets {
					fmt.Fprintf(cmd.OutOrStdout(), "%s (%d)\n", b.When, len(b.Items))
					for _, ep := range b.Items {
						fmt.Fprintln(cmd.OutOrStdout(), "  "+memory.FormatHit(ep))
					}
				}
			}, buckets)
		},
	}
	cmd.Flags().Int("limit", 80, "Maximum episodes")
	cmd.Flags().Int64("around", 0, "Anchor episode id")
	cmd.Flags().Int("before", 5, "Episodes before --around")
	cmd.Flags().Int("after", 5, "Episodes after --around")
	return cmd
}

func memoryCaptureCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "capture",
		Short: "Write a session rollup or note (request/learned/next)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			sessionID, _ := cmd.Flags().GetString("session")
			title, _ := cmd.Flags().GetString("title")
			text, _ := cmd.Flags().GetString("text")
			request, _ := cmd.Flags().GetString("request")
			learned, _ := cmd.Flags().GetString("learned")
			next, _ := cmd.Flags().GetString("next")
			kind, _ := cmd.Flags().GetString("kind")
			pin, _ := cmd.Flags().GetBool("pin")
			if text == "" {
				var parts []string
				if request != "" {
					parts = append(parts, "request: "+request)
				}
				if learned != "" {
					parts = append(parts, "learned: "+learned)
				}
				if next != "" {
					parts = append(parts, "next: "+next)
				}
				text = strings.Join(parts, "\n")
			}
			ep, err := memory.CaptureRoot(repoRoot(), memory.CaptureInput{
				SessionID: sessionID,
				Kind:      kind,
				Source:    memory.SourceAgent,
				Title:     title,
				Text:      text,
				Pin:       pin,
			})
			if err != nil {
				return err
			}
			return out().HumanOrJSON("memory", func() {
				fmt.Fprintf(cmd.OutOrStdout(), "captured #%d %s\n", ep.ID, ep.Title)
			}, ep)
		},
	}
	cmd.Flags().String("session", "", "Session id")
	cmd.Flags().String("title", "", "Title")
	cmd.Flags().String("text", "", "Body")
	cmd.Flags().String("request", "", "What was asked")
	cmd.Flags().String("learned", "", "What was learned")
	cmd.Flags().String("next", "", "Next steps")
	cmd.Flags().String("kind", memory.KindSession, "Episode kind")
	cmd.Flags().Bool("pin", false, "Pin after capture")
	return cmd
}

func memoryContradictCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "contradict <id>",
		Short: "Write a successor memory that contradicts an older one",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			oldID, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return err
			}
			text, _ := cmd.Flags().GetString("text")
			title, _ := cmd.Flags().GetString("title")
			store, err := memory.OpenRoot(repoRoot())
			if err != nil {
				return err
			}
			defer store.Close()
			ep, err := store.Contradict(oldID, memory.CaptureInput{
				Title:  title,
				Text:   text,
				Source: memory.SourceAgent,
			})
			if err != nil {
				return err
			}
			return out().HumanOrJSON("memory", func() {
				fmt.Fprintf(cmd.OutOrStdout(), "contradict #%d → #%d\n", oldID, ep.ID)
			}, ep)
		},
	}
	cmd.Flags().String("text", "", "Corrected fact")
	cmd.Flags().String("title", "", "Title")
	return cmd
}

func memoryIngestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ingest [session-id]",
		Short: "Project events.jsonl into memory episodes (local, no LLM)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := repoRoot()
			backfill, _ := cmd.Flags().GetBool("backfill")
			current, _ := cmd.Flags().GetString("current")
			if backfill {
				all := memory.IngestBackfill(root, current, 8)
				return out().HumanOrJSON("memory_ingest", func() {
					fmt.Fprintf(cmd.OutOrStdout(), "backfill %d session(s)\n", len(all))
				}, all)
			}
			if len(args) == 1 {
				res, err := memory.IngestSession(root, args[0])
				if err != nil {
					return err
				}
				return out().HumanOrJSON("memory_ingest", func() {
					fmt.Fprintf(cmd.OutOrStdout(), "ingested %s inserted=%d existing=%d skipped=%d\n", res.SessionID, res.Inserted, res.Existing, res.Skipped)
				}, res)
			}
			all, err := memory.IngestAll(root)
			if err != nil {
				return err
			}
			return out().HumanOrJSON("memory_ingest", func() {
				fmt.Fprintf(cmd.OutOrStdout(), "ingested %d session(s)\n", len(all))
			}, all)
		},
	}
	cmd.Flags().Bool("backfill", false, "Ingest up to 8 recent sessions missing KindPrompt")
	cmd.Flags().String("current", "", "Session id to skip during --backfill")
	return cmd
}

func memoryTeachCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "teach [file]",
		Short: "Study a file or --text (chunked, deduped, recall-verified)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			title, _ := cmd.Flags().GetString("title")
			text, _ := cmd.Flags().GetString("text")
			if len(args) == 1 {
				rep, err := memory.TeachPath(repoRoot(), args[0], title)
				if err != nil {
					return err
				}
				return out().HumanOrJSON("memory_teach", func() {
					fmt.Fprintf(cmd.OutOrStdout(), "taught inserted=%d verified=%d/%d\n", rep.Inserted, rep.RecallVerified, rep.RecallTested)
				}, rep)
			}
			ep, err := memory.TeachText(repoRoot(), title, text)
			if err != nil {
				return err
			}
			return out().HumanOrJSON("memory", func() {
				fmt.Fprintf(cmd.OutOrStdout(), "taught #%d %s\n", ep.ID, ep.Title)
			}, ep)
		},
	}
	cmd.Flags().String("title", "", "Title")
	cmd.Flags().String("text", "", "Inline teaching text")
	return cmd
}

func memoryUploadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "upload <path>",
		Short: "Chunk and capture a file or directory into memory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rep, err := memory.TeachPath(repoRoot(), args[0], "")
			if err != nil {
				return err
			}
			return out().HumanOrJSON("memory_teach", func() {
				fmt.Fprintf(cmd.OutOrStdout(), "uploaded inserted=%d\n", rep.Inserted)
			}, rep)
		},
	}
}

func memoryWatchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "watch [dir]",
		Short: "Restudy changed files and fade deleted teachings",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			once, _ := cmd.Flags().GetBool("once")
			root := repoRoot()
			run := func() error {
				res, err := memory.WatchOnce(root, dir)
				if err != nil {
					return err
				}
				return out().HumanOrJSON("memory_watch", func() {
					fmt.Fprintf(cmd.OutOrStdout(), "taught=%d faded=%d\n", res.Taught.Inserted, len(res.Faded))
				}, res)
			}
			if once {
				return run()
			}
			for {
				if err := run(); err != nil {
					return err
				}
				time.Sleep(2 * time.Second)
			}
		},
	}
	cmd.Flags().Bool("once", false, "Run a single pass")
	return cmd
}

func memoryPinCmd(use string, fade bool) *cobra.Command {
	short := "Pin a memory"
	if fade {
		short = "Fade a memory (rescue until consolidate)"
	}
	return &cobra.Command{
		Use:   use + " <id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return err
			}
			store, err := memory.OpenRoot(repoRoot())
			if err != nil {
				return err
			}
			defer store.Close()
			if fade {
				err = store.Fade(id)
			} else {
				err = store.Pin(id)
			}
			if err != nil {
				return err
			}
			ep, _ := store.Get(id)
			return out().HumanOrJSON("memory", func() {
				fmt.Fprintf(cmd.OutOrStdout(), "%s #%d\n", use, id)
			}, ep)
		},
	}
}

func memoryRescueCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rescue <id>",
		Short: "Unfade a memory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return err
			}
			store, err := memory.OpenRoot(repoRoot())
			if err != nil {
				return err
			}
			defer store.Close()
			if err := store.Rescue(id); err != nil {
				return err
			}
			ep, _ := store.Get(id)
			return out().HumanOrJSON("memory", func() {
				fmt.Fprintf(cmd.OutOrStdout(), "rescued #%d\n", id)
			}, ep)
		},
	}
}

func memoryStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Memory health, economy, pending distill",
		RunE: func(cmd *cobra.Command, _ []string) error {
			store, err := memory.OpenRoot(repoRoot())
			if err != nil {
				return err
			}
			defer store.Close()
			st, err := store.Status()
			if err != nil {
				return err
			}
			return out().HumanOrJSON("memory_status", func() {
				fmt.Fprintf(cmd.OutOrStdout(), "moments: %d  knowledge: %d  skills: %d  connections: %d  forgetting: %d\n",
					st.Counts.Episodic, st.Counts.Semantic, st.Counts.Procedural, st.Counts.Edges, st.Counts.Tombstoned)
				fmt.Fprintf(cmd.OutOrStdout(), "lifecycle: %s  coverage: %.1f%%  connected: %.2fx  cleaned: %.1f%%  pending: %d\n",
					st.Lifecycle, st.KnowledgePct, st.Connected, st.CleanedPct, len(st.PendingDistill))
				fmt.Fprintf(cmd.OutOrStdout(), "economy packs=%d injected=%d saved=%d searches=%d\n",
					st.Economy.PacksServed, st.Economy.TokensInjected, st.Economy.TokensSaved, st.Economy.FallbackSearches)
			}, st)
		},
	}
}

func memoryDistillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "distill [session-id]",
		Short: "Headless session rollup, or mark pending / pause / consolidate",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := repoRoot()
			pause, _ := cmd.Flags().GetBool("pause")
			resume, _ := cmd.Flags().GetBool("resume")
			consolidate, _ := cmd.Flags().GetBool("consolidate")
			sleep, _ := cmd.Flags().GetBool("sleep")
			restart, _ := cmd.Flags().GetBool("restart")
			detach, _ := cmd.Flags().GetBool("detach")
			if restart {
				store, err := memory.OpenRoot(root)
				if err != nil {
					return err
				}
				if err := store.SetDistillPaused(false); err != nil {
					store.Close()
					return err
				}
				store.Close()
				consolidate = true
			}
			if sleep {
				if err := memory.SleepRoot(root); err != nil {
					return err
				}
				return out().HumanOrJSON("memory_sleep", func() {
					fmt.Fprintf(cmd.OutOrStdout(), "slept\n")
				}, map[string]any{"ok": true})
			}
			if pause || resume {
				store, err := memory.OpenRoot(root)
				if err != nil {
					return err
				}
				defer store.Close()
				if err := store.SetDistillPaused(pause && !resume); err != nil {
					return err
				}
				return out().HumanOrJSON("memory_distill", func() {
					fmt.Fprintf(cmd.OutOrStdout(), "distill paused=%v\n", pause && !resume)
				}, map[string]any{"paused": pause && !resume})
			}
			if consolidate {
				_, _ = memory.IngestAll(root)
				store, err := memory.OpenRoot(root)
				if err != nil {
					return err
				}
				_ = store.ClusterTopics()
				pending := append([]string{}, store.PendingDistill()...)
				store.Close()
				var results []memory.DistillResult
				for _, id := range pending {
					results = append(results, memory.MaybeDistill(root, id, detach))
				}
				return out().HumanOrJSON("memory_distill", func() {
					fmt.Fprintf(cmd.OutOrStdout(), "consolidated %d pending\n", len(results))
				}, results)
			}
			id := ""
			if len(args) == 1 {
				id = args[0]
			}
			if detach {
				spawn := []string{"memory", "distill"}
				if id != "" {
					spawn = append(spawn, id)
				}
				cli.SpawnSO(root, spawn...)
				return nil
			}
			if id == "" {
				return fmt.Errorf("session id required (or --consolidate / --pause / --sleep)")
			}
			res, err := memory.DistillSession(root, id)
			if err != nil {
				return err
			}
			return out().HumanOrJSON("memory_distill", func() {
				if res.Pending {
					fmt.Fprintf(cmd.OutOrStdout(), "pending %s (%s)\n", res.SessionID, res.Skipped)
					return
				}
				fmt.Fprintf(cmd.OutOrStdout(), "distilled %s via %s → #%d\n", res.SessionID, res.Provider, res.EpisodeID)
			}, res)
		},
	}
	cmd.Flags().Bool("detach", false, "Run in the background")
	cmd.Flags().Bool("pause", false, "Pause automatic distill")
	cmd.Flags().Bool("resume", false, "Resume automatic distill")
	cmd.Flags().Bool("consolidate", false, "Ingest all sessions, cluster topics, distill pending")
	cmd.Flags().Bool("sleep", false, "Run the sleep pipeline (decay, erase hints, cluster)")
	cmd.Flags().Bool("restart", false, "Resume distill then consolidate")
	return cmd
}

func memorySleepCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sleep",
		Short: "Decay edges, erase hinted memories, cluster topics",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := memory.SleepRoot(repoRoot()); err != nil {
				return err
			}
			return out().HumanOrJSON("memory_sleep", func() {
				fmt.Fprintf(cmd.OutOrStdout(), "slept\n")
			}, map[string]any{"ok": true})
		},
	}
}

func memoryLayoutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "layout",
		Short: "Emit a render-ready memory map (stellar coordinates)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			store, err := memory.OpenRoot(repoRoot())
			if err != nil {
				return err
			}
			defer store.Close()
			maxNodes, _ := cmd.Flags().GetInt("max-nodes")
			layout, err := store.Layout(maxNodes)
			if err != nil {
				return err
			}
			return out().HumanOrJSON("memory_layout", func() {
				fmt.Fprintf(cmd.OutOrStdout(), "nodes: %d  edges: %d\n", len(layout.Nodes), len(layout.Edges))
			}, layout)
		},
	}
	cmd.Flags().Int("max-nodes", 2000, "Node budget")
	return cmd
}

func memoryObserveCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "observe [session-id]",
		Short:  "Write typed observation rows (heuristic; headless if available)",
		Hidden: true,
		Args:   cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := ""
			if len(args) == 1 {
				id = args[0]
			}
			if id == "" {
				return fmt.Errorf("session id required")
			}
			res, err := memory.ObserveSession(repoRoot(), id)
			if err != nil {
				return err
			}
			return out().HumanOrJSON("memory_observe", func() {
				fmt.Fprintf(cmd.OutOrStdout(), "observed %s inserted=%d via %s\n", res.SessionID, res.Inserted, res.Provider)
			}, res)
		},
	}
}

func memoryEmbedWorkerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "embed-worker",
		Short:  "Loopback embed HTTP (spawned by hooks)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			listen, _ := cmd.Flags().GetString("listen")
			return memory.ServeEmbedWorker(listen)
		},
	}
	cmd.Flags().String("listen", "127.0.0.1:0", "Listen address")
	return cmd
}
