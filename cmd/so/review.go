package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ishanjainn/superopen/internal/agentcli"
	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/eval"
	"github.com/ishanjainn/superopen/internal/execx"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/harnessvalid"
	"github.com/ishanjainn/superopen/internal/harvest"
	"github.com/ishanjainn/superopen/internal/learn"
	"github.com/ishanjainn/superopen/internal/llm"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/recommend"
	"github.com/ishanjainn/superopen/internal/session"
	"github.com/ishanjainn/superopen/internal/sync"
	"github.com/ishanjainn/superopen/internal/tracestore"
)

func cmdReviewBrief() *cobra.Command {
	return &cobra.Command{
		Use:   "review-brief [session-id]",
		Short: "Print the live-agent session-review prompt without creating a file",
		RunE: func(cmd *cobra.Command, args []string) error {
			root := repoRoot()
			paths := harness.Resolve(root)
			if !paths.Exists() {
				return fmt.Errorf("run so init first")
			}
			cfg, _ := config.Load(paths.Config)
			applyTracesDir(root, &paths, cfg)
			id, err := resolveReviewSessionID(paths, args)
			if err != nil {
				return err
			}
			doc, _ := session.NewStore(paths).ReadDocument(id)
			if doc.Review.Status == "complete" {
				fmt.Printf("session %s review status=complete — skip apply-review\n", id)
				return nil
			}
			if doc.Review.Status == "running" {
				fmt.Printf("session %s review status=running — skip apply-review\n", id)
				return nil
			}
			store := tracestore.NewLocalJSONL(paths.TracesDir)
			spans, _ := store.Query(tracestore.QueryFilter{SessionID: id})
			brief, err := eval.BuildBrief(paths, id, "", spans)
			if err != nil {
				return err
			}
			fmt.Print(brief.System)
			fmt.Print("\n\n")
			fmt.Println(brief.User)
			return nil
		},
	}
}

func cmdApplyReview() *cobra.Command {
	return &cobra.Command{
		Use:   "apply-review [session-id] [file|-]",
		Short: "Apply assistant-produced session review JSON",
		Long:  "Reads reviewer JSON (from the live coding agent) and writes evals, recommendations, and memory. Pass a file path OR stdin — never both. Session id defaults to the previous pending same-vendor session, else the latest session.",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := repoRoot()
			paths := harness.Resolve(root)
			if !paths.Exists() {
				return fmt.Errorf("run so init first")
			}
			id := ""
			fileArg := ""
			switch len(args) {
			case 1:
				if strings.HasSuffix(strings.ToLower(args[0]), ".json") || args[0] == "-" {
					fileArg = args[0]
				} else {
					id = args[0]
				}
			case 2:
				id, fileArg = args[0], args[1]
			}
			cfg, _ := config.Load(paths.Config)
			applyTracesDir(root, &paths, cfg)
			var err error
			if id == "" {
				id, err = resolveReviewSessionID(paths, nil)
				if err != nil {
					return err
				}
			}
			useFile := fileArg != "" && fileArg != "-"
			if useFile && stdinRedirected() {
				return fmt.Errorf("apply-review: pass a file OR stdin/heredoc, not both (got %q with redirected stdin)", fileArg)
			}
			var raw []byte
			if useFile {
				raw, err = os.ReadFile(fileArg)
			} else {
				raw, err = io.ReadAll(os.Stdin)
			}
			if err != nil {
				return err
			}
			if len(bytes.TrimSpace(raw)) == 0 {
				return fmt.Errorf("apply-review: empty JSON (write a file and pass its path, or pipe/heredoc JSON on stdin)")
			}
			meta, _ := session.NewStore(paths).Get(id)
			backend := "live_agent:" + harness.NormalizeVendorKind(meta.Vendor)
			res, err := applyReviewJSON(root, paths, cfg, id, backend, raw, false)
			if err != nil {
				return err
			}
			fmt.Printf("Applied review for session %s backend=%s badge=%s\n", id, res.Backend, res.Badge)
			return nil
		},
	}
}

func resolveReviewSessionID(paths harness.Paths, args []string) (string, error) {
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" && args[0] != "-" && !strings.HasSuffix(strings.ToLower(args[0]), ".json") {
		return strings.TrimSpace(args[0]), nil
	}
	entries, err := session.NewStore(paths).List()
	if err != nil || len(entries) == 0 {
		return "", fmt.Errorf("no sessions")
	}
	latest := entries[0]
	if prev := harvest.PendingVendor(paths, latest.ID, latest.Vendor); prev != "" {
		return prev, nil
	}
	return latest.ID, nil
}

func applyReviewJSON(root string, paths harness.Paths, cfg config.Config, sessionID, backend string, raw []byte, skipTracked bool) (eval.Result, error) {
	store := tracestore.NewLocalJSONL(paths.TracesDir)
	spans, _ := store.Query(tracestore.QueryFilter{SessionID: sessionID})
	sess := session.NewStore(paths)
	doc, _ := sess.ReadDocument(sessionID)
	if doc.Review.Status == "complete" {
		if prior, ok := eval.LatestResult(paths, sessionID); ok {
			return prior, nil
		}
	}
	release, claimed := sess.ClaimReview(sessionID, "apply-review")
	if !claimed {
		return eval.Result{}, fmt.Errorf("session %s review is already running", sessionID)
	}
	var retErr error
	defer func() {
		release()
		if retErr != nil {
			_ = sess.WriteDocument(sessionID, func(d *session.Document) {
				d.Review.Status = "failed"
				d.Review.Error = retErr.Error()
			})
		}
	}()
	res, err := eval.ApplyJSON(paths, cfg, sessionID, backend, raw, spans)
	if err != nil {
		retErr = err
		return res, err
	}
	if err := persistReviewSideEffects(root, paths, cfg, sessionID, res, skipTracked); err != nil {
		retErr = err
		return res, err
	}
	return res, nil
}

