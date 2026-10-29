// Package version exposes the CLI build version and a `version` subcommand.
package version

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/ishanjainn/superopen/internal/cli"
)

// Version is the CLI's semver string without a leading "v" (e.g. "0.1.0").
// Overridden at release build time via:
//
//	-ldflags "-X github.com/ishanjainn/superopen/internal/version.Version=1.2.3"
//
// Keep the default in sync with /VERSION at the repo root.
var Version = "0.8.0"

// Commit is the short commit SHA, set at build time via -ldflags.
var Commit = ""

// Display returns Version without a leading "v" (e.g. "0.1.0").
func Display() string {
	v := strings.TrimSpace(Version)
	if v == "" {
		v = "0.1.0"
	}
	return strings.TrimPrefix(v, "v")
}

// NewCmd returns the cobra `version` subcommand.
func NewCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "version",
		Short:         "Print the so CLI version",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			o := cli.NewFromCmd(cmd)
			data := map[string]any{"version": Display()}
			if Commit != "" {
				data["commit"] = Commit
			}
			o.Next("so --help")
			o.Object("version", data)
			return nil
		},
	}
}
