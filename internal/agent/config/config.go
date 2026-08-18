// Package config resolves the CLI's runtime configuration from (in order):
//
//  1. SUPEROPEN_* environment variables
//  2. ~/.config/superopen/config.env on Unix, %APPDATA%\superopen\config.env
//     on Windows; honors $XDG_CONFIG_HOME if set. Allow-listed keys; 0600.
//
// Keep this dead simple: no eval-style sourcing of arbitrary shell, no
// secrets in flags' help text. The config file is a flat KEY=VALUE document
// where only an explicit allow-list of keys is honored.
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Resolved holds the effective configuration after all sources are merged.
type Resolved struct {
	// Environment / ApplicationName flow into the OTel resource attrs.
	Environment     string
	ApplicationName string

	// Source records where each value came from for `so configure
	// --show` and debugging.
	Source map[string]string
}

// Flags holds the values parsed from CLI flags. Pass an empty struct
// when invoked from a context that doesn't expose flags.
type Flags struct{}

// Defaults captures hardcoded fallbacks. Kept as a separate struct so
// tests can replace them without touching env or flags.
type Defaults struct {
	Environment     string
	ApplicationName string
}

func builtinDefaults() Defaults {
	return Defaults{
		Environment:     "default",
		ApplicationName: "so-cli",
	}
}

// Allow-listed keys honored from ~/.config/superopen/config.env. Anything
// else is silently ignored (we never want this file to be a vector for
// arbitrary env-injection on the developer's machine).
var allowedFileKeys = map[string]struct{}{
	"SUPEROPEN_ENVIRONMENT":           {},
	"SUPEROPEN_APPLICATION_NAME":      {},
	"SUPEROPEN_CODING_REPO_ALLOWLIST": {},
	"OTEL_RESOURCE_ATTRIBUTES":        {},
}

// PromoteFileToEnv re-exports the config-file values that downstream
// adapters read directly from os.Getenv (e.g. classifier reading
// SUPEROPEN_CODING_REPO_ALLOWLIST). Without this step, values set in
// ~/.config/superopen/config.env would silently be unavailable to
// per-vendor adapters that bypass the resolved struct. Existing env
// vars take precedence so we never override a real shell setting.
func PromoteFileToEnv() error {
	vals, err := readConfigFile()
	if err != nil {
		return err
	}
	for k, v := range vals {
		if v == "" {
			continue
		}
		if _, exists := os.LookupEnv(k); !exists {
			_ = os.Setenv(k, v)
		}
	}
	return nil
}

// Load resolves config across all sources.
//
// `flags` may be nil when no command-level flags are involved (e.g. the
// hot-path hook subcommand reads only env + file).
func Load(flags *Flags) (*Resolved, error) {
	defaults := builtinDefaults()
	res := &Resolved{Source: map[string]string{}}

	// Step 4: file (lowest priority - gets overridden by env and flags).
	fileVals, err := readConfigFile()
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}
	apply := func(src string, key string, val string) {
		if val == "" {
			return
		}
		switch key {
		case "SUPEROPEN_ENVIRONMENT":
			res.Environment = val
			res.Source["environment"] = src
		case "SUPEROPEN_APPLICATION_NAME":
			res.ApplicationName = val
			res.Source["application_name"] = src
		}
	}

	for k, v := range fileVals {
		apply("file", k, v)
	}

	// SUPEROPEN_* environment variables override the config file.
	for _, k := range []string{
		"SUPEROPEN_ENVIRONMENT",
		"SUPEROPEN_APPLICATION_NAME",
	} {
		if v := os.Getenv(k); v != "" {
			apply("env_superopen", k, v)
		}
	}

	_ = flags

	// Apply defaults for anything still empty.
	if res.Environment == "" {
		res.Environment = defaults.Environment
		res.Source["environment"] = "default"
	}
	if res.ApplicationName == "" {
		res.ApplicationName = defaults.ApplicationName
		res.Source["application_name"] = "default"
	}
	return res, nil
}

// Path returns the absolute path to the config file, even if it doesn't
// yet exist. Used by `so configure` for write-side and by Load for
// read-side.
func Path() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.env"), nil
}

// configDir returns the directory that holds config.env. Honors
// XDG_CONFIG_HOME, falling back to $HOME/.config/superopen/. We do not
// path-traverse the value - env that contains ".." or absolute paths is
// taken at face value here; the consumer (Load/Save) only reads/writes
// the explicit "config.env" leaf which can't escape via traversal.
func configDir() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "superopen"), nil
	}
	// On Windows the idiomatic config root is %APPDATA% (Roaming).
	// os.UserConfigDir returns exactly that on Windows, whereas on
	// macOS/Linux it returns paths we deliberately do NOT want
	// (~/Library/Application Support and ~/.config respectively -
	// the latter matches but is also what we already use, and on
	// macOS we want the XDG-style ~/.config/superopen/ for parity with
	// Linux so users moving between hosts find the same file).
	if runtime.GOOS == "windows" {
		if cfg, err := os.UserConfigDir(); err == nil && cfg != "" {
			return filepath.Join(cfg, "superopen"), nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config", "superopen"), nil
}

// readConfigFile parses ~/.config/superopen/config.env if it exists. Returns
// an empty map (no error) if the file is missing - that's the expected
// state on a fresh install.
func readConfigFile() (map[string]string, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path) //nolint:gosec // path is constructed from $HOME, not user input
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck

	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		if _, allowed := allowedFileKeys[key]; !allowed {
			continue
		}
		out[key] = val
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Save writes the supplied key/value pairs to ~/.config/superopen/config.env
// with mode 0600. Existing keys are preserved unless overridden; unknown
// (non-allowlisted) keys are dropped silently.
func Save(updates map[string]string) (string, error) {
	path, err := Path()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}

	existing, err := readConfigFile()
	if err != nil {
		return "", err
	}
	for k, v := range updates {
		if _, ok := allowedFileKeys[k]; !ok {
			continue
		}
		if v == "" {
			delete(existing, k)
			continue
		}
		existing[k] = v
	}

	// Stable ordering: sort keys alphabetically so diffs are clean.
	var lines []string
	lines = append(lines,
		"# so CLI config - written by `so configure`.",
		"# Allow-listed keys only; anything else is ignored at read time.",
		"",
	)
	keys := make([]string, 0, len(existing))
	for k := range existing {
		keys = append(keys, k)
	}
	sortStrings(keys)
	for _, k := range keys {
		lines = append(lines, fmt.Sprintf("%s=%s", k, existing[k]))
	}

	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// sortStrings is an inline sort helper to avoid pulling in the `sort`
// package indirectly via the test-runner; the cost is one tiny function.
func sortStrings(s []string) {
	// Insertion sort - list is always small (<20 keys).
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