func applyReviewWithCompleter(root string, paths harness.Paths, cfg config.Config, sessionID string, completer llm.Completer, skipTracked bool) (eval.Result, error) {
	store := tracestore.NewLocalJSONL(paths.TracesDir)
	spans, _ := store.Query(tracestore.QueryFilter{SessionID: sessionID})
	sess := session.NewStore(paths)
	doc, _ := sess.ReadDocument(sessionID)
	if doc.Review.Status == "complete" {
		if prior, ok := eval.LatestResult(paths, sessionID); ok {
			return prior, nil
		}
	}
	release, claimed := sess.ClaimReview(sessionID, "cli-review")
	if !claimed {
		return eval.Result{}, fmt.Errorf("session %s review is already running", sessionID)
	}
	var retErr error
	defer func() {
		release()
		if retErr != nil {
			_ = sess.WriteDocument(sessionID, func(d *session.Document) {
				d.Review.Status = "failed"
				d.Review.Error = retErr.Error()
			})
		}
	}()
	res, err := eval.Run(paths, cfg, sessionID, spans, completer, eval.RunOptions{Final: true})
	if err != nil {
		retErr = err
		return res, err
	}
	if !res.CompleteReview {
		retErr = fmt.Errorf("no model reviewer available")
		return res, retErr
	}
	if err := persistReviewSideEffects(root, paths, cfg, sessionID, res, skipTracked); err != nil {
		retErr = err
		return res, err
	}
	return res, nil
}

func persistReviewSideEffects(root string, paths harness.Paths, cfg config.Config, sessionID string, ev eval.Result, skipTracked bool) error {
	meta, _ := session.NewStore(paths).Get(sessionID)
	if cfg.MemoryEnabled() {
		mem := memory.NewStore(paths)
		_ = recommend.RecordFindings(paths, sessionID, meta.Vendor, ev.Findings)
		for _, lesson := range ev.Memory.Lessons {
			_ = mem.AddLesson(memory.Lesson{Text: lesson, Scope: "workspace", Confidence: 0.8, SourceSession: sessionID}, memory.ModePersistent)
		}
		if strings.TrimSpace(ev.Memory.Preference) != "" {
			_ = mem.AppendPreferenceText(ev.Memory.Preference)
		}
		if strings.TrimSpace(ev.Memory.ProjectNote) != "" {
			_ = mem.AppendProjectNote(ev.Memory.ProjectNote)
		}
	}
	if cfg.Recommendations.Auto {
		recs, _ := recommend.Generate(paths, sessionID, ev, nil)
		if !skipTracked {
			appliedAny := false
			for _, r := range recs {
				tier := harnessvalid.Tier(r.Type)
				if !cfg.AllowsAutoApplyTier(tier) {
					continue
				}
				allowed, reason := recommend.ShouldAutoApply(paths, r)
				if !allowed {
					continue
				}
				if err := recommend.Apply(paths, r.ID, recommend.Decision{
					Reason: "Automatically applied: " + reason + ".",
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
	_, _, _ = learn.MineSessionFile(paths, sessionID, nil)
	_, _ = harvest.Run(paths, cfg, sessionID, harvest.TriggerFinalize, harvest.RunOpts{
		SkipNativeDocs: skipTracked,
		LocalOnly:      true,
	})
	return nil
}

func maybeSpawnCLIReview(root, sessionID string, cfg config.Config) {
	if !cfg.CLIFallbackEnabled() || cfg.ExplicitHeuristics() {
		return
	}
	if len(agentcli.DetectAll()) == 0 {
		return
	}
	execx.SpawnSO(root, "sessions", "review", sessionID)
}

func reviewSessionCLI(root, sessionID string, skipTracked bool) error {
	paths := harness.Resolve(root)
	cfg, _ := config.Load(paths.Config)
	applyTracesDir(root, &paths, cfg)
	if sessionID == "" {
		var err error
		sessionID, err = resolveReviewSessionID(paths, nil)
		if err != nil {
			return err
		}
	}
	meta, _ := session.NewStore(paths).Get(sessionID)
	client := llm.NewVendorCompleter(cfg, meta.Vendor)
	if client == nil || !client.Available() {
		fmt.Printf("Session %s review still pending (no sealed CLI); next live agent can apply-review\n", sessionID)
		return nil
	}
	res, err := applyReviewWithCompleter(root, paths, cfg, sessionID, client, skipTracked)
	if err != nil {
		fmt.Fprintf(os.Stderr, "review: %v\n", err)
		return nil
	}
	fmt.Printf("Reviewed session %s backend=%s badge=%s\n", sessionID, res.Backend, res.Badge)
	return nil
}
