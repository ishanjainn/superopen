package engine

import (
	"context"
	"io/fs"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func IndexIncremental(ctx context.Context, request api.IncrementalRequest, engineVersion string, assets fs.FS) (api.BuildResult, error) {
	changes := request.Changes
	req := request.BuildRequest
	req.Incremental = true

	if changes.RequiresFull || changes.RevisionChanged || changeVolume(changes) == 0 && !req.Force {
		// Empty change set with an existing DB → skip via IndexAllDevelopment's unchanged path.
		// RequiresFull / revision drift → full rebuild.
		if changeVolume(changes) == 0 && !changes.RequiresFull && !changes.RevisionChanged {
			req.Force = false
		} else {
			req.Force = true
		}
		return IndexAllDevelopment(ctx, req, engineVersion, assets)
	}

	// Content-hash planning already identified changed files. Until per-file
	// selective publication lands, converge through a forced full assemble so
	// the published graph matches the working tree exactly.
	req.Force = true
	result, err := IndexAllDevelopment(ctx, req, engineVersion, assets)
	if err != nil {
		return result, err
	}
	result.Changes = &changes
	return result, nil
}
