package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ishanjainn/superopen/internal/retention"
)

func cmdGC() *cobra.Command {
	var show bool
	var sessionHours int
	var memoryHours int
	cmd := &cobra.Command{
		Use:   "gc",
		Short: "Apply retention: delete old sessions and unpinned memories",
		Long: `Delete session transcripts and unpinned memories older than the
configured retention (hours; default 168 = 7 days). 0 keeps that store forever.

Teachings, pins, and the code graph are never deleted by age.
Checkpoints live inside session folders and go with the session.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			settings, err := retention.LoadSettings()
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("sessions-hours") {
				settings.SessionHours = sessionHours
			}
			if cmd.Flags().Changed("memory-hours") {
				settings.MemoryHours = memoryHours
			}
			if cmd.Flags().Changed("sessions-hours") || cmd.Flags().Changed("memory-hours") {
				settings, err = retention.SaveSettings(settings)
				if err != nil {
					return err
				}
			}
			if show {
				return out().HumanOrJSON("retention", func() {
					fmt.Fprintf(cmd.OutOrStdout(), "sessions %dh\nmemories %dh\n(0 = keep forever; default 168h)\n", settings.SessionHours, settings.MemoryHours)
				}, settings)
			}
			root := repoRoot()
			if skipIfUnmanaged(cmd, root) {
				return nil
			}
			result, err := retention.Sweep(root)
			if err != nil {
				return err
			}
			result.SessionHours = settings.SessionHours
			result.MemoryHours = settings.MemoryHours
			return out().HumanOrJSON("gc", func() {
				if len(result.SessionsDeleted) == 0 && result.MemoriesDeleted == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "0 expired")
					return
				}
				fmt.Fprintf(cmd.OutOrStdout(), "sessions=%d memories=%d\n", len(result.SessionsDeleted), result.MemoriesDeleted)
			}, result)
		},
	}
	cmd.Flags().BoolVar(&show, "show", false, "Print retention hours without deleting")
	cmd.Flags().IntVar(&sessionHours, "sessions-hours", 0, "Set session retention in hours (0 = forever)")
	cmd.Flags().IntVar(&memoryHours, "memory-hours", 0, "Set unpinned-memory retention in hours (0 = forever)")
	return cmd
}

func runRetentionSweep(root string) error {
	_, err := retention.Sweep(root)
	return err
}
