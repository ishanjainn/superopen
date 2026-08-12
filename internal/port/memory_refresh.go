package port

import (
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/memory"
)

// RefreshMemoryAfterPort updates episodic memory and context.md after a successful port.
func RefreshMemoryAfterPort(repoRoot, from, to, sourceID, title string) {
	if repoRoot == "" {
		return
	}
	paths := harness.Resolve(repoRoot)
	_ = memory.NewStore(paths).NotePort(from, to, sourceID, title)
}
