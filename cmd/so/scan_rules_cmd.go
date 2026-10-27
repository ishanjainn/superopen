package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ishanjainn/superopen/internal/cli"
	"github.com/ishanjainn/superopen/internal/guards/rules"
	"github.com/ishanjainn/superopen/internal/paths"
)

// ruleView is a rule as `so scan rules` prints it and the Scan page loads it.
type ruleView struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Severity    string `json:"severity"`
	Status      string `json:"status"`
	Posture     string `json:"posture"`
	// Source is baseline (built in), store (custom) or override (a custom
	// rule replacing a built-in of the same id).
	Source  string `json:"source"`
	Enabled bool   `json:"enabled"`
	YAML    string `json:"yaml"`
}

func viewOf(e rules.Entry) ruleView {
	return ruleView{
		ID: e.Rule.ID, Title: e.Rule.Title, Description: e.Rule.Description,
		Severity: string(e.Rule.Severity), Status: string(e.Rule.Status), Posture: string(e.Rule.Posture),
		Source: string(e.Source), Enabled: e.Enabled(), YAML: e.YAML,
	}
}

func ruleStore() string { return filepath.Join(paths.Resolve(repoRoot()).Root, "guards") }

func cmdScanRules() *cobra.Command {
	group := &cobra.Command{
		Use:   "rules",
		Short: "List, add, edit, disable and remove detection rules",
		Long: `Detection rules are the built-in set plus your own in .so/guards/*.rule.yaml.
A custom rule with a built-in's id replaces it; remove it to get the built-in back.
Disabled rules stay on file but are skipped by every scan. The Scan page in the UI
edits the same files.`,
		Args: cobra.NoArgs,
	}
	group.RunE = func(cmd *cobra.Command, _ []string) error {
		sub, _, _ := cmd.Find([]string{"list"})
		sub.SetContext(cmd.Context())
		return sub.RunE(sub, nil)
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "List every rule with its source and whether it is enabled",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			entries, err := rules.List(ruleStore())
			if err != nil {
				return err
			}
			views := make([]ruleView, 0, len(entries))
			rows := make([]map[string]any, 0, len(entries))
			for _, e := range entries {
				views = append(views, viewOf(e))
				rows = append(rows, map[string]any{"id": e.Rule.ID, "severity": e.Rule.Severity, "source": e.Source, "enabled": e.Enabled(), "title": e.Rule.Title})
			}
			out().Next("so scan rules show <id>", "so scan rules set <file>")
			return out().HumanOrJSONRows("rules", []string{"id", "severity", "source", "enabled", "title"}, rows, views)
		},
	}

	show := &cobra.Command{
		Use:   "show <id>",
		Short: "Print a rule's YAML",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			entry, err := rules.Get(ruleStore(), args[0])
			if err != nil {
				return cli.NotFound(err.Error(), "so scan rules list")
			}
			view := viewOf(entry)
			out().Next("so scan rules set <file>", "so scan rules disable "+view.ID)
			return out().HumanOrJSON("rule", func() {
				w := cmd.OutOrStdout()
				fmt.Fprintf(w, "id: %s\nsource: %s\nenabled: %t\nyaml: |\n", view.ID, view.Source, view.Enabled)
				for _, line := range strings.Split(strings.TrimRight(view.YAML, "\n"), "\n") {
					fmt.Fprintf(w, "  %s\n", line)
				}
			}, view)
		},
	}

	set := &cobra.Command{
		Use:   "set <file|->",
		Short: "Create a rule or replace the one with the same id, from a YAML file or stdin",
		Long: `Reads one rule, checks it (fields, expression, and that every test fixture gives its
stated verdict), and writes .so/guards/<id>.rule.yaml. Nothing is written when the
rule is invalid. A built-in id becomes an override.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := failIfUnmanaged(repoRoot()); err != nil {
				return err
			}
			var data []byte
			var err error
			if args[0] == "-" {
				data, err = io.ReadAll(cmd.InOrStdin())
			} else {
				data, err = os.ReadFile(args[0])
			}
			if err != nil {
				return err
			}
			entry, err := rules.Set(ruleStore(), data)
			if err != nil {
				return cli.Usage(err.Error(), "so scan rules show <id>  # for a working example")
			}
			view := viewOf(entry)
			out().Next("so scan")
			return out().HumanOrJSON("rule_saved", func() {
				fmt.Fprintf(cmd.OutOrStdout(), "saved: ok\nid: %s\nsource: %s\nenabled: %t\n", view.ID, view.Source, view.Enabled)
			}, view)
		},
	}

	remove := &cobra.Command{
		Use:   "remove <id>",
		Short: "Delete a custom rule; removing an override brings the built-in back",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := failIfUnmanaged(repoRoot()); err != nil {
				return err
			}
			if _, err := rules.Remove(ruleStore(), args[0]); err != nil {
				return cli.NotFound(err.Error(), "so scan rules list")
			}
			after, _ := rules.Get(ruleStore(), args[0])
			restored := after.Rule != nil
			return out().HumanOrJSON("rule_removed", func() {
				fmt.Fprintf(cmd.OutOrStdout(), "removed: ok\nid: %s\nbuilt_in_restored: %t\n", args[0], restored)
			}, map[string]any{"id": args[0], "built_in_restored": restored})
		},
	}

	toggle := func(use string, disable bool) *cobra.Command {
		verb, short := "enable", "Run a rule in scans again"
		if disable {
			verb, short = "disable", "Keep a rule on file but skip it in scans (works on built-ins too)"
		}
		return &cobra.Command{
			Use:   use,
			Short: short,
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := failIfUnmanaged(repoRoot()); err != nil {
					return err
				}
				entry, err := rules.SetDisabled(ruleStore(), args[0], disable)
				if err != nil {
					return cli.NotFound(err.Error(), "so scan rules list")
				}
				view := viewOf(entry)
				return out().HumanOrJSON("rule_"+verb, func() {
					fmt.Fprintf(cmd.OutOrStdout(), "%sd: ok\nid: %s\nenabled: %t\n", verb, view.ID, view.Enabled)
				}, view)
			},
		}
	}

	group.AddCommand(list, show, set, remove, toggle("enable <id>", false), toggle("disable <id>", true))
	return group
}
