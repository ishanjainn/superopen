package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	userBinPathMarker = ".superopen/bin"
	shellPathComment  = "# Superopen CLI"
)

// UserBinDir is where the curl/release installer places `so`
// ($HOME/.superopen/bin or %USERPROFILE%\.superopen\bin, overridable
// with SUPEROPEN_INSTALL_DIR). Package managers (Homebrew, Scoop, …)
// do not use this directory.
func UserBinDir() (string, error) {
	if d := strings.TrimSpace(os.Getenv("SUPEROPEN_INSTALL_DIR")); d != "" {
		return filepath.Clean(d), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	return filepath.Join(home, ".superopen", "bin"), nil
}

// UserBinPath is UserBinDir plus `so` (or `so.exe` on Windows).
func UserBinPath() (string, error) {
	dir, err := UserBinDir()
	if err != nil {
		return "", err
	}
	name := "so"
	if runtime.GOOS == "windows" {
		name = "so.exe"
	}
	return filepath.Join(dir, name), nil
}

// CurlInstallRoot is the release-installer prefix (~/.superopen on every OS).
func CurlInstallRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".superopen"), nil
}

// IsHomebrewPath reports whether p is inside Homebrew or Linuxbrew.
func IsHomebrewPath(p string) bool {
	s := slashLower(p)
	return strings.Contains(s, "/cellar/") ||
		strings.Contains(s, "/homebrew/") ||
		strings.Contains(s, "/linuxbrew/") ||
		strings.HasPrefix(s, "/opt/homebrew/")
}

// IsPackageManagedPath reports whether p is owned by Homebrew, Linuxbrew,
// Scoop, Chocolatey, or WinGet — uninstall must not delete those binaries.
func IsPackageManagedPath(p string) bool {
	if IsHomebrewPath(p) {
		return true
	}
	s := slashLower(p)
	return strings.Contains(s, "/scoop/") ||
		strings.Contains(s, "/chocolatey/") ||
		strings.Contains(s, "/winget/packages/") ||
		strings.Contains(s, "/windowsapps/")
}

// PackageManagerUninstallHint is the command that removes a package-managed
// so binary, or empty if p is the release-installer / a local build.
func PackageManagerUninstallHint(p string) string {
	s := slashLower(p)
	switch {
	case IsHomebrewPath(p):
		return "brew uninstall so"
	case strings.Contains(s, "/scoop/"):
		return "scoop uninstall so"
	case strings.Contains(s, "/chocolatey/"):
		return "choco uninstall so"
	case strings.Contains(s, "/winget/packages/"), strings.Contains(s, "/windowsapps/"):
		return "winget uninstall so"
	default:
		return ""
	}
}

// PathUnder reports whether child is parent or a descendant. Comparison is
// case-insensitive on Windows.
func PathUnder(child, parent string) bool {
	if parent == "" || child == "" {
		return false
	}
	child = filepath.Clean(child)
	parent = filepath.Clean(parent)
	if runtime.GOOS == "windows" {
		child = strings.ToLower(child)
		parent = strings.ToLower(parent)
	}
	if child == parent {
		return true
	}
	sep := string(os.PathSeparator)
	return strings.HasPrefix(child, parent+sep)
}

// IsSuperopenHookCommand reports whether a Cursor hooks.json command was
// authored by Superopen (including path-rewritten graph refresh / finalize).
func IsSuperopenHookCommand(cmd string) bool {
	if cmd == "" {
		return false
	}
	return strings.Contains(cmd, "coding hook --vendor=cursor") ||
		strings.Contains(cmd, "sessions finalize") ||
		strings.Contains(cmd, "sessions refresh") ||
		strings.Contains(cmd, "graph refresh")
}

func slashLower(p string) string {
	return strings.ToLower(strings.ReplaceAll(p, `\`, `/`))
}

func pathDirsToStrip() []string {
	var dirs []string
	if d, err := UserBinDir(); err == nil && d != "" {
		dirs = append(dirs, d)
	}
	if root, err := CurlInstallRoot(); err == nil && root != "" {
		dirs = append(dirs, filepath.Join(root, "bin"))
	}
	return dirs
}

// stripPathList removes dirs from a PATH-style string (':' or ';' separated).
func stripPathList(path string, dirs []string) (string, bool) {
	if path == "" || len(dirs) == 0 {
		return path, false
	}
	want := make([]string, 0, len(dirs))
	for _, d := range dirs {
		want = append(want, filepath.Clean(d))
	}
	sep := string(os.PathListSeparator)
	parts := strings.Split(path, sep)
	var kept []string
	changed := false
	for _, part := range parts {
		if part == "" {
			continue
		}
		clean := filepath.Clean(part)
		drop := false
		for _, d := range want {
			if pathEntriesEqual(clean, d) {
				drop = true
				break
			}
		}
		if drop {
			changed = true
			continue
		}
		kept = append(kept, part)
	}
	if !changed {
		return path, false
	}
	return strings.Join(kept, sep), true
}

func pathEntriesEqual(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
