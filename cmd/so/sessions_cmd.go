package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ishanjainn/superopen/internal/agent/hook"
	"github.com/ishanjainn/superopen/internal/checkpoint"
	"github.com/ishanjainn/superopen/internal/cli"
	"github.com/ishanjainn/superopen/internal/harvest"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/projects"
	"github.com/ishanjainn/superopen/internal/session"
	"github.com/ishanjainn/superopen/internal/session/replay"
	"github.com/ishanjainn/superopen/internal/session/trace"
)

func cmdSessions() *cobra.Command {
	command := &cobra.Command{
		Use:   "sessions",
		Short: "Inspect observability-derived coding sessions",
		Args:  cobra.ArbitraryArgs,
	}
	list := &cobra.Command{
		Use:   "list",
		Short: "List coding sessions",
		RunE:  runSessionsList,
	}
	command.AddCommand(list)
	command.AddCommand(&cobra.Command{
		Use:   "show <id>",
		Short: "Show a materialized session document",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store := session.NewStore(paths.Resolve(repoRoot()))
			document, err := store.ReadDocument(args[0])
			if err != nil {
				return cli.NotFound(fmt.Sprintf("session %s not found", args[0]), "so sessions list")
			}
			out := out()
			out.Next("so sessions finalize <id>", "so sessions tokens <id>")
			if out.Flags.JSON || out.Flags.Full {
				return out.HumanOrJSON("session", func() {
					enc := json.NewEncoder(out.W)
					enc.SetIndent("", "  ")
					_ = enc.Encode(document)
				}, document)
			}
			turns := store.TurnsFor(args[0], document.Turns)
			return out.HumanOrJSON("session", func() {
				fmt.Fprintf(out.W, "id: %s\n", document.ID)
				fmt.Fprintf(out.W, "vendor: %s\n", document.Vendor)
				fmt.Fprintf(out.W, "status: %s\n", document.Status)
				fmt.Fprintf(out.W, "title: %s\n", out.Truncate(session.DisplayName(document.Meta), cli.DefaultTruncate))
				fmt.Fprintf(out.W, "turns: %d\n", turns)
				fmt.Fprintf(out.W, "tokens: %d\n", document.Tokens)
			}, map[string]any{
				"id":     document.ID,
				"vendor": document.Vendor,
				"status": document.Status,
				"title":  session.DisplayName(document.Meta),
				"turns":  turns,
				"tokens": document.Tokens,
			})
		},
	})
	finalize := &cobra.Command{
		Use:   "finalize [id]",
		Short: "Materialize observability spans into a completed session and session map",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := ""
			if len(args) == 1 {
				id = strings.TrimSpace(args[0])
			}
			root, hookID := hookRepoAndSession()
			if skipIfUnmanaged(cmd, root) {
				return nil
			}
			if id == "" {
				id = hookID
			}
			detach, _ := cmd.Flags().GetBool("detach")
			if detach {
				spawn := []string{"--root", root, "sessions", "finalize"}
				if id != "" {
					spawn = append(spawn, id)
				}
				cli.SpawnSO(root, spawn...)
				return nil
			}
			return finalizeSession(root, id, out())
		},
	}
	finalize.Flags().Bool("detach", false, "Materialize in a background process")
	command.AddCommand(finalize)
	command.AddCommand(&cobra.Command{
		Use:   "refresh [id]",
		Short: "Refresh an active session from current observability spans",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			id := ""
			if len(args) == 1 {
				id = strings.TrimSpace(args[0])
			}
			return refreshSession(repoRoot(), id, out())
		},
	})
	command.AddCommand(&cobra.Command{
		Use:   "demo",
		Short: "Create a synthetic observability session for UI testing",
		RunE: func(*cobra.Command, []string) error {
			return demoSession(repoRoot())
		},
	})
	command.AddCommand(&cobra.Command{
		Use:   "tokens [id]",
		Short: "Show token and cost totals",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store := session.NewStore(paths.Resolve(repoRoot()))
			id := ""
			if len(args) == 1 {
				id = args[0]
			} else {
				items, _ := store.List()
				if len(items) == 0 {
					out := out()
					out.Next("so sessions list", "so install")
					out.Empty("sessions")
					return nil
				}
				id = items[0].ID
			}
			meta, err := store.Get(id)
			if err != nil {
				return err
			}
			return out().HumanOrJSON("session_tokens", func() {
				fmt.Fprintf(cmd.OutOrStdout(), "session %s\n  tokens=%d  cost=$%.4f\n", meta.ID, meta.Tokens, meta.CostUSD)
			}, map[string]any{"id": meta.ID, "tokens": meta.Tokens, "cost_usd": meta.CostUSD})
		},
	})
	command.AddCommand(cmdSessionCheckpoint())
	command.AddCommand(hook.NewCmd())
	command.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return cli.UnknownCommand("sessions", args[0])
		}
		return runSessionsList(cmd, args)
	}
	return command
}

