// Package coding hosts the `so coding ...` subcommand group.
//
// v1 ships the following children:
//
//	so coding hook      --vendor=cc|cursor|codex --event=...
//	so coding install   --vendor=all|<single>
//	so coding uninstall --vendor=all|<single> [--purge]
//	so coding launch    <claude|cursor|codex>
//
// All children share the resolved config from internal/config and the
// OTLP exporter from internal/codingotlp. The hook subcommand is the hot path
// invoked once per agent event and follows the crash-isolation rules
// documented in cmd/so (crash-isolation / hard timeout).
package coding

import (
	"github.com/superopen/so/internal/coding/hook"
	"github.com/superopen/so/internal/coding/install"
	"github.com/superopen/so/internal/coding/launch"
	"github.com/superopen/so/internal/coding/uninstall"
	"github.com/spf13/cobra"
)

// NewCmd returns the `coding` cobra command tree.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "coding",
		Short: "Coding-agent observability (Claude Code, Cursor, Codex)",
		Long: `Send telemetry from AI coding agents into Superopen.

Three install paths land at the same plugin manifests under plugins/:
  A) so coding launch <claude|cursor|codex>             # one-liner
  B) so coding install --vendor=all                     # write manifests, no agent TUI
  C) From inside the agent: plugin marketplace flows for Claude Code / Codex

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
	cmd.AddCommand(launch.NewCmd())

	return cmd
}
