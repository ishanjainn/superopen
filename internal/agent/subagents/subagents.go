// Package subagents installs the user-global Superopen graph subagents into
// coding-agent agent directories. Delegating graph work to a child keeps the
// exploration turns and their tokens out of the parent's context.
package subagents

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ishanjainn/superopen/internal/paths"
)

//go:embed so-scout.md so-verify.md so-auditor.md
var agentFS embed.FS

// Names are the subagent files Superopen owns. Uninstall removes exactly
// these, so a user's own agent definitions are never touched.
var Names = []string{"so-scout.md", "so-verify.md", "so-auditor.md"}

// InstallAll writes the Superopen subagents into every agent directory whose
// vendor is present on this machine.
func InstallAll() ([]string, error) {
	var written []string
	for _, dir := range agentDirs() {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return written, fmt.Errorf("mkdir %s: %w", dir, err)
		}
		for _, name := range Names {
			body, err := agentFS.ReadFile(name)
			if err != nil {
				return written, err
			}
			dst := filepath.Join(dir, name)
			if err := os.WriteFile(dst, body, 0o644); err != nil {
				return written, err
			}
			written = append(written, dst)
		}
	}
	return written, nil
}

// RemoveAll deletes the Superopen subagent files (best-effort).
func RemoveAll() []string {
	var removed []string
	for _, dir := range agentDirs() {
		for _, name := range Names {
			path := filepath.Join(dir, name)
			if err := os.Remove(path); err == nil {
				removed = append(removed, path)
			}
		}
	}
	return removed
}

// agentDirs returns agent directories for vendors that exist on this machine.
// Unlike skills, an agent file in an unknown location is inert, so we only
// write where a vendor home is already present.
func agentDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	candidates := []struct{ vendorHome, agentDir string }{
		{filepath.Join(home, ".claude"), filepath.Join(home, ".claude", "agents")},
		{filepath.Join(home, ".cursor"), filepath.Join(home, ".cursor", "agents")},
		{filepath.Join(home, ".agents"), filepath.Join(home, ".agents", "agents")},
		{filepath.Join(home, ".codex"), filepath.Join(home, ".codex", "agents")},
	}
	if cfg, err := paths.OpenCodeConfigDir(); err == nil && cfg != "" {
		candidates = append(candidates, struct{ vendorHome, agentDir string }{cfg, filepath.Join(cfg, "agents")})
	}
	if copilot := strings.TrimSpace(os.Getenv("COPILOT_HOME")); copilot != "" {
		candidates = append(candidates, struct{ vendorHome, agentDir string }{copilot, filepath.Join(copilot, "agents")})
	}
	if runtime.GOOS == "windows" {
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			candidates = append(candidates, struct{ vendorHome, agentDir string }{
				filepath.Join(local, "claude"), filepath.Join(local, "claude", "agents"),
			})
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate.vendorHome); err != nil || !info.IsDir() {
			continue
		}
		dir := filepath.Clean(candidate.agentDir)
		if seen[dir] {
			continue
		}
		seen[dir] = true
		out = append(out, dir)
	}
	return out
}

// AgentFS exposes the embedded subagent files for tests.
func AgentFS() fs.FS { return agentFS }
