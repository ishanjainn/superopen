//go:build !windows

package paths

import (
	"os"
	"path/filepath"
	"strings"
)

// RemoveUserBinFromPATH strips release-installer PATH snippets from shell rc files.
func RemoveUserBinFromPATH() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var touched []string
	for _, name := range []string{".zprofile", ".zshrc", ".bash_profile", ".bashrc"} {
		path := filepath.Join(home, name)
		if stripPathSnippet(path) {
			touched = append(touched, path)
		}
	}
	return touched
}

func stripPathSnippet(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	lines := strings.Split(string(data), "\n")
	var out []string
	changed := false
	skipNext := false
	for _, line := range lines {
		if skipNext {
			skipNext = false
			changed = true
			continue
		}
		if strings.TrimSpace(line) == shellPathComment {
			skipNext = true
			changed = true
			continue
		}
		if strings.Contains(line, userBinPathMarker) && strings.Contains(line, "export PATH=") {
			changed = true
			continue
		}
		out = append(out, line)
	}
	if !changed {
		return false
	}
	body := strings.Join(out, "\n")
	return os.WriteFile(path, []byte(body), 0o644) == nil
}
