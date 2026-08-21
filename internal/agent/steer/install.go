package steer

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/ishanjainn/superopen/internal/paths"
)

// InstallAll merges durable graph-first guidance into each agent’s user-level
// instruction surface. Never writes into a repository working tree.
func InstallAll() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	var written []string
	targets := durableTargets(home)
	for _, path := range targets {
		if err := WriteMergedFile(path); err != nil {
			return written, err
		}
		written = append(written, path)
	}
	// Cursor alwaysApply rule (user-global).
	cursorRule := filepath.Join(home, ".cursor", "rules", "superopen.mdc")
	if err := writeCursorRule(cursorRule); err != nil {
		return written, err
	}
	written = append(written, cursorRule)
	return written, nil
}

// InstallProjectCursorRule writes <repo>/.cursor/rules/superopen.mdc so this
// repository always includes Superopen guidance even without the user-global rule.
func InstallProjectCursorRule(repoRoot string) (string, error) {
	path := filepath.Join(repoRoot, ".cursor", "rules", "superopen.mdc")
	if err := writeCursorRule(path); err != nil {
		return "", err
	}
	return path, nil
}

// RemoveProjectCursorRule deletes the per-repo Cursor rule if Superopen wrote it.
func RemoveProjectCursorRule(repoRoot string) string {
	path := filepath.Join(repoRoot, ".cursor", "rules", "superopen.mdc")
	if err := os.Remove(path); err == nil {
		return path
	}
	return ""
}

// RemoveAll strips Superopen durable blocks from user-level instruction files.
func RemoveAll() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var removed []string
	for _, path := range durableTargets(home) {
		prev, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if err := RemoveFromFile(path); err != nil {
			continue
		}
		if string(prev) != "" {
			removed = append(removed, path)
		}
	}
	cursorRule := filepath.Join(home, ".cursor", "rules", "superopen.mdc")
	if err := os.Remove(cursorRule); err == nil {
		removed = append(removed, cursorRule)
	}
	return removed
}

func durableTargets(home string) []string {
	codexHome, _ := paths.CodexHome()
	opencode, _ := paths.OpenCodeConfigDir()
	copilot, _ := paths.CopilotHome()
	out := []string{
		filepath.Join(home, ".claude", "CLAUDE.md"),
		filepath.Join(codexHome, "AGENTS.md"),
		filepath.Join(home, ".agents", "AGENTS.md"),
		filepath.Join(home, ".gemini", "GEMINI.md"),
		filepath.Join(opencode, "AGENTS.md"),
		filepath.Join(copilot, "AGENTS.md"),
		filepath.Join(home, ".pi", "agent", "AGENTS.md"),
	}
	if runtime.GOOS == "windows" {
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			out = append(out, filepath.Join(local, "claude", "CLAUDE.md"))
		}
	}
	return out
}

func writeCursorRule(path string) error {
	body := "---\ndescription: Superopen — ignore unless this workspace has a .so directory\nalwaysApply: true\n---\n\n" +
		beginMarker + "\n" + CursorRule() + endMarker + "\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(body), 0o644)
}
