// Package userpaths resolves Superopen config and data directories on
// macOS, Linux, and Windows (XDG / %APPDATA% / %LOCALAPPDATA%).
package userpaths

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ConfigDir returns ~/.config/superopen (Unix) or %APPDATA%\superopen (Windows).
func ConfigDir() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "superopen"), nil
	}
	if runtime.GOOS == "windows" {
		if cfg, err := os.UserConfigDir(); err == nil && cfg != "" {
			return filepath.Join(cfg, "superopen"), nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	return filepath.Join(home, ".config", "superopen"), nil
}

// DataDir returns XDG data home / %LOCALAPPDATA%\superopen for marketplaces and caches.
func DataDir() (string, error) {
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "superopen"), nil
	}
	if runtime.GOOS == "windows" {
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			return filepath.Join(local, "superopen"), nil
		}
		if cfg, err := os.UserConfigDir(); err == nil && cfg != "" {
			// Fallback: sibling of Roaming → Local is unreliable; use home\AppData\Local.
			return filepath.Join(filepath.Dir(cfg), "Local", "superopen"), nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", "superopen"), nil
}

// CodexMarketplaceDir is where we materialize the Codex plugin marketplace.
func CodexMarketplaceDir() (string, error) {
	base, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "codex-marketplace"), nil
}

// ShellPath rewrites a path for POSIX sh hooks (Git for Windows).
func ShellPath(p string) string {
	p = filepath.Clean(p)
	if runtime.GOOS == "windows" {
		p = strings.ReplaceAll(p, `\`, `/`)
	}
	return p
}

// QuoteForHook quotes a binary path for hook command lines.
// JSON manifests use strconv.Quote separately; this is for shell/cmd strings.
func QuoteForHook(p string) string {
	if p == "" {
		return `""`
	}
	if runtime.GOOS == "windows" {
		// Prefer JSON-safe double quotes for Cursor/Claude hook JSON commands.
		if !strings.ContainsAny(p, " \t\"") {
			return p
		}
		return `"` + strings.ReplaceAll(p, `"`, `\"`) + `"`
	}
	if !strings.ContainsAny(p, " \t\n'\"\\$`") {
		return p
	}
	return "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
}

// IsSoBinary reports whether base name is so or so.exe.
func IsSoBinary(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return base == "so" || base == "so.exe"
}
