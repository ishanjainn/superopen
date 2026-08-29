package main

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/ishanjainn/superopen/internal/cli"
	"github.com/ishanjainn/superopen/internal/harvest"
)

func cmdHarvest() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "harvest",
		Short: "Stage playbook patches from coding sessions until you approve them",
	}
	cmd.AddCommand(
		harvestInventoryCmd(),
		harvestProposeCmd(),
		harvestBriefCmd(),
		harvestScanCmd(),
		harvestSkipCmd(),
		harvestListCmd(),
		harvestShowCmd(),
		harvestApplyCmd(),
		harvestDeclineCmd(),
		harvestReviewCmd(),
	)
	return cmd
}

func harvestInventoryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "inventory",
		Short: "List playbook files (hash, protected)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			root := repoRoot()
			if skipIfUnmanaged(cmd, root) {
				return nil
			}
			files, err := harvest.Inventory(root)
			if err != nil {
				return err
			}
			rows := make([]map[string]any, 0, len(files))
			for _, f := range files {
				rows = append(rows, map[string]any{
					"path": f.Path, "hash": f.Hash, "bytes": f.Bytes, "protected": f.Protected,
				})
			}
			if len(rows) == 0 {
				out().Empty("playbooks")
				return nil
			}
			out().Next("so harvest scan [session]")
			out().Rows("playbooks", []string{"path", "hash", "bytes", "protected"}, rows)
			return nil
		},
	}
}

func harvestProposeCmd() *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "propose",
		Short: "Ingest JSON proposals from stdin or --file (live wrap-up / headless)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			root := repoRoot()
			if skipIfUnmanaged(cmd, root) {
				return nil
			}
			var raw []byte
			var err error
			if file != "" {
				raw, err = os.ReadFile(file)
			} else {
				raw, err = io.ReadAll(cmd.InOrStdin())
			}
			if err != nil {
				return err
			}
			items, err := harvest.ProposeJSON(root, raw)
			if err != nil {
				return err
			}
			return out().HumanOrJSON("harvest_propose", func() {
				for _, p := range items {
					fmt.Fprintf(cmd.OutOrStdout(), "#%d %s %s %s\n", p.ID, p.Status, p.Kind, p.Title)
				}
			}, items)
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "JSON file instead of stdin")
	return cmd
}

func harvestScanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scan [session]",
		Short: "Run skip gates then at most one bounded generate on the session's own CLI",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := repoRoot()
			if skipIfUnmanaged(cmd, root) {
				return nil
			}
			id := ""
			if len(args) == 1 {
				id = args[0]
			}
			res := harvest.Generate(root, id)
			return out().HumanOrJSON("harvest_scan", func() {
				if res.Skipped != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "skipped %s %s\n", res.SessionID, res.Skipped)
					return
				}
				fmt.Fprintf(cmd.OutOrStdout(), "session %s provider=%s inserted=%d\n", res.SessionID, res.Provider, res.Inserted)
			}, res)
		},
	}
	return cmd
}

func harvestBriefCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "brief [session]",
		Short: "Print the harvest prompt for the live agent (pending session if omitted)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := repoRoot()
			if skipIfUnmanaged(cmd, root) {
				return nil
			}
			id := ""
			if len(args) == 1 {
				id = args[0]
			}
			text, err := harvest.Brief(root, id)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), text)
			return nil
		},
	}
}

func harvestSkipCmd() *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "skip <session>",
		Short: "Close a pending harvest with nothing to propose",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := repoRoot()
			if skipIfUnmanaged(cmd, root) {
				return nil
			}
			if err := harvest.Skip(root, args[0], reason); err != nil {
				return err
			}
			return out().HumanOrJSON("harvest_skip", func() {
				fmt.Fprintf(cmd.OutOrStdout(), "skipped %s\n", args[0])
			}, map[string]any{"session_id": args[0], "status": "skipped"})
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "Why nothing was proposed")
	return cmd
}

