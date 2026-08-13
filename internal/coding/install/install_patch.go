package install

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ishanjainn/superopen/internal/userpaths"
)

// patchManifestBytes rewrites hook manifests so they do not depend on the
// caller's PATH. GUI apps (VS Code, Cursor) often launch hook subprocesses
// with a minimal PATH that omits ~/.local/bin. Replacing the bare
// `so` command with the absolute path of the binary that ran
// `so coding install` removes that whole class of "hooks fire but
// don't find the CLI" failures.
// The `so ...` commands being rewritten live inside JSON string values (e.g.
// `"command": "so coding hook --vendor=cursor"`), so the replacement is
// escaped for that context. Skipping this breaks every Windows install: a raw
// `C:\Users\...` produces the invalid JSON escape `\U`, and a shell-quoted
// path's leading `"` terminates the string value early.
func patchManifestBytes(name string, body []byte, soBin string) []byte {
	s := string(body)
	jsonManifest := strings.HasSuffix(strings.ToLower(name), ".json")
	if strings.Contains(s, "__SO_BIN__") {
		bin := soBin
		if jsonManifest {
			bin = userpaths.EscapeJSONString(bin)
		}
		s = strings.ReplaceAll(s, "__SO_BIN__", bin)
	}
	quoted := shellQuote(soBin)
	if jsonManifest {
		quoted = userpaths.EscapeJSONString(quoted)
	}
	for _, verb := range []string{"so coding hook", "so sessions finalize", "so sessions refresh"} {
		suffix := strings.TrimPrefix(verb, "so")
		s = strings.ReplaceAll(s, verb, quoted+suffix)
	}
	return []byte(s)
}

func resolveSoBin() (string, error) {
	if exe, err := os.Executable(); err == nil {
		if userpaths.IsSoBinary(exe) {
			return exe, nil
		}
	}
	if p, err := exec.LookPath("so"); err == nil {
		return p, nil
	}
	return exec.LookPath("so.exe")
}

func shellQuote(s string) string {
	return userpaths.QuoteForHook(s)
}

