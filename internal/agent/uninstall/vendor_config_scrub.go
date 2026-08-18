// File-level scrubbers that run after the vendor-CLI based uninstall
// steps. Claude Code and Codex CLIs can leave Superopen-owned state in
// on-disk config even after their uninstall commands succeed. These
// scrubbers delete only Superopen-owned keys/sections and preserve
// everything else. Missing files are a no-op.
package uninstall

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// stripClaudeMarketplaceJSON removes the Superopen marketplace entry from
// `~/.claude/plugins/known_marketplaces.json` when present. The marketplace
// key matches the current install registration name ("so").
func stripClaudeMarketplaceJSON(dryRun bool) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(home, ".claude", "plugins", "known_marketplaces.json")
	raw, err := os.ReadFile(path) //nolint:gosec // path is derived from $HOME
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}
	if _, ok := data["so"]; !ok {
		return "", nil
	}
	if dryRun {
		return path, nil
	}
	delete(data, "so")
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return path, err
	}
	out = append(out, '\n')
	if err := writeFileAtomic(path, out, 0o600); err != nil {
		return path, err
	}
	return path, nil
}

// codexOwnedSectionRe matches every TOML section header that Superopen owns.
var codexOwnedSectionRe = regexp.MustCompile(
	`^\[(?:` +
		`marketplaces\.superopen` +
		`|plugins\."superopen@superopen"` +
		`|hooks\.state\."superopen@superopen:[^"]*"` +
		`)\]\s*$`,
)

var codexAnySectionRe = regexp.MustCompile(`^\[[^\]]+\]\s*$`)

// stripCodexConfigTOML rewrites `~/.codex/config.toml` so that every
// Superopen-owned section is removed.
func stripCodexConfigTOML(dryRun bool) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(home, ".codex", "config.toml")
	raw, err := os.ReadFile(path) //nolint:gosec // path is derived from $HOME
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	rewritten, changed := stripCodexOwnedSections(string(raw))
	if !changed {
		return "", nil
	}
	if dryRun {
		return path, nil
	}
	if err := writeFileAtomic(path, []byte(rewritten), 0o600); err != nil {
		return path, err
	}
	return path, nil
}

func stripCodexOwnedSections(src string) (string, bool) {
	sep := "\n"
	if strings.Contains(src, "\r\n") && !strings.Contains(strings.ReplaceAll(src, "\r\n", ""), "\n") {
		sep = "\r\n"
	}
	lines := strings.Split(src, sep)

	out := make([]string, 0, len(lines))
	drop := false
	changed := false
	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		if codexAnySectionRe.MatchString(trimmed) {
			if codexOwnedSectionRe.MatchString(trimmed) {
				drop = true
				changed = true
				continue
			}
			drop = false
		}
		if drop {
			continue
		}
		out = append(out, line)
	}

	joined := strings.Join(out, sep)
	collapsePattern := regexp.MustCompile(`(?:` + regexp.QuoteMeta(sep) + `){3,}`)
	joined = collapsePattern.ReplaceAllString(joined, sep+sep)
	return joined, changed
}
