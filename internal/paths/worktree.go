package paths

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// LinkedWorktreeParent reports the primary checkout of a linked git worktree.
// ok is false when root is not a linked worktree or git is unavailable.
func LinkedWorktreeParent(root string) (parent string, ok bool) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", false
	}
	gitDir := gitAbs(root, "--git-dir")
	commonDir := gitAbs(root, "--git-common-dir")
	if gitDir == "" || commonDir == "" {
		return "", false
	}
	if filepath.Clean(gitDir) == filepath.Clean(commonDir) {
		return "", false
	}
	parent = commonDir
	for strings.HasSuffix(parent, string(filepath.Separator)) {
		parent = strings.TrimSuffix(parent, string(filepath.Separator))
	}
	if filepath.Base(parent) == ".git" {
		parent = filepath.Dir(parent)
	}
	parent = filepath.Clean(parent)
	if parent == "" || parent == root || parent == filepath.Clean(root) {
		return "", false
	}
	return parent, true
}

func gitAbs(root string, flag string) string {
	cmd := exec.Command("git", "-C", root, "rev-parse", "--path-format=absolute", flag)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
