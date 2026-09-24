package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/ishanjainn/superopen/internal/cli"
	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/ishanjainn/superopen/internal/graph/client"
	"github.com/ishanjainn/superopen/internal/graph/engine"
	"github.com/ishanjainn/superopen/internal/paths"
)

func failIfUnmanaged(root string) error {
	engine.SeedLinkedWorktree(root)
	if paths.Managed(root) {
		return nil
	}
	return cli.Fail(cli.ExitFail, paths.UnmanagedMessage, "so init")
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

// readRefreshSlotWait is how long this repo waits for a build slot held by
// another repo. The refresh itself runs until it finishes or the command
// context ends. --no-refresh and SUPEROPEN_NO_REFRESH skip both.
const readRefreshSlotWait = 3 * time.Minute

func ensureFreshGraph(cmd *cobra.Command, root string) string {
	if graphNoRefresh(cmd) || !paths.Managed(root) {
		return ""
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	dirty, err := engine.ProbeDirty(ctx, root, nil)
	if err != nil || !dirty {
		return ""
	}
	if err := refreshGraphForRead(ctx, root); err == nil {
		dirty, err = engine.ProbeDirty(ctx, root, nil)
		if err == nil && !dirty {
			return ""
		}
	}
	changes, planErr := engine.PlanIncrementalFromProbe(ctx, root, "", nil)
	if planErr != nil {
		return engine.GraphStaleHeader
	}
	return engine.StaleHeader(changes)
}

// refreshGraphForRead indexes root before a graph read. A build already
// running in another repo delays this one; it does not answer from the old index.
func refreshGraphForRead(ctx context.Context, root string) error {
	c, err := client.Resolve()
	if err != nil {
		return err
	}
	deadline := time.Now().Add(readRefreshSlotWait)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var result api.BuildResult
		err := c.Call(ctx, api.OpBuild, api.BuildRequest{
			RepoRoot: root, Mode: "full", Incremental: true, FromProbe: true,
		}, &result)
		if err != nil {
			return err
		}
		switch result.Status {
		case "ok", "unchanged":
			return nil
		case "pool_full", "refresh_in_progress":
			if !time.Now().Before(deadline) {
				return fmt.Errorf("graph refresh not finished")
			}
			if engine.WaitUntilFresh(ctx, root, nil, time.Second) {
				return nil
			}
			timer := time.NewTimer(200 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		default:
			return fmt.Errorf("graph refresh status %s", result.Status)
		}
	}
}
