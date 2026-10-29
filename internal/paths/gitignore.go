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

// ExcludeGraphFromGit keeps machine-local graph data out of git status without
// changing the checkout's tracked .gitignore.
func ExcludeGraphFromGit(repoRoot string) error {
	exclude, pattern, err := graphGitExclude(repoRoot)
	if err != nil || exclude == "" {
		return err
	}
	content, err := os.ReadFile(exclude)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, line := range strings.Split(string(content), "\n") {
		if strings.TrimSpace(line) == pattern {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(exclude), 0o755); err != nil {
		return err
	}
	if len(content) > 0 && content[len(content)-1] != '\n' {
		content = append(content, '\n')
	}
	content = append(content, []byte("# Superopen machine-local data\n"+pattern+"\n")...)
	return os.WriteFile(exclude, content, 0o644)
}

func graphGitExclude(repoRoot string) (string, string, error) {
	top := gitShowTopLevel(repoRoot)
	if top == "" {
		return "", "", nil
	}
	cmd := exec.Command("git", "-C", repoRoot, "rev-parse", "--git-path", "info/exclude")
	output, err := cmd.Output()
	if err != nil {
		return "", "", err
	}
	exclude := strings.TrimSpace(string(output))
	if !filepath.IsAbs(exclude) {
		exclude = filepath.Join(repoRoot, exclude)
	}
	rel, err := filepath.Rel(top, canonicalPath(repoRoot))
	if err != nil {
		return "", "", err
	}
	pattern := "/.so/"
	if rel != "." {
		pattern = "/" + filepath.ToSlash(rel) + "/.so/"
	}
	return exclude, pattern, nil
}

func removeGraphGitExclude(repoRoot string) error {
	exclude, pattern, err := graphGitExclude(repoRoot)
	if err != nil || exclude == "" {
		return err
	}
	content, err := os.ReadFile(exclude)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	next := strings.Replace(string(content), "# Superopen machine-local data\n"+pattern+"\n", "", 1)
	if next == string(content) {
		return nil
	}
	return os.WriteFile(exclude, []byte(next), 0o644)
}

// RemoveRepoIgnore strips our local exclude entry and a marker-fenced .so/
// block that older init wrote into the top-level .gitignore.
func RemoveRepoIgnore(repoRoot string) error {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return nil
	}
	if err := removeGraphGitExclude(repoRoot); err != nil {
		return err
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
