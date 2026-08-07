// Per-vendor manifest writers. The actual manifest contents live under
// .claude-plugin/ + plugins/ at the repo root and are embedded into the
// binary via go:embed as a single assembled tree under marketplace/.
// installVendor writes the appropriate subset to the user's home directory
// for each vendor.

package install

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/userpaths"
)

// marketplaceFS holds the embedded Claude Code marketplace tree. The
// contents are assembled by `cli/scripts/sync-plugins.sh` from the
// two repo-root sources (`.claude-plugin/marketplace.json` and
// `plugins/<vendor>/`) into a single in-tree mirror so the CLI binary
// is self-contained - users don't need to clone the repo to run
// `so coding install`.
//
// Layout inside the embed root matches the repo-root
// layout exactly:
//
//	marketplace/
//	  .claude-plugin/marketplace.json
//	  plugins/claude-code/
//	  plugins/cursor/
//	  plugins/codex/
//
// Keeping the layouts identical means the single marketplace.json
// `source: "./plugins/claude-code"` resolves correctly whether Claude
// fetches the marketplace from GitHub or from the local materialized
// directory.
//
// `all:` prefix is required so go:embed descends into dot-prefixed
// subdirectories (Claude Code's `.claude-plugin/`, Cursor's
// `.cursor-plugin/`, Codex's `.codex-plugin/`).
//
//go:embed all:marketplace
var marketplaceFS embed.FS

// InstallVendor writes the manifest set for one vendor. Exported for so init/sync.
func InstallVendor(vendor string, dryRun bool) ([]string, error) {
	return installVendor(vendor, dryRun)
}

// installVendor writes the manifest set for one vendor to the user's
// home directory. Returns the absolute paths it touched.
func installVendor(vendor string, dryRun bool) ([]string, error) {
	// Cursor is special: its supported way to install agent-wide
	// hooks is to merge into ~/.cursor/hooks.json (user scope), not
	// to drop a plugin tree. See install_cursor.go for the why.
	if vendor == "cursor" {
		return installCursorHooks(dryRun)
	}
	switch vendor {
	case "gemini", "opencode", "copilot-cli", "pi":
		return installGenericVendor(vendor, dryRun)
	}

	dest, err := vendorDestRoot(vendor)
	if err != nil {
		return nil, err
	}
	srcDir := "marketplace/plugins/" + vendor

	soBin, binErr := resolveSoBin()
	if binErr != nil {
		return nil, fmt.Errorf("locate so binary: %w (install so and ensure it is on PATH)", binErr)
	}

	// Codex materializes a whole marketplace tree - wipe stale plugin
	// dirs (e.g. leftover legacy plugin dirs) so the registered marketplace only
	// exposes the current Superopen plugin.
	if !dryRun && vendor == "codex" {
		_ = os.RemoveAll(dest)
	}

	var written []string
	walkErr := fs.WalkDir(marketplaceFS, srcDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(p, srcDir+"/")
		target := filepath.Join(dest, rel)
		written = append(written, target)
		if dryRun {
			return nil
		}
		body, readErr := marketplaceFS.ReadFile(p)
		if readErr != nil {
			return readErr
		}
		body = patchManifestBytes(rel, body, soBin)
		if mkErr := os.MkdirAll(filepath.Dir(target), 0o755); mkErr != nil {
			return mkErr
		}
		return os.WriteFile(target, body, 0o644)
	})
	if walkErr != nil {
		// fs.ErrNotExist means we haven't authored manifests for this
		// vendor yet - surface it as an explicit not-implemented rather
		// than a confusing path error.
		if os.IsNotExist(walkErr) {
			return nil, fmt.Errorf("manifest set not bundled for vendor %q (please file an issue)", vendor)
		}
		return nil, walkErr
	}

	if !dryRun && vendor == "claude-code" {
		if err := enableClaudeCodePlugin(); err != nil {
			fmt.Fprintf(os.Stderr, "so install: could not register Claude Code plugin via `claude` CLI: %v\n", err)
			fmt.Fprintf(os.Stderr, "so install: hooks were still written to %s - in Claude Code run: /plugin marketplace add <path-to-superopen>/plugins then /plugin install superopen@superopen\n", dest)
		}
	}
	if !dryRun && vendor == "codex" {
		if err := enableCodexPlugin(dest); err != nil {
			fmt.Fprintf(os.Stderr, "so install: could not register Codex plugin via `codex` CLI: %v\n", err)
			fmt.Fprintf(os.Stderr, "so install: marketplace was written to %s - in Codex run: codex plugin marketplace add %s then codex plugin add superopen@superopen, then open /hooks inside Codex and trust each hook\n", dest, dest)
		}
	}
	return written, nil
}

// vendorDestRoot returns the directory under $HOME where this vendor's
// manifests must land. We honor each vendor's own conventions:
//
//	Claude Code → ~/.claude/plugins/superopen/ (also cleans legacy plugin ids)
//	Codex       → ~/.local/share/so/codex-marketplace/  (local
//	              marketplace registered via `codex plugin marketplace add`)
//
// Cursor is intentionally absent: it installs by merging into
// ~/.cursor/hooks.json (user scope), not by writing a plugin tree.
// See install_cursor.go for the rationale.
//
// For Codex specifically: dropping files under `~/.codex/plugins/<name>/`
// is NOT how Codex's plugin loader discovers plugins. The loader scans
// configured marketplaces (`codex plugin marketplace list`) and only
// installs plugins by name via `codex plugin add <plugin>@<marketplace>`.
// We therefore materialize a self-contained marketplace tree at a
// stable location and register it via the `codex` CLI in
// enableCodexPlugin() (see install_patch.go).
func vendorDestRoot(vendor string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not resolve home directory: %w", err)
	}
	switch vendor {
	case "claude-code":
		return filepath.Join(home, ".claude", "plugins", "superopen-cc"), nil
	case "codex":
		return userpaths.CodexMarketplaceDir()
	default:
		return "", fmt.Errorf("unknown vendor %q", vendor)
	}
}
