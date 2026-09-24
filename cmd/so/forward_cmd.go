package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ishanjainn/superopen/internal/forward"
)

func cmdForward() *cobra.Command {
	return &cobra.Command{
		Use:          "forward",
		Short:        "Copy new session events to a file or URL",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			n, err := forward.Run(repoRoot())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "forwarded %d line(s)\n", n)
			return nil
		},
	}
}
