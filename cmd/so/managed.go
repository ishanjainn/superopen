package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ishanjainn/superopen/internal/paths"
)

// skipIfUnmanaged prints the init hint and returns true when root has no .so/.
// Callers should return nil so hooks and CLI stay fail-open (exit 0).
func skipIfUnmanaged(cmd *cobra.Command, root string) bool {
	if paths.Managed(root) {
		return false
	}
	fmt.Fprintln(cmd.OutOrStdout(), paths.UnmanagedMessage)
	return true
}
