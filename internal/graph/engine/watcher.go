package engine

import (
	"context"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

// WatchChanges plans incremental updates for a coalesced watcher batch. Full
// selective publication remains gated behind readiness; this helper only exposes
// the pinned change-set protocol.
func WatchChanges(ctx context.Context, repoRoot, project string, excludes []string) (api.ChangeSet, error) {
	return PlanIncremental(ctx, repoRoot, project, excludes)
}
