package port

import (
	"github.com/superopen/so/internal/harness"
	"github.com/superopen/so/internal/memory"
)

// RefreshMemoryAfterPort updates episodic memory and active-context.md after a successful port.
func RefreshMemoryAfterPort(repoRoot, from, to, sourceID, title string) {
	if repoRoot == "" {
		return
	}
	paths := harness.Resolve(repoRoot)
	_ = memory.NewStore(paths).NotePort(from, to, sourceID, title)
}
