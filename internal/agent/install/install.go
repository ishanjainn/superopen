// Package install implements `so coding install --vendor=...`.
//
// Writes per-vendor host plugin manifests to the user's home directory
// so the agent (Claude Code, Cursor, Codex) finds them on next
// launch. The manifest payloads themselves live under internal/agent/install/marketplace/
// (mirrored from the repo-root `.claude-plugin/` + `plugins/` by
// `scripts/sync-plugins.sh`) and are embedded into the binary at
// build time, so a single statically-linked CLI carries everything.
package install

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// NewCmd returns the cobra command for `so coding install`.
func NewCmd() *cobra.Command {
	var (
		vendor string
		dryRun bool
	)

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install per-vendor coding-agent host plugin manifests",
		Long: `Install per-vendor host plugin manifests so coding agents pipe
telemetry through 'so coding hook'.

Vendors:
  claude-code   Plugin under ~/.claude/plugins/superopen/ + 'claude plugin install'
  cursor        Hook entries merged into ~/.cursor/hooks.json (user scope)
  codex         Marketplace + 'codex plugin add superopen@superopen'
  gemini        Hooks merged into ~/.gemini/settings.json
  opencode      Plugin under the OpenCode/XDG config directory
  copilot-cli   Hooks under COPILOT_HOME (default ~/.copilot)
  pi            Extension under ~/.pi/agent/extensions
  all           shorthand for every supported vendor

The 'so' binary must be on PATH. Install via Homebrew, the
prebuilt binaries on GitHub Releases, the curl|sh installer, or
'go install github.com/ishanjainn/superopen/cmd/so@latest'.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(cmd, vendor, dryRun)
		},
	}

	cmd.Flags().StringVar(&vendor, "vendor", "", "Vendor (claude-code | cursor | codex | gemini | opencode | copilot-cli | pi | all)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print what would be written without modifying any files")
	_ = cmd.MarkFlagRequired("vendor")

	return cmd
}

func run(cmd *cobra.Command, vendor string, dryRun bool) error {
	vendor = strings.ToLower(strings.TrimSpace(vendor))
	if vendor == "" {
		return errors.New("--vendor is required")
	}

	targets, err := vendorsFromArg(vendor)
	if err != nil {
		return err
	}

	for _, v := range targets {
		written, err := installVendor(v, dryRun)
		if err != nil {
			return fmt.Errorf("install %s: %w", v, err)
		}
		if dryRun {
			fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] would write %d file(s) for %s\n", len(written), v)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "so: installed %s plugin (%d file(s))\n", v, len(written))
		}
		for _, p := range written {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", p)
		}
		if !dryRun {
			if v == "codex" {
				fmt.Fprintln(cmd.OutOrStdout(), "")
				fmt.Fprintln(cmd.OutOrStdout(), "Next steps for Codex:")
				fmt.Fprintln(cmd.OutOrStdout(), "  1. Restart Codex (or run `codex` in a new shell).")
				fmt.Fprintln(cmd.OutOrStdout(), "  2. Inside Codex, run `/hooks` and trust each `superopen@superopen` entry.")
				fmt.Fprintln(cmd.OutOrStdout(), "  3. Start a session - turns show up under `so sessions` / `so dev`.")
			}
		}
	}
	return nil
}

// vendorsFromArg expands the --vendor argument into a slice of vendor IDs.
func vendorsFromArg(arg string) ([]string, error) {
	switch arg {
	case "all":
		return []string{"claude-code", "cursor", "codex", "gemini", "opencode", "copilot-cli", "pi"}, nil
	case "claude-code", "cc":
		return []string{"claude-code"}, nil
	case "cursor":
		return []string{"cursor"}, nil
	case "codex":
		return []string{"codex"}, nil
	case "gemini", "gemini-cli":
		return []string{"gemini"}, nil
	case "opencode", "open-code":
		return []string{"opencode"}, nil
	case "copilot", "copilot-cli":
		return []string{"copilot-cli"}, nil
	case "pi":
		return []string{"pi"}, nil
	default:
		return nil, fmt.Errorf("unknown --vendor %q", arg)
	}
}
