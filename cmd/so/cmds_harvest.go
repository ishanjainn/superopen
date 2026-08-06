package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superopen/so/internal/config"
	"github.com/superopen/so/internal/harness"
	"github.com/superopen/so/internal/harvest"
	"github.com/superopen/so/internal/axi"
	"github.com/superopen/so/internal/session"
)

func cmdHarvest() *cobra.Command {
	c := &cobra.Command{
		Use:   "harvest",
		Short: "Budgeted session harvest into memory + knowledge/rules/skills",
		Long: `Summarize a session locally, then one small coding-agent CLI call for JSON deltas.
Triggers: session end, idle (≥ memory.idle_harvest_hours), or explicit finalize.`,
	}

	runOne := &cobra.Command{
		Use:   "run [session-id]",
		Short: "Harvest one session (default: latest)",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := harness.Resolve(repoRoot())
			cfg, _ := config.Load(paths.Config)
			sid := ""
			if len(args) > 0 {
				sid = args[0]
			}
			if sid == "" {
				entries, err := session.NewStore(paths).List()
				if err != nil || len(entries) == 0 {
					return axi.Err(fmt.Errorf("no sessions to harvest"))
				}
				sid = entries[0].ID
			}
			trigger := harvest.TriggerFinalize
			if t, _ := cmd.Flags().GetString("trigger"); t != "" {
				trigger = harvest.Trigger(t)
			}
			res, err := harvest.Run(paths, cfg, sid, trigger)
			if err != nil {
				return axi.Err(err)
			}
			return out().HumanOrJSON("result", func() {
				if res.Skipped {
					fmt.Printf("harvest skipped (%s): %s\n", res.SessionID, res.Reason)
					return
				}
				fmt.Printf("harvested %s applied=%d recs=%d\n", res.SessionID, res.Applied, res.Recs)
			}, res)
		},
	}
	runOne.Flags().String("trigger", "finalize", "session_end|idle|finalize")
	c.AddCommand(runOne)

	idle := &cobra.Command{
		Use:   "idle",
		Short: "Harvest open sessions idle ≥ memory.idle_harvest_hours",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := harness.Resolve(repoRoot())
			cfg, _ := config.Load(paths.Config)
			results, err := harvest.IdleSweep(paths, cfg)
			if err != nil {
				return axi.Err(err)
			}
			return out().HumanOrJSON("result", func() {
				fmt.Printf("idle harvest: %d session(s)\n", len(results))
				for _, r := range results {
					if r.Skipped {
						fmt.Printf("  skip %s (%s)\n", r.SessionID, r.Reason)
						continue
					}
					fmt.Printf("  ok %s applied=%d\n", r.SessionID, r.Applied)
				}
			}, results)
		},
	}
	c.AddCommand(idle)
	return c
}