func runSessionsList(cmd *cobra.Command, _ []string) error {
	root := repoRoot()
	items, err := session.NewStore(paths.Resolve(root)).ListDetailed()
	if err != nil {
		return err
	}
	out := out()
	out.Next("so sessions show <id>", "so sessions finalize <id>")
	rows := make([]map[string]any, 0, len(items))
	for _, item := range items {
		rows = append(rows, map[string]any{
			"id":     item.ID,
			"status": item.Status,
			"title":  session.DisplayName(item.Meta),
			"vendor": item.Vendor,
		})
	}
	out.Rows("sessions", []string{"id", "status", "title", "vendor"}, rows)
	return nil
}

func cmdSessionCheckpoint() *cobra.Command {
	command := &cobra.Command{Use: "checkpoint", Short: "Manage restorable session checkpoints"}
	command.AddCommand(&cobra.Command{
		Use:   "list <session-id>",
		Short: "List checkpoints for a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			list, err := checkpoint.NewStore(paths.Resolve(repoRoot())).List(args[0])
			if err != nil {
				return err
			}
			return out().HumanOrJSON("checkpoints", func() {
				for _, item := range list {
					fmt.Fprintf(cmd.OutOrStdout(), "%d  %s  %s  files=%d\n", item.ID, item.CreatedAt.Format(time.RFC3339), item.Label, len(item.Files))
				}
			}, list)
		},
	})
	command.AddCommand(&cobra.Command{
		Use:   "create <session-id> [label]",
		Short: "Snapshot edited files from the session footprint",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := repoRoot()
			label := "checkpoint"
			if len(args) > 1 {
				label = args[1]
			}
			meta, err := checkpoint.NewStore(paths.Resolve(root)).CreateFromFootprint(args[0], root, label)
			if err != nil {
				return fmt.Errorf("%w (checkpoints are also created automatically on session finalize)", err)
			}
			return out().HumanOrJSON("result", func() {
				fmt.Fprintf(cmd.OutOrStdout(), "created checkpoint %d (%d files)\n", meta.ID, len(meta.Files))
			}, meta)
		},
	})
	command.AddCommand(&cobra.Command{
		Use:   "show <session-id> <checkpoint-id>",
		Short: "Show a checkpoint manifest",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			meta, err := checkpoint.NewStore(paths.Resolve(repoRoot())).Get(args[0], args[1])
			if err != nil {
				return err
			}
			return out().HumanOrJSON("checkpoint", func() {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(meta)
			}, meta)
		},
	})
	command.AddCommand(&cobra.Command{
		Use:   "restore <session-id> <checkpoint-id>",
		Short: "Restore checkpoint files into the repository",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := repoRoot()
			if err := checkpoint.NewStore(paths.Resolve(root)).Restore(args[0], args[1], root); err != nil {
				return err
			}
			return out().HumanOrJSON("result", func() {
				fmt.Fprintln(cmd.OutOrStdout(), "Restored.")
			}, map[string]any{"ok": true, "session_id": args[0], "checkpoint_id": args[1]})
		},
	})
	return command
}

func finalizeSession(root, requestedID string, out *cli.Out) error {
	paths := paths.Resolve(root)
	id, spans, err := loadSessionSpans(trace.NewLocalJSONL(paths.TracesDir), requestedID)
	if err != nil {
		return err
	}
	if !session.SpansHaveActivity(spans) {
		_ = session.NewStore(paths).Delete(id)
		return nil
	}
	store := session.NewStore(paths)
	if existing, existingErr := store.Get(id); existingErr == nil && existing.Status == session.StatusEnded && existing.EndedAt != nil {
		latest := time.Time{}
		for _, span := range spans {
			at := time.Unix(0, span.EndTimeUnixN).UTC()
			if span.EndTimeUnixN == 0 {
				at = time.Unix(0, span.StartTimeUnixN).UTC()
			}
			if at.After(latest) {
				latest = at
			}
		}
		if !latest.After(*existing.EndedAt) {
			if out != nil {
				out.Next("so sessions show <id>", "so memory distill <id>")
				out.OK("session", map[string]any{"id": id, "status": existing.Status})
			}
			return nil
		}
	}
	_ = store.Start(session.Meta{ID: id, Vendor: session.VendorFromSpans(spans), StartedAt: session.StartTimeFromSpans(spans)})
	tokens, cost, _ := trace.NewLocalJSONL(paths.TracesDir).SessionCost(id)
	meta, err := store.MaterializeFromSpans(id, spans, tokens, cost)
	if err != nil {
		return err
	}
	session.BackfillFromGitLog(&meta, root, 50)
	meta.RepoRoot = root
	if err := store.UpdateMeta(meta); err != nil {
		return err
	}
	_ = session.NewStateStore(paths).End(id)
	_ = projects.TouchSession(root)
	if _, err := replay.BuildReplayFromSpans(paths, id, spans); err != nil {
		return err
	}
	if _, err := checkpoint.NewStore(paths).CreateFromFootprint(id, root, "finalize"); err != nil {
		// Footprint may be empty for prompt-only sessions.
		_ = err
	}
	if ing, err := memory.IngestSession(root, id); err != nil {
		fmt.Fprintf(os.Stderr, "so memory ingest: %v\n", err)
	} else {
		_ = ing
		_ = memory.MaybeDistill(root, id, true)
	}
	_ = harvest.MaybeGenerate(root, id)
	_ = runRetentionSweep(root)
	if out != nil {
		out.Next("so sessions show <id>", "so memory distill <id>")
		out.OK("session", map[string]any{"id": id, "status": meta.Status})
	}
	return nil
}

