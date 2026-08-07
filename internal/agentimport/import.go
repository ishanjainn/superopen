// Package agentimport imports vendor transcript history into .so/sessions.
// Deprecated: prefer `so sessions port --from <harness> --to so --all`.
// These helpers remain as thin wrappers around the port adapters.
package agentimport

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/port"
	"github.com/ishanjainn/superopen/internal/port/adapters"
)

// ClaudeCode imports Claude Code sessions into the .so hub via the port orchestrator.
func ClaudeCode(paths harness.Paths) (int, error) {
	return portToHub(paths, port.HarnessClaude)
}

// Codex imports Codex sessions into the .so hub via the port orchestrator.
func Codex(paths harness.Paths) (int, error) {
	return portToHub(paths, port.HarnessCodex)
}

func portToHub(paths harness.Paths, from port.HarnessID) (int, error) {
	repoRoot := filepath.Dir(paths.Root)
	if strings.EqualFold(filepath.Base(paths.Root), harness.DirName) {
		repoRoot = filepath.Dir(paths.Root)
	}
	reg := port.NewRegistry()
	adapters.RegisterAll(reg, repoRoot)
	o := &port.Orchestrator{
		Reg:      reg,
		Ledger:   port.NewLedger(port.DefaultLedgerPath(paths.Root)),
		RepoRoot: repoRoot,
	}
	res, err := o.Port(port.PortOptions{From: from, To: port.HarnessSOHub, All: true, Force: false})
	if err != nil {
		return res.Ported, err
	}
	if res.Failed > 0 && res.Ported == 0 {
		return res.Ported, fmt.Errorf("import failed for all sessions (%d errors)", res.Failed)
	}
	return res.Ported + res.Skipped, nil
}
