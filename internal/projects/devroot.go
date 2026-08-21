package projects

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/paths"
)

// ResolveDevRoot picks the Superopen-managed repo `so dev` should bind to.
// explicit is --root / SUPEROPEN_ROOT (must already have .so).
// cwd is used when explicit is empty: that tree if inited, else the active
// or most recently seen registered project that still has .so.
func ResolveDevRoot(explicit, cwd string) (string, error) {
	if root := strings.TrimSpace(explicit); root != "" {
		if !paths.Managed(root) {
			return "", fmt.Errorf("%s", paths.UnmanagedMessage)
		}
		return root, nil
	}
	if cwd != "" {
		found, err := paths.FindRoot(cwd)
		if err == nil && paths.Managed(found) && !sharedTempRoot(found) {
			return found, nil
		}
		if paths.Managed(cwd) {
			return cwd, nil
		}
	}
	if p, err := Active(); err == nil && paths.Managed(p.RepoRoot) {
		return p.RepoRoot, nil
	}
	list, err := List()
	if err != nil {
		return "", err
	}
	for _, p := range list {
		if paths.Managed(p.RepoRoot) {
			return p.RepoRoot, nil
		}
	}
	return "", fmt.Errorf("no Superopen-managed repos; run so init in a repository")
}

// sharedTempRoot reports whether path is the process temp directory itself
// (for example /tmp). FindRoot walks through that directory, and a leftover
// /tmp/.so on a CI runner must not become the so dev project.
func sharedTempRoot(path string) bool {
	clean := filepath.Clean(path)
	temps := []string{filepath.Clean(os.TempDir())}
	if resolved, err := filepath.EvalSymlinks(os.TempDir()); err == nil {
		temps = append(temps, filepath.Clean(resolved))
	}
	resolvedPath := clean
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		resolvedPath = filepath.Clean(resolved)
	}
	for _, tmp := range temps {
		if tmp == "" {
			continue
		}
		if clean == tmp || resolvedPath == tmp {
			return true
		}
	}
	return false
}
