package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/axi"
	"github.com/ishanjainn/superopen/internal/port"
	"github.com/ishanjainn/superopen/internal/port/adapters"
)

func newPortOrchestrator() *port.Orchestrator {
	root := repoRoot()
	paths := harness.Resolve(root)
	reg := port.NewRegistry()
	adapters.RegisterAll(reg, root)
	return &port.Orchestrator{
		Reg:      reg,
		Ledger:   port.NewLedger(port.DefaultLedgerPath(paths.Root)),
		RepoRoot: root,
	}
}

func parseHarness(s string) (port.HarnessID, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "claude", "claude-code", "claudecode":
		return port.HarnessClaude, nil
	case "codex":
		return port.HarnessCodex, nil
	case "opencode", "open-code":
		return port.HarnessOpenCode, nil
	case "cursor":
		return port.HarnessCursor, nil
	case "pi":
		return port.HarnessPi, nil
	case "so", "sohub", ".so", "hub":
		return port.HarnessSOHub, nil
	default:
		return "", fmt.Errorf("unknown harness %q (claude|codex|opencode|cursor|pi|so)", s)
	}
}

func attachSessionsPort(sessions *cobra.Command) {

	detect := &cobra.Command{
		Use:   "detect",
		Short: "Detect available coding-agent session stores",
		RunE: func(cmd *cobra.Command, args []string) error {
			o := newPortOrchestrator()
			det := o.Detect()
			return out().HumanOrJSON("result", func() {
				for id, caps := range det {
					fmt.Printf("%-10s  import=%v  export=%v\n", id, caps["import"], caps["export"])
				}
			}, det)
		},
	}

	portCmd := &cobra.Command{
		Use:   "port",
		Short: "Port sessions between coding agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			fromS, _ := cmd.Flags().GetString("from")
			toS, _ := cmd.Flags().GetString("to")
			all, _ := cmd.Flags().GetBool("all")
			force, _ := cmd.Flags().GetBool("force")
			preview, _ := cmd.Flags().GetBool("preview")
			ids, _ := cmd.Flags().GetStringSlice("id")
			from, err := parseHarness(fromS)
			if err != nil {
				return axi.Err(err)
			}
			to, err := parseHarness(toS)
			if err != nil {
				return axi.Err(err)
			}
			if !preview && !all && len(ids) == 0 {
				return axi.Err(fmt.Errorf("select sessions with --id or --all (or use --preview)"))
			}
			o := newPortOrchestrator()
			res, err := o.Port(port.PortOptions{
				From: from, To: to, IDs: ids, All: all, Force: force, Preview: preview,
			})
			if err != nil {
				return axi.Err(err)
			}
			return out().HumanOrJSON("result", func() {
				if preview {
					fmt.Printf("preview %s → %s (%d sessions)\n", from, to, len(res.Refs))
					for _, r := range res.Refs {
						mark := " "
						if r.Imported {
							mark = "✓"
						}
						if r.SourceChanged {
							mark = "Δ"
						}
						fmt.Printf("  %s %s  %s\n", mark, short(r.SourceSessionID), firstLine(r.Title))
					}
					return
				}
				fmt.Printf("ported=%d skipped=%d failed=%d\n", res.Ported, res.Skipped, res.Failed)
				if res.DroppedTurns > 0 {
					fmt.Printf("dropped %d non-text turns (tool calls, results, reasoning)", res.DroppedTurns)
					if res.WorkingStateSessions > 0 {
						fmt.Printf(" — recovered working state for %d/%d sessions", res.WorkingStateSessions, res.Ported)
					}
					fmt.Println()
				}
				if res.ResumeArmed && res.ResumeID != "" {
					fmt.Printf("resume armed: next %s SessionStart injects %s\n", to, res.ResumeID)
				}
				for _, e := range res.Events {
					if e.Type == "error" {
						fmt.Fprintf(os.Stderr, "error %s: %s\n", e.ID, e.Error)
					}
				}
			}, res)
		},
	}
	portCmd.Flags().String("from", "", "Source harness (claude|codex|opencode|cursor|pi|so)")
	portCmd.Flags().String("to", "", "Destination harness")
	portCmd.Flags().StringSlice("id", nil, "Source session id(s)")
	portCmd.Flags().Bool("all", false, "Port all discovered sessions")
	portCmd.Flags().Bool("force", false, "Force rewrite even if ledger hit")
	portCmd.Flags().Bool("preview", false, "Discover and list only")
	_ = portCmd.MarkFlagRequired("from")
	_ = portCmd.MarkFlagRequired("to")

	verify := &cobra.Command{
		Use:   "verify",
		Short: "Sample-parse sessions and optionally round-trip via .so hub",
		RunE: func(cmd *cobra.Command, args []string) error {
			fromS, _ := cmd.Flags().GetString("from")
			toS, _ := cmd.Flags().GetString("to")
			sample, _ := cmd.Flags().GetInt("sample")
			from, err := parseHarness(fromS)
			if err != nil {
				return axi.Err(err)
			}
			to := port.HarnessSOHub
			if toS != "" {
				to, err = parseHarness(toS)
				if err != nil {
					return axi.Err(err)
				}
			}
			o := newPortOrchestrator()
			res, err := o.Verify(from, to, sample, func(repoRoot string) (port.ImportAdapter, port.ExportAdapter) {
				return adapters.SOHubImport{RepoRoot: repoRoot}, adapters.SOHubExport{RepoRoot: repoRoot}
			})
			if err != nil {
				return axi.Err(err)
			}
			return out().HumanOrJSON("result", func() {
				fmt.Printf("verify %s: sampled=%d ok=%d failed=%d\n", from, res.Sampled, res.OK, res.Failed)
				for _, d := range res.Details {
					fmt.Println(" ", d)
				}
			}, res)
		},
	}
	verify.Flags().String("from", "", "Source harness")
	verify.Flags().String("to", "so", "Optional dest (default so hub round-trip)")
	verify.Flags().Int("sample", 3, "Number of sessions to sample")
	_ = verify.MarkFlagRequired("from")

	sessions.AddCommand(detect, portCmd, verify)
}