func refreshSession(root, requestedID string, out *cli.Out) error {
	paths := paths.Resolve(root)
	traceStore := trace.NewLocalJSONL(paths.TracesDir)
	id, spans, err := loadSessionSpans(traceStore, requestedID)
	if err != nil {
		return err
	}
	if !session.SpansHaveActivity(spans) {
		return nil
	}
	store := session.NewStore(paths)
	_ = store.Start(session.Meta{ID: id, Vendor: session.VendorFromSpans(spans), StartedAt: session.StartTimeFromSpans(spans)})
	tokens, cost, _ := traceStore.SessionCost(id)
	meta, err := store.MaterializeFromSpans(id, spans, tokens, cost)
	if err != nil {
		return err
	}
	meta.Status = session.StatusActive
	meta.EndedAt = nil
	meta.DurationMs = max(0, time.Since(meta.StartedAt).Milliseconds())
	meta.RepoRoot = root
	if err := store.UpdateMeta(meta); err != nil {
		return err
	}
	_ = session.NewStateStore(paths).Save(session.State{SessionID: id, Vendor: meta.Vendor, Phase: session.PhaseActive, RepoRoot: root})
	_, err = replay.BuildReplayFromSpans(paths, id, spans)
	if err != nil {
		return err
	}
	if _, ingErr := memory.IngestSession(root, id); ingErr != nil {
		fmt.Fprintf(os.Stderr, "so memory ingest: %v\n", ingErr)
	}
	if out != nil {
		out.Next("so sessions show <id>", "so sessions list")
		out.OK("session", map[string]any{"id": id, "status": meta.Status})
	}
	return nil
}

func loadSessionSpans(store *trace.LocalJSONL, requestedID string) (string, []trace.Span, error) {
	id := strings.TrimSpace(requestedID)
	if id == "" {
		var err error
		id, err = store.LatestSessionID()
		if err != nil {
			return "", nil, err
		}
	}
	if id == "" {
		return "", nil, fmt.Errorf("no observability sessions")
	}
	spans, err := store.Query(trace.QueryFilter{SessionID: id})
	if err != nil {
		return "", nil, err
	}
	if len(spans) == 0 {
		return "", nil, fmt.Errorf("no spans for session %s", id)
	}
	return id, spans, nil
}

func demoSession(root string) error {
	paths := paths.Resolve(root)
	if err := paths.EnsureDirs(); err != nil {
		return err
	}
	id := fmt.Sprintf("ses_demo_%d", time.Now().Unix())
	now := time.Now().UnixNano()
	spans := []trace.Span{
		{TraceID: id, SpanID: "1", Name: "coding_agent.prompt", StartTimeUnixN: now, EndTimeUnixN: now + 1e6, SessionID: id, Attributes: map[string]string{"coding_agent.vendor": "claude-code", "gen_ai.prompt": "Inspect the native graph", "gen_ai.request.model": "claude-haiku", "gen_ai.usage.total_tokens": "1200"}},
		{TraceID: id, SpanID: "2", Name: "coding_agent.search", StartTimeUnixN: now + 2e6, SessionID: id, Attributes: map[string]string{"coding_agent.vendor": "claude-code", "coding_agent.file_path": "internal/graph/engine/server.go"}},
	}
	traceStore := trace.NewLocalJSONL(paths.TracesDir)
	if err := traceStore.Write(spans); err != nil {
		return err
	}
	store := session.NewStore(paths)
	_ = store.Start(session.Meta{ID: id, Vendor: "claude-code", Model: "claude-haiku", PromptPreview: "Inspect the native graph", StartedAt: time.Unix(0, now).UTC()})
	if _, err := store.MaterializeFromSpans(id, spans, 1200, 0); err != nil {
		return err
	}
	if _, err := replay.BuildReplayFromSpans(paths, id, spans); err != nil {
		return err
	}
	fmt.Printf("Demo session %s created\n", id)
	return nil
}

func cmdStatus() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show active observability sessions",
		RunE: func(cmd *cobra.Command, _ []string) error {
			active, err := session.NewStateStore(paths.Resolve(repoRoot())).ListActive()
			if err != nil {
				return cli.Err(err)
			}
			rows := make([]map[string]any, 0, len(active))
			for _, state := range active {
				rows = append(rows, map[string]any{"id": state.SessionID, "phase": state.Phase, "vendor": state.Vendor, "branch": state.Branch})
			}
			out().Rows("active_sessions", []string{"id", "phase", "vendor", "branch"}, rows)
			return nil
		},
	}
}

var _ = os.Stdout
