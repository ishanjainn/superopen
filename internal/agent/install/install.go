// Package install implements per-vendor host plugin manifest writes for
// `so install --vendor=...`.
//
// Writes per-vendor host plugin manifests to the user's home directory
// so the agent (Claude Code, Cursor, Codex) finds them on next
// launch. The manifest payloads themselves live under internal/agent/install/marketplace/
// (mirrored from the repo-root `.claude-plugin/` + `plugins/` by
// `scripts/sync-plugins.sh`) and are embedded into the binary at
// build time, so a single statically-linked CLI carries everything.
package install

import (
	"fmt"
	"strings"

	"github.com/ishanjainn/superopen/internal/agent/vendors"
)

// vendorsFromArg expands the --vendor argument into a slice of vendor IDs.
func vendorsFromArg(arg string) ([]string, error) {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "all":
		return append([]string{"claude-code", "cursor", "codex", "gemini", "opencode", "copilot-cli", "pi"}, vendors.IDs()...), nil
	case "claude-code", "cc":
		return []string{"claude-code"}, nil
	case "cursor":
		return []string{"cursor"}, nil
	case "codex":
		return []string{"codex"}, nil
	case "gemini", "gemini-cli":
		return []string{"gemini"}, nil
	case "opencode", "open-code":
		return []string{"opencode"}, nil
	case "copilot", "copilot-cli":
		return []string{"copilot-cli"}, nil
	case "pi":
		return []string{"pi"}, nil
	default:
		if spec, ok := vendors.ByID(arg); ok {
			return []string{spec.ID}, nil
		}
		return nil, fmt.Errorf("unknown --vendor %q", arg)
	}
}