// enableClaudeCodePlugin registers the bundled marketplace and installs
// the Superopen Claude plugin for the user. Best-effort when `claude` is missing.
//
// Skipping silently is only correct for "claude isn't installed on this
// machine" - everything else (missing embedded template, can't resolve
// our own binary) is a real failure that the caller surfaces via stderr.
// Swallowing those would leave the on-disk plugin orphaned because Claude
// Code only loads plugins it knows about via a marketplace, which is
// the entire point of materializing one here.
func enableClaudeCodePlugin() error {
	claudeBin, err := exec.LookPath("claude")
	if err != nil {
		return nil
	}

	soBin, err := resolveSoBin()
	if err != nil {
		return fmt.Errorf("resolve so binary: %w", err)
	}
	marketplaceRoot, err := materializeClaudeMarketplace(soBin)
	if err != nil {
		return fmt.Errorf("materialize claude marketplace: %w", err)
	}

	add := exec.Command(claudeBin, "plugin", "marketplace", "add", marketplaceRoot) //nolint:gosec
	if out, err := add.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" && !strings.Contains(strings.ToLower(msg), "already") {
			fmt.Fprintf(os.Stderr, "so install: claude marketplace add: %s\n", msg)
		}
	}

	inst := exec.Command(claudeBin, "plugin", "install", "superopen-cc@superopen", "--scope", "user") //nolint:gosec
	if out, err := inst.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if strings.Contains(strings.ToLower(msg), "already") {
			return nil
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// materializeClaudeMarketplace writes the embedded Claude Code
// marketplace tree under the platform data dir so
// `claude plugin marketplace add` has a stable directory path.
func materializeClaudeMarketplace(soBin string) (string, error) {
	base, err := userpaths.DataDir()
	if err != nil {
		return "", err
	}
	root := filepath.Join(base, "claude-marketplace")
	// Wipe a stale tree so removals in the embed (e.g. a vendor dir
	// being dropped) propagate. The marketplace dir is owned wholly
	// by us; no user data lives here.
	if err := os.RemoveAll(root); err != nil {
		return "", fmt.Errorf("clean %s: %w", root, err)
	}
	if err := extractEmbeddedDir("marketplace", root, soBin); err != nil {
		return "", err
	}
	return root, nil
}

// enableCodexPlugin registers the freshly written Superopen marketplace
// with Codex and installs the `superopen@superopen` plugin so the hooks
// configured in plugin.json actually fire. Best-effort: if the `codex`
// CLI is missing we fall through with a helpful stderr note and
// expect the user to register the marketplace manually.
//
// Codex requires this two-step dance because its plugin loader does NOT
// auto-discover plugin directories - it only scans configured
// marketplaces (`codex plugin marketplace list`) and installed plugins
// (`codex plugin list`). The path `~/.codex/plugins/<name>/` is a
// red herring: nothing in Codex reads it, even if `plugin.json` and
// `hooks.json` are present there.
//
// After this call returns successfully the plugin's hooks are wired
// but Codex still requires a one-time `/hooks` review inside the TUI
// (a security measure) - we surface that requirement in the install
// command's stdout summary.
func enableCodexPlugin(marketplaceRoot string) error {
	codexBin, err := resolveCodexBin()
	if err != nil {
		return err
	}

	// 1. Register (or refresh) the marketplace. Re-running this is
	//    idempotent in newer Codex builds - older builds error with a
	//    "marketplace already exists" message that we swallow.
	add := exec.Command(codexBin, "plugin", "marketplace", "add", marketplaceRoot) //nolint:gosec
	if out, err := add.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		lower := strings.ToLower(msg)
		// "already" → idempotent add, not a real error.
		// "remote_plugin" / "feature flag" → newer builds gate the
		//    git/remote source flow, which we don't need (we pass a
		//    local path), so any other failure mode is what we want
		//    to surface.
		if msg != "" && !strings.Contains(lower, "already") {
			return fmt.Errorf("codex plugin marketplace add: %s", msg)
		}
	}

	// 2. Install the plugin from the just-added marketplace. The slug
	//    is `<plugin_name>@<marketplace_name>`, both of which come
	//    from the JSON we ship (`name: "superopen"` in
	//    `.codex-plugin/plugin.json` and `.agents/plugins/marketplace.json`).
	const slug = "superopen@superopen"
	inst := exec.Command(codexBin, "plugin", "add", slug) //nolint:gosec
	if out, err := inst.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if strings.Contains(strings.ToLower(msg), "already") {
			return nil
		}
		return fmt.Errorf("codex plugin add %s: %s", slug, msg)
	}
	return nil
}

// resolveCodexBin finds the codex binary in $PATH, then falls back to
// the Codex.app default install location used by the macOS GUI app.
// Codex on macOS doesn't symlink itself into /usr/local/bin out of the
// box, which is why an `so coding install --vendor=codex` from a
// shell where `codex` isn't on $PATH would silently no-op without
// this fallback.
func resolveCodexBin() (string, error) {
	if path, err := exec.LookPath("codex"); err == nil {
		return path, nil
	}
	fallbacks := []string{
		"/Applications/Codex.app/Contents/Resources/codex",
	}
	if home, err := os.UserHomeDir(); err == nil {
		codexName := "codex"
		if runtime.GOOS == "windows" {
			codexName = "codex.exe"
		}
		codexHome, _ := userpaths.CodexHome()
		fallbacks = append(fallbacks,
			filepath.Join(home, "Applications", "Codex.app", "Contents", "Resources", "codex"),
			filepath.Join(codexHome, "bin", codexName),
		)
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			fallbacks = append(fallbacks,
				filepath.Join(local, "Programs", "Codex", "codex.exe"),
				filepath.Join(local, "codex", "codex.exe"),
			)
		}
	}
	for _, p := range fallbacks {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("codex binary not found on PATH or in a standard Codex installation")
}

func extractEmbeddedDir(src, dest, soBin string) error {
	return fs.WalkDir(marketplaceFS, src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(p, src+"/")
		target := filepath.Join(dest, rel)
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
}
