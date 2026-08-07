// Package githooks installs prepare-commit-msg / post-commit hooks that
// link commits to Superopen sessions via SO-Session / SO-Attribution trailers.
package githooks

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ishanjainn/superopen/internal/userpaths"
)

const (
	TrailerSession     = "SO-Session"
	TrailerAttribution = "SO-Attribution"
)

// Install writes Superopen-managed git hooks under .git/hooks (or core.hooksPath).
// Scripts use #!/bin/sh (works with Git for Windows' sh) and quote the so binary
// with forward-slash paths so spaces and Windows drive letters work.
func Install(repoRoot, soBinary string) error {
	hooksDir, err := hooksDir(repoRoot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return err
	}
	if soBinary == "" {
		soBinary = "so"
		if runtime.GOOS == "windows" {
			if p, err := exec.LookPath("so"); err == nil {
				soBinary = p
			} else if p, err := exec.LookPath("so.exe"); err == nil {
				soBinary = p
			}
		}
	}
	soSh := userpaths.ShellPath(soBinary)

	hooks := []struct {
		name string
		desc string
		args string
	}{
		{"prepare-commit-msg", "appends SO-Session trailer when a session is active", "prepare-commit-msg"},
		{"post-commit", "finalize session + optional attribution", "post-commit"},
		{"post-merge", "refresh harness after git pull/merge", "post-merge"},
		{"post-checkout", "refresh harness after branch checkout", "post-checkout"},
	}
	for _, h := range hooks {
		body := fmt.Sprintf(`#!/bin/sh
# Superopen %s - %s.
# Git for Windows runs this via sh.exe; paths use forward slashes.
exec "%s" githook %s "$@"
`, h.name, h.desc, soSh, h.args)
		if err := writeHook(filepath.Join(hooksDir, h.name), body); err != nil {
			return err
		}
		// Companion .cmd helps some Windows hosts that invoke hooks without sh.
		if runtime.GOOS == "windows" {
			cmdBody := fmt.Sprintf("@echo off\r\nREM Superopen %s\r\n\"%s\" githook %s %%*\r\n",
				h.name, soBinary, h.args)
			_ = os.WriteFile(filepath.Join(hooksDir, h.name+".cmd"), []byte(cmdBody), 0o755)
		}
	}
	return nil
}

func writeHook(path, body string) error {
	// Preserve user hooks by chaining if a non-SO hook exists.
	if data, err := os.ReadFile(path); err == nil {
		s := string(data)
		if !strings.Contains(s, "Superopen") && !strings.Contains(s, "so\" githook") && !strings.Contains(s, "githook ") {
			backup := path + ".so-backup"
			_ = os.WriteFile(backup, data, 0o755)
		}
	}
	return os.WriteFile(path, []byte(body), 0o755)
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
