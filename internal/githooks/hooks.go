// Package githooks previously installed prepare-commit-msg / post-commit /
// pre-push hooks. Those hooks made commits slow and pushes hang (pre-push
// tried to FF-push refs/so/sessions/*). Installation is disabled; Install now
// removes any leftover Superopen-managed hooks.
package githooks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	TrailerSession     = "SO-Session"
	TrailerAttribution = "SO-Attribution"
)

var managedHooks = []string{
	"prepare-commit-msg",
	"post-commit",
	"post-merge",
	"post-checkout",
	"pre-push",
}

// Install used to write Superopen git hooks. It now only removes them so
// `so sync` / `so init` cannot put slow/hanging hooks back.
func Install(repoRoot, soBinary string) error {
	_ = soBinary
	return Remove(repoRoot)
}

// Remove deletes Superopen-managed hooks under .git/hooks (or core.hooksPath).
// Non-Superopen hooks are left alone. Safe to call repeatedly.
func Remove(repoRoot string) error {
	hooksDir, err := hooksDir(repoRoot)
	if err != nil {
		return err
	}
	for _, name := range managedHooks {
		path := filepath.Join(hooksDir, name)
		if !isSuperopenHook(path) {
			continue
		}
		_ = os.Remove(path)
		_ = os.Remove(path + ".cmd")
		_ = os.Remove(path + ".so-backup")
	}
	return nil
}

func isSuperopenHook(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	s := string(data)
	return strings.Contains(s, "Superopen") ||
		strings.Contains(s, "so\" githook") ||
		strings.Contains(s, "githook ")
}

func hooksDir(repoRoot string) (string, error) {
	cmd := exec.Command("git", "-C", repoRoot, "rev-parse", "--git-path", "hooks")
	out, err := cmd.Output()
	if err != nil {
		return filepath.Join(repoRoot, ".git", "hooks"), nil
	}
	p := strings.TrimSpace(string(out))
	if !filepath.IsAbs(p) {
		p = filepath.Join(repoRoot, p)
	}
	return p, nil
}

// AppendTrailer appends a git trailer to a commit message file.
func AppendTrailer(msgPath, key, value string) error {
	data, err := os.ReadFile(msgPath)
	if err != nil {
		return err
	}
	msg := string(data)
	line := key + ": " + value
	if strings.Contains(msg, key+":") {
		return nil // already linked
	}
	trimmed := strings.TrimRight(msg, "\n")
	// Keep comment lines at end (git commit editor).
	lines := strings.Split(trimmed, "\n")
	commentStart := len(lines)
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "#") {
			commentStart = i
			continue
		}
		break
	}
	body := strings.Join(lines[:commentStart], "\n")
	comments := ""
	if commentStart < len(lines) {
		comments = "\n" + strings.Join(lines[commentStart:], "\n")
	}
	body = strings.TrimRight(body, "\n")
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	// Ensure blank line before trailer block.
	if body != "" && !strings.HasSuffix(body, "\n\n") {
		body += "\n"
	}
	newMsg := body + line + "\n" + comments
	if !strings.HasSuffix(newMsg, "\n") {
		newMsg += "\n"
	}
	return os.WriteFile(msgPath, []byte(newMsg), 0o644)
}

// ParseTrailers extracts SO-* trailers from a commit message.
func ParseTrailers(msg string) (sessionID, attribution string) {
	for _, line := range strings.Split(msg, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, TrailerSession+":") {
			sessionID = strings.TrimSpace(strings.TrimPrefix(line, TrailerSession+":"))
		}
		if strings.HasPrefix(line, TrailerAttribution+":") {
			attribution = strings.TrimSpace(strings.TrimPrefix(line, TrailerAttribution+":"))
		}
	}
	return sessionID, attribution
}
