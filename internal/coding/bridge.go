package coding

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/coding/install"
	"github.com/ishanjainn/superopen/internal/userpaths"
)

// Install installs coding-agent observability for the given vendors.
// endpoint is best-effort written to the platform config.env when non-empty.
// pluginRoot is unused (manifests are embedded); kept for call-site compatibility.
func Install(repoRoot, endpoint string, vendors []string, pluginRoot string) error {
	_ = repoRoot
	_ = pluginRoot
	if endpoint != "" {
		_ = writeEndpointConfig(endpoint)
	}
	targets := vendors
	if len(targets) == 0 {
		targets = []string{"claude-code", "cursor", "codex", "gemini", "opencode", "copilot-cli", "pi"}
	}
	for _, v := range targets {
		switch v {
		case "claude", "claude-code":
			v = "claude-code"
		}
		if _, err := install.InstallVendor(v, false); err != nil {
			return fmt.Errorf("install %s: %w", v, err)
		}
	}
	return nil
}

// Status reports whether each vendor looks installed.
func Status(repoRoot string, vendors []string) map[string]bool {
	_ = repoRoot
	out := map[string]bool{}
	home, err := os.UserHomeDir()
	if err != nil {
		return out
	}
	codexDir, _ := userpaths.CodexMarketplaceDir()
	legacySO, _ := userpaths.LegacyDataDirSO()

	for _, v := range vendors {
		switch v {
		case "claude", "claude-code":
			_, e1 := os.Stat(filepath.Join(home, ".claude", "plugins", "superopen-cc"))
			out["claude-code"] = e1 == nil
		case "cursor":
			data, e := os.ReadFile(filepath.Join(home, ".cursor", "hooks.json"))
			out["cursor"] = e == nil && (strings.Contains(string(data), "so coding hook --vendor=cursor") ||
				strings.Contains(string(data), " coding hook --vendor=cursor"))
		case "codex":
			ok := false
			for _, p := range []string{
				codexDir,
				filepath.Join(legacySO, "codex-marketplace"),
			} {
				if p == "" {
					continue
				}
				if _, e := os.Stat(p); e == nil {
					ok = true
					break
				}
			}
			out["codex"] = ok
		case "gemini":
			data, e := os.ReadFile(filepath.Join(home, ".gemini", "settings.json"))
			out["gemini"] = e == nil && strings.Contains(string(data), "coding hook --vendor=gemini")
		case "opencode":
			_, e := os.Stat(filepath.Join(home, ".opencode", "plugins", "superopen.ts"))
			out["opencode"] = e == nil
		case "copilot-cli", "copilot":
			data, e := os.ReadFile(filepath.Join(home, ".github", "hooks", "superopen.json"))
			out["copilot-cli"] = e == nil && strings.Contains(string(data), "coding hook --vendor=copilot")
		case "pi":
			_, e := os.Stat(filepath.Join(home, ".pi", "extensions", "superopen", "index.ts"))
			out["pi"] = e == nil
		}
	}
	return out
}

func writeEndpointConfig(endpoint string) error {
	dir, err := userpaths.ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "config.env")
	if prev, err := os.ReadFile(path); err == nil {
		s := string(prev)
		if strings.Contains(s, "SUPEROPEN_OTLP_ENDPOINT=") {
			return nil
		}
	}
	body := "# written by so init / so coding install\nSUPEROPEN_OTLP_ENDPOINT=" + endpoint + "\n"
	return os.WriteFile(path, []byte(body), 0o600)
}
