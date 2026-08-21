package projects

import (
	"fmt"
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
		if err == nil && paths.Managed(found) {
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
