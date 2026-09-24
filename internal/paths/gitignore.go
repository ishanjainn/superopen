package paths

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	repoIgnoreBegin = "# BEGIN SUPEROPEN"
	repoIgnoreEnd   = "# END SUPEROPEN"
)

// RemoveRepoIgnore strips a marker-fenced .so/ block that older init wrote
// into the git top-level .gitignore. No-op outside git or when the block is absent.
func RemoveRepoIgnore(repoRoot string) error {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return nil
	}
	top := gitShowTopLevel(repoRoot)
	if top == "" {
		return nil
	}
	ignorePath := filepath.Join(top, ".gitignore")
	prev, err := os.ReadFile(ignorePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	next := stripRepoIgnore(string(prev))
	if next == string(prev) {
		return nil
	}
	trimmed := strings.TrimSpace(next)
	if trimmed == "" {
		return os.Remove(ignorePath)
	}
	return os.WriteFile(ignorePath, []byte(next), 0o644)
}

func gitShowTopLevel(dir string) string {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return canonicalPath(strings.TrimSpace(string(out)))
}

func canonicalPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return filepath.Clean(real)
	}
	return filepath.Clean(abs)
}

func stripRepoIgnore(existing string) string {
	start := strings.Index(existing, repoIgnoreBegin)
	end := strings.Index(existing, repoIgnoreEnd)
	if start < 0 || end < start {
		return existing
	}
	end += len(repoIgnoreEnd)
	for end < len(existing) && (existing[end] == '\n' || existing[end] == '\r') {
		end++
	}
	out := existing[:start] + existing[end:]
	if strings.TrimSpace(out) == "" {
		return ""
	}
	return out
}
