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

// EnsureRepoIgnore adds a marker-fenced .so/ pattern to the git top-level
// .gitignore so `git add -A` never stages Superopen machine-local data.
// No-op outside a git checkout, and no-op when git already ignores the
// directory (global excludesfile, info/exclude, or an existing pattern).
func EnsureRepoIgnore(repoRoot string) (string, error) {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return "", nil
	}
	top := gitShowTopLevel(repoRoot)
	if top == "" {
		return "", nil
	}
	repoRoot = canonicalPath(repoRoot)
	top = canonicalPath(top)
	soDir := Resolve(repoRoot).Root
	if gitIgnores(top, soDir) {
		return "", nil
	}
	pattern := ignorePattern(top, soDir)
	if pattern == "" {
		return "", nil
	}
	ignorePath := filepath.Join(top, ".gitignore")
	prev, err := os.ReadFile(ignorePath)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	next := mergeRepoIgnore(string(prev), pattern)
	if next == string(prev) {
		return "", nil
	}
	if err := os.WriteFile(ignorePath, []byte(next), 0o644); err != nil {
		return "", err
	}
	return ignorePath, nil
}

// RemoveRepoIgnore strips the Superopen marker-fenced block from the git
// top-level .gitignore. No-op outside git or when the block is absent.
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

func gitIgnores(top, path string) bool {
	if top == "" || path == "" {
		return false
	}
	rel := path
	if r, err := filepath.Rel(top, path); err == nil {
		rel = r
	}
	cmd := exec.Command("git", "-C", top, "check-ignore", "-q", "--", rel)
	err := cmd.Run()
	if err == nil {
		return true
	}
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		return false
	}
	return false
}

func ignorePattern(gitTop, soDir string) string {
	rel, err := filepath.Rel(gitTop, soDir)
	if err != nil {
		return ""
	}
	rel = filepath.ToSlash(rel)
	if rel == "" || rel == "." {
		return DirName + "/"
	}
	if strings.HasPrefix(rel, "../") {
		return ""
	}
	return strings.TrimSuffix(rel, "/") + "/"
}

func mergeRepoIgnore(existing, pattern string) string {
	nl := detectNewline(existing)
	if alreadyIgnoresPattern(existing, pattern) {
		return existing
	}
	block := repoIgnoreBegin + nl + pattern + nl + repoIgnoreEnd + nl
	start := strings.Index(existing, repoIgnoreBegin)
	end := strings.Index(existing, repoIgnoreEnd)
	if start >= 0 && end > start {
		end += len(repoIgnoreEnd)
		for end < len(existing) && (existing[end] == '\n' || existing[end] == '\r') {
			end++
		}
		inner := strings.TrimSpace(existing[start+len(repoIgnoreBegin) : end-len(repoIgnoreEnd)])
		inner = strings.ReplaceAll(inner, "\r\n", "\n")
		inner = strings.ReplaceAll(inner, "\r", "\n")
		lines := splitNonEmpty(inner)
		if !containsLine(lines, pattern) {
			lines = append(lines, pattern)
		}
		var b strings.Builder
		b.WriteString(existing[:start])
		b.WriteString(repoIgnoreBegin)
		b.WriteString(nl)
		for _, line := range lines {
			b.WriteString(line)
			b.WriteString(nl)
		}
		b.WriteString(repoIgnoreEnd)
		b.WriteString(nl)
		b.WriteString(existing[end:])
		return b.String()
	}
	trimmed := strings.TrimRight(existing, " \t")
	if trimmed == "" {
		return block
	}
	if !strings.HasSuffix(trimmed, "\n") && !strings.HasSuffix(trimmed, "\r") {
		trimmed += nl
	}
	return trimmed + nl + block
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

func alreadyIgnoresPattern(content, pattern string) bool {
	start := strings.Index(content, repoIgnoreBegin)
	end := strings.Index(content, repoIgnoreEnd)
	if start >= 0 && end > start {
		inner := content[start:end]
		if containsIgnoreLine(inner, pattern) {
			return true
		}
	}
	return containsIgnoreLine(content, pattern) ||
		containsIgnoreLine(content, strings.TrimSuffix(pattern, "/")) ||
		containsIgnoreLine(content, "/"+strings.TrimPrefix(pattern, "/"))
}

func containsIgnoreLine(content, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	for _, line := range strings.Split(normalized, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == want || line == strings.TrimSuffix(want, "/") {
			return true
		}
	}
	return false
}

func splitNonEmpty(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == repoIgnoreBegin || line == repoIgnoreEnd {
			continue
		}
		out = append(out, line)
	}
	return out
}

func containsLine(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}

func detectNewline(s string) string {
	if strings.Contains(s, "\r\n") {
		return "\r\n"
	}
	return "\n"
}
