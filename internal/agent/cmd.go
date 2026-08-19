// Package agent hosts the `so coding ...` subcommand group (agent hooks,
// install, and uninstall). The CLI command name stays `coding` for compatibility
// with existing agent plugin manifests.
package agent

import (
	"github.com/ishanjainn/superopen/internal/agent/hook"
	"github.com/ishanjainn/superopen/internal/agent/install"
	"github.com/ishanjainn/superopen/internal/agent/uninstall"
	"github.com/spf13/cobra"
)

// NewCmd returns the `coding` cobra command tree.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "coding",
		Short: "Coding-agent observability (Claude, Cursor, Codex, Gemini, OpenCode, Copilot, Pi)",
		Long: `Send telemetry from AI coding agents into Superopen.

Install paths:
  - so coding install --vendor=all
  - From inside the agent: plugin marketplace flows for Claude Code / Codex

To stop tracking, use 'so coding uninstall --vendor=<v>' (add
--purge to also drop ~/.config/superopen and the session-state cache).

The 'hook' subcommand is invoked by the host plugin manifests once per
agent event and is the hot path. It always exits 0 on telemetry-path
failure so a broken pipeline never blocks a developer's prompt.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(hook.NewCmd())
	cmd.AddCommand(install.NewCmd())
	cmd.AddCommand(uninstall.NewCmd())
	return cmd
}
