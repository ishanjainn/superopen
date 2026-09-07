package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ishanjainn/superopen/internal/cli"
	"github.com/ishanjainn/superopen/internal/graph/engine"
	"github.com/ishanjainn/superopen/internal/paths"
)

func skipIfUnmanaged(cmd *cobra.Command, root string) bool {
	engine.SeedLinkedWorktree(root)
	if paths.Managed(root) {
		return false
	}
	fmt.Fprintln(cmd.OutOrStdout(), paths.UnmanagedMessage)
	return true
}

func graphNoRefresh(cmd *cobra.Command) bool {
	if engine.RefreshDisabled() {
		return true
	}
	if cmd == nil {
		return false
	}
	v, err := cmd.Flags().GetBool("no-refresh")
	if err == nil && v {
		return true
	}
	if cmd.Parent() != nil {
		v, err = cmd.Parent().PersistentFlags().GetBool("no-refresh")
		return err == nil && v
	}
	return false
}

func ensureFreshGraph(cmd *cobra.Command, root string) string {
	if graphNoRefresh(cmd) || !paths.Managed(root) {
		return ""
	}
	dirty, err := engine.ProbeDirty(cmd.Context(), root, nil)
	if err != nil || !dirty {
		return ""
	}
	changes, err := engine.PlanIncrementalFromProbe(cmd.Context(), root, "", nil)
	if err != nil || changes.RequiresFull {
		return engine.GraphStaleHeader
	}
	if !engine.BuildBusy(root) && !engine.BuildPoolFull() {
		cli.SpawnSO(root, "--root", root, "graph", "refresh", "--probe")
	}
	if engine.WaitUntilFresh(cmd.Context(), root, nil, engine.QueryRefreshWait) {
		return ""
	}
	return engine.GraphStaleHeader
}