func harvestListCmd() *cobra.Command {
	var status string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List harvest proposals (default: open)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			root := repoRoot()
			if skipIfUnmanaged(cmd, root) {
				return nil
			}
			store, err := harvest.OpenRoot(root)
			if err != nil {
				return err
			}
			defer store.Close()
			items, err := store.List(status)
			if err != nil {
				return err
			}
			rows := make([]map[string]any, 0, len(items))
			for _, p := range items {
				rows = append(rows, map[string]any{
					"id": p.ID, "status": p.Status, "kind": p.Kind, "target": p.Target,
					"title": p.Title, "plus": p.Plus, "minus": p.Minus, "session": p.SessionID,
				})
			}
			if len(rows) == 0 {
				out().Empty("proposals")
				return nil
			}
			out().Next("so harvest show <id>", "so harvest apply <id>", "so harvest decline <id>")
			out().Rows("proposals", []string{"id", "status", "kind", "target", "title", "plus", "minus"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "Filter status (open|applied|declined|noop|stale)")
	return cmd
}

func harvestShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show reason, evidence, and diff",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := repoRoot()
			if skipIfUnmanaged(cmd, root) {
				return nil
			}
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return cli.Usage("id must be a number", "so harvest list")
			}
			store, err := harvest.OpenRoot(root)
			if err != nil {
				return err
			}
			defer store.Close()
			p, err := store.GetProposal(id)
			if err != nil {
				return cli.NotFound("proposal not found", "so harvest list")
			}
			return out().HumanOrJSON("proposal", func() {
				fmt.Fprintf(cmd.OutOrStdout(), "#%d %s %s %s\n", p.ID, p.Status, p.Kind, p.Target)
				fmt.Fprintf(cmd.OutOrStdout(), "title: %s\n", p.Title)
				fmt.Fprintf(cmd.OutOrStdout(), "reason: %s\n", p.Reason)
				if p.SessionID != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "session: %s\n", p.SessionID)
				}
				for _, e := range p.Evidence {
					fmt.Fprintf(cmd.OutOrStdout(), "evidence: %s %s %s\n", e.Kind, e.ID, e.Label)
				}
				if p.Diff != "" {
					fmt.Fprintln(cmd.OutOrStdout(), p.Diff)
				}
			}, p)
		},
	}
}

func harvestApplyCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "apply <id>",
		Short: "Write an approved proposal to the live playbook",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := repoRoot()
			if skipIfUnmanaged(cmd, root) {
				return nil
			}
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return cli.Usage("id must be a number", "so harvest list")
			}
			p, err := harvest.Apply(root, id, force)
			if err != nil {
				return err
			}
			return out().HumanOrJSON("harvest_apply", func() {
				fmt.Fprintf(cmd.OutOrStdout(), "applied #%d %s %s\n", p.ID, p.Kind, p.Target)
			}, p)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Allow simplify/create/non-additive patches")
	return cmd
}

func harvestDeclineCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "decline <id>",
		Short: "Reject a proposal",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := repoRoot()
			if skipIfUnmanaged(cmd, root) {
				return nil
			}
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return cli.Usage("id must be a number", "so harvest list")
			}
			p, err := harvest.Decline(root, id)
			if err != nil {
				return err
			}
			return out().HumanOrJSON("harvest_decline", func() {
				fmt.Fprintf(cmd.OutOrStdout(), "declined #%d\n", p.ID)
			}, p)
		},
	}
}

func harvestReviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "review",
		Short: "Compact OPEN pack (load only when asked)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			root := repoRoot()
			if skipIfUnmanaged(cmd, root) {
				return nil
			}
			text, items, err := harvest.Review(root)
			if err != nil {
				return err
			}
			return out().HumanOrJSON("harvest_review", func() {
				fmt.Fprintln(cmd.OutOrStdout(), text)
			}, items)
		},
	}
}
