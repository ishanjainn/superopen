// Package skills installs the user-global /so skill into coding-agent skill dirs.
package skills

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ishanjainn/superopen/internal/paths"
)

//go:embed so/SKILL.md
var skillFS embed.FS

const skillName = "so"
const soBinPlaceholder = "__SO_BIN__"

// InstallAll writes the Superopen skill into every supported agent skill location.
// soBin is substituted for __SO_BIN__; when empty, resolveSoBin is used, then "so".
func InstallAll(soBin string) ([]string, error) {
	body, err := skillFS.ReadFile("so/SKILL.md")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(soBin) == "" {
		soBin, err = resolveSoBin()
		if err != nil || soBin == "" {
			soBin = "so"
		}
	}
	rendered := strings.ReplaceAll(string(body), soBinPlaceholder, soBin)
	var written []string
	for _, dir := range skillDirs() {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return written, fmt.Errorf("mkdir %s: %w", dir, err)
		}
		dst := filepath.Join(dir, "SKILL.md")
		if err := os.WriteFile(dst, []byte(rendered), 0o644); err != nil {
			return written, err
		}
		written = append(written, dst)
	}
	return written, nil
}

func resolveSoBin() (string, error) {
	if exe, err := os.Executable(); err == nil {
		base := strings.TrimSuffix(filepath.Base(exe), ".exe")
		if base == "so" {
			return exe, nil
		}
	}
	return exec.LookPath("so")
}

// RemoveAll deletes installed Superopen skill trees (best-effort).
func RemoveAll() []string {
	var removed []string
	for _, dir := range skillDirs() {
		if err := os.RemoveAll(dir); err == nil {
			removed = append(removed, dir)
		}
	}
	return removed
}

func skillDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	dirs := []string{
		filepath.Join(home, ".claude", "skills", skillName),
		filepath.Join(home, ".cursor", "skills", skillName),
		filepath.Join(home, ".agents", "skills", skillName),
		filepath.Join(home, ".codex", "skills", skillName),
		filepath.Join(home, ".gemini", "skills", skillName),
		filepath.Join(home, ".pi", "agent", "skills", skillName),
	}
	if cfg, err := paths.OpenCodeConfigDir(); err == nil && cfg != "" {
		dirs = append(dirs, filepath.Join(cfg, "skills", skillName))
	}
	if copilot := strings.TrimSpace(os.Getenv("COPILOT_HOME")); copilot != "" {
		dirs = append(dirs, filepath.Join(copilot, "skills", skillName))
	} else {
		dirs = append(dirs, filepath.Join(home, ".copilot", "skills", skillName))
	}
	if runtime.GOOS == "windows" {
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			dirs = append(dirs, filepath.Join(local, "claude", "skills", skillName))
		}
	}
	return uniqueExistingParents(dirs)
}

func uniqueExistingParents(dirs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, d := range dirs {
		d = filepath.Clean(d)
		if seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	return out
}

// SkillFS exposes embedded skill files for tests.
func SkillFS() fs.FS { return skillFS }
