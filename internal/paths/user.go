// Package userpaths resolves Superopen config and data directories on
// macOS, Linux, and Windows (XDG / %APPDATA% / %LOCALAPPDATA%).
package paths

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// RuntimeDir returns machine-local transient state outside the repository.
func RuntimeDir(repoRoot string) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(filepath.Clean(repoRoot)))
	return filepath.Join(base, "superopen", "runtime", fmt.Sprintf("%x", sum[:12])), nil
}

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

// ClaudeConfigDir returns Claude Code's user config root. CLAUDE_CONFIG_DIR
// is authoritative when set; otherwise Claude uses ~/.claude.
func ClaudeConfigDir() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); configured != "" {
		return filepath.Clean(configured), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	return filepath.Join(home, ".claude"), nil
}

// CodexHome returns the host's Codex configuration root. CODEX_HOME is
// authoritative when set; otherwise Codex uses ~/.codex on every platform.
func CodexHome() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("CODEX_HOME")); configured != "" {
		return filepath.Clean(configured), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	return filepath.Join(home, ".codex"), nil
}

// CopilotHome returns the GitHub Copilot CLI configuration root. The CLI's
// documented COPILOT_HOME override applies to hook installation.
func CopilotHome() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("COPILOT_HOME")); configured != "" {
		return filepath.Clean(configured), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	return filepath.Join(home, ".copilot"), nil
}

// OpenCodeConfigDir returns OpenCode's global configuration directory.
// OpenCode documents ~/.config/opencode and honors XDG_CONFIG_HOME.
// Native Windows uses %APPDATA%\opencode unless XDG_CONFIG_HOME is set
// (WSL and Git-Bash users who export XDG still follow the Unix branch).
func OpenCodeConfigDir() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); configured != "" {
		return filepath.Join(configured, "opencode"), nil
	}
	if runtime.GOOS == "windows" {
		if cfg, err := os.UserConfigDir(); err == nil && cfg != "" {
			return filepath.Join(cfg, "opencode"), nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	return filepath.Join(home, ".config", "opencode"), nil
}

// OpenCodeDataDir returns the directory containing OpenCode's local database.
// Native Windows uses LocalAppData; WSL follows the Unix/XDG branch.
func OpenCodeDataDir() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); configured != "" {
		return filepath.Join(configured, "opencode"), nil
	}
	if runtime.GOOS == "windows" {
		if local := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); local != "" {
			return filepath.Join(local, "opencode"), nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", "opencode"), nil
}

// ShellPath rewrites a path for POSIX sh hooks (Git for Windows).
func ShellPath(p string) string {
	p = filepath.Clean(p)
	if runtime.GOOS == "windows" {
		p = strings.ReplaceAll(p, `\`, `/`)
	}
	return p
}

// QuoteForHook quotes a binary path so it survives as a single argument on a
// hook command line. This produces *shell* syntax only; callers splicing the
// result into a JSON manifest string must additionally run it through
// EscapeJSONString to escape the backslashes and quotes it may contain.
func QuoteForHook(p string) string {
	if p == "" {
		return `""`
	}
	if runtime.GOOS == "windows" {
		// cmd.exe has no escape for a literal quote inside a quoted token,
		// so only wrap when whitespace makes it necessary.
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

// EscapeJSONString escapes s for use inside a JSON string literal, without
// adding the surrounding quotes. Windows paths make this mandatory: a raw
// `C:\Users\...` spliced into a manifest yields `\U`, which is not a valid
// JSON escape, and an unescaped `"` would terminate the string early.
func EscapeJSONString(s string) string {
	encoded, err := json.Marshal(s)
	if err != nil {
		// json.Marshal never fails for a string; fall back to the two
		// escapes that actually matter rather than dropping the value.
		out := strings.ReplaceAll(s, `\`, `\\`)
		return strings.ReplaceAll(out, `"`, `\"`)
	}
	// Strip the quotes json.Marshal adds; the caller supplies its own.
	return string(encoded[1 : len(encoded)-1])
}

// IsSoBinary reports whether base name is so or so.exe.
func IsSoBinary(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return base == "so" || base == "so.exe"
}

// LookPathSo finds so or so.exe on PATH. On Windows it tries so.exe first
// then so, matching PATHEXT-aware lookup without depending on it.
func LookPathSo() (string, error) {
	name := "so"
	if runtime.GOOS == "windows" {
		name = "so.exe"
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	if runtime.GOOS == "windows" {
		if p, err := exec.LookPath("so"); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s binary not found on PATH", name)
}

// MentionsCommand reports whether s includes a so or so.exe invocation of sub.
func MentionsCommand(s, sub string) bool {
	sub = strings.TrimSpace(sub)
	if sub == "" {
		return false
	}
	return strings.Contains(s, "so "+sub) || strings.Contains(s, "so.exe "+sub)
}

// ResolveSoBin is the absolute so binary to paste into the host shell tool, or
// "so"/"so.exe" when this process is not the CLI (tests). Prefer the running
// executable so hooks and SessionStart match `so install`, then SUPEROPEN_SO_BIN,
// then PATH name.
func ResolveSoBin() string {
	if exe, err := os.Executable(); err == nil && IsSoBinary(exe) {
		if abs, err := filepath.Abs(exe); err == nil {
			return QuoteForHook(abs)
		}
		return QuoteForHook(exe)
	}
	if v := strings.TrimSpace(os.Getenv("SUPEROPEN_SO_BIN")); v != "" {
		return QuoteForHook(v)
	}
	if runtime.GOOS == "windows" {
		return "so.exe"
	}
	return "so"
}
