package main

import (
	"context"
	"errors"
	"fmt"
	"os"
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
	return cli.Skip("not a Superopen repo")
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
// another repo. --no-refresh and SUPEROPEN_NO_REFRESH skip the refresh.
const readRefreshSlotWait = 3 * time.Minute

// readRefreshBudget bounds how long a read blocks on the refresh. Agent
// shells give up on a silent command at about 30 s (Cursor backgrounds it and
// the agent sees empty stdout), so a slow refresh finishes in the background
// and this read answers from the previous index with a stale header.
var readRefreshBudget = 12 * time.Second

func ensureFreshGraph(cmd *cobra.Command, root string) (string, error) {
	if graphNoRefresh(cmd) || !paths.Managed(root) {
		return "", nil
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	dirty, err := engine.ProbeDirty(ctx, root, nil)
	if err != nil || !dirty {
		return "", nil
	}
	// The first build of a repository has no previous index to answer from, so
	// that read waits for it instead of timing out into an error.
	refreshCtx, cancel := context.WithCancel(ctx)
	if indexExists(root) {
		refreshCtx, cancel = context.WithTimeout(ctx, readRefreshBudget)
	}
	err = refreshGraphForRead(refreshCtx, root)
	timedOut := ctx.Err() == nil && errors.Is(refreshCtx.Err(), context.DeadlineExceeded)
	cancel()
	if err != nil {
		// The engine runs in-process and flattens errors to text, so the
		// deadline is read from the context, not from err.
		if !timedOut {
			return "", err
		}
		if !engine.BuildBusy(root) {
			cli.SpawnSO(root, "--root", root, "graph", "refresh", "--probe")
		}
	}
	dirty, err = engine.ProbeDirty(ctx, root, nil)
	if err == nil && !dirty {
		return "", nil
	}
	changes, planErr := engine.PlanIncrementalFromProbe(ctx, root, "", nil)
	if planErr != nil {
		return engine.GraphStaleHeader, nil
	}
	return engine.StaleHeader(changes), nil
}

// refreshGraphForRead indexes root before a graph read. A build already
// running in another repo delays this one, up to the caller's deadline.
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

// indexExists is true when root already has a graph database.
func indexExists(root string) bool {
	paths, err := engine.CachePaths(root)
	if err != nil {
		return false
	}
	_, err = os.Stat(paths.Database)
	return err == nil
}
