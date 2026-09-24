package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/ishanjainn/superopen/internal/guards"
	"github.com/ishanjainn/superopen/internal/guards/rules"
	"github.com/ishanjainn/superopen/internal/paths"
)

func cmdScan() *cobra.Command {
	var rulesDir, minSeverity, sessionID, failOn string
	cmd := &cobra.Command{
		Use:          "scan",
		Short:        "Run detection rules over recorded tool calls",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root := repoRoot()
			layout := paths.Resolve(root)
			minRank := 0
			if minSeverity != "" {
				rank, ok := severityRank(minSeverity)
				if !ok {
					return fmt.Errorf("invalid --min-severity %q (info|low|medium|high|critical)", minSeverity)
				}
				minRank = rank
			}
			failRank := -1
			if failOn != "" {
				rank, ok := severityRank(failOn)
				if !ok {
					return fmt.Errorf("invalid --fail-on %q (info|low|medium|high|critical)", failOn)
				}
				failRank = rank
			}
			found, err := rules.ScanSessionDir(layout.SessionsDir, filepath.Join(layout.Root, "guards"), rulesDir, sessionID, minRank)
			if err != nil {
				return err
			}
			if found == nil {
				found = []guards.Finding{}
			}
			if err := out().HumanOrJSON("scan", func() {
				if len(found) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "no findings")
					return
				}
				for _, f := range found {
					fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n", f.Severity, f.RuleID, f.SessionID, f.Reason)
				}
			}, found); err != nil {
				return err
			}
			if failRank >= 0 && guards.CountAtOrAbove(found, failRank) > 0 {
				return fmt.Errorf("scan: %d finding(s) at or above %s", guards.CountAtOrAbove(found, failRank), failOn)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&rulesDir, "rules", "", "Rule directory to scan with, instead of the stored rules")
	cmd.Flags().StringVar(&minSeverity, "min-severity", "", "Drop findings below this severity")
	cmd.Flags().StringVar(&sessionID, "session", "", "Scan one session id")
	cmd.Flags().StringVar(&failOn, "fail-on", "", "Exit with an error when a finding is at or above this severity")
	return cmd
}

func severityRank(name string) (int, bool) {
	switch name {
	case "info", "low", "medium", "high", "critical":
		return guards.SeverityRank(guards.Severity(name)), true
	default:
		return 0, false
	}
}
