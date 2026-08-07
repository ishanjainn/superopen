package install

// Additional vendors beyond the embedded marketplace trees get full
// hook manifests written to the user's config dirs (Gemini settings,
// OpenCode/Pi TypeScript plugins, Copilot CLI hooks).

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func installGenericVendor(vendor string, dryRun bool) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	soBin, err := resolveSoBin()
	if err != nil {
		return nil, err
	}
	q := strconv.Quote

	var path string
	var body string

	switch vendor {
	case "gemini":
		path = filepath.Join(home, ".gemini", "settings.json")
		body = fmt.Sprintf(`{
  "hooks": {
    "SessionStart": [{"hooks": [{"type": "command", "command": %s}]}],
    "SessionEnd": [{"hooks": [
      {"type": "command", "command": %s},
      {"type": "command", "command": %s, "timeout": 5}
    ]}],
    "UserPromptSubmit": [{"hooks": [{"type": "command", "command": %s}]}],
    "PreToolUse": [{"hooks": [{"type": "command", "command": %s}]}]
  }
}
`, q(soBin+" coding hook --vendor=gemini --event=SessionStart"),
			q(soBin+" coding hook --vendor=gemini --event=SessionEnd"),
			q(soBin+" sessions finalize --detach"),
			q(soBin+" coding hook --vendor=gemini --event=UserPromptSubmit"),
			q(soBin+" coding hook --vendor=gemini --event=PreToolUse"))

	case "opencode":
		// OpenCode auto-loads ~/.config/opencode/plugins/*.ts
		// https://opencode.ai/docs/plugins/
		// Host event model: OpenCode plugin events (session.*, message.*, tool.*).
		path = filepath.Join(home, ".config", "opencode", "plugins", "superopen.ts")
		raw, readErr := marketplaceFS.ReadFile("marketplace/plugins/opencode/superopen.ts")
		if readErr != nil {
			return nil, fmt.Errorf("read embedded opencode plugin: %w", readErr)
		}
		body = patchPluginSoBin(string(raw), soBin)

	case "pi":
		// Pi auto-loads ~/.pi/agent/extensions/*/index.ts
		// https://pi.dev/docs/latest/extensions
		// Host event model: Pi extension events (incl. turn_end).
		path = filepath.Join(home, ".pi", "agent", "extensions", "superopen", "index.ts")
		raw, readErr := marketplaceFS.ReadFile("marketplace/plugins/pi/index.ts")
		if readErr != nil {
			return nil, fmt.Errorf("read embedded pi extension: %w", readErr)
		}
		body = patchPluginSoBin(string(raw), soBin)

	case "copilot-cli":
		// Copilot CLI loads ~/.copilot/hooks/*.json,
		// not ~/.github/hooks (cloud-agent style).
		path = filepath.Join(home, ".copilot", "hooks", "superopen.json")
		body = fmt.Sprintf(`{
  "hooks": {
    "sessionStart": [{"type": "command", "command": %s}],
    "sessionEnd": [
      {"type": "command", "command": %s},
      {"type": "command", "command": %s}
    ],
    "userPromptSubmitted": [{"type": "command", "command": %s}],
    "preToolUse": [{"type": "command", "command": %s}],
    "postToolUse": [{"type": "command", "command": %s}],
    "postToolUseFailure": [{"type": "command", "command": %s}],
    "errorOccurred": [{"type": "command", "command": %s}],
    "agentStop": [
      {"type": "command", "command": %s},
      {"type": "command", "command": %s}
    ]
  }
}
`, q(soBin+" coding hook --vendor=copilot-cli --event=sessionStart"),
			q(soBin+" coding hook --vendor=copilot-cli --event=sessionEnd"),
			q(soBin+" sessions finalize --detach"),
			q(soBin+" coding hook --vendor=copilot-cli --event=userPromptSubmitted"),
			q(soBin+" coding hook --vendor=copilot-cli --event=preToolUse"),
			q(soBin+" coding hook --vendor=copilot-cli --event=postToolUse"),
			q(soBin+" coding hook --vendor=copilot-cli --event=postToolUseFailure"),
			q(soBin+" coding hook --vendor=copilot-cli --event=errorOccurred"),
			q(soBin+" coding hook --vendor=copilot-cli --event=agentStop"),
			q(soBin+" sessions finalize --detach"))

	default:
		return nil, fmt.Errorf("unsupported generic vendor %q", vendor)
	}

	if dryRun {
		return []string{path}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return nil, err
	}
	written := []string{path}
	for _, stale := range staleGenericVendorPaths(home, vendor) {
		if stale == path {
			continue
		}
		if err := os.RemoveAll(stale); err == nil {
			written = append(written, "removed-stale:"+stale)
		}
	}
	return written, nil
}

// patchPluginSoBin pins the absolute so binary in TypeScript plugins that
// resolve via SUPEROPEN_SO_BIN || "so".
func patchPluginSoBin(src, soBin string) string {
	quoted := strconv.Quote(soBin)
	out := src
	out = strings.ReplaceAll(out,
		`return process.env.SUPEROPEN_SO_BIN?.trim() || "so";`,
		`return process.env.SUPEROPEN_SO_BIN?.trim() || `+quoted+`;`,
	)
	out = strings.ReplaceAll(out,
		`return process.env.SUPEROPEN_SO_BIN?.trim() || 'so';`,
		`return process.env.SUPEROPEN_SO_BIN?.trim() || `+quoted+`;`,
	)
	return out
}

// staleGenericVendorPaths are older install locations hosts never load.
func staleGenericVendorPaths(home, vendor string) []string {
	switch vendor {
	case "opencode":
		return []string{filepath.Join(home, ".opencode", "plugins", "superopen.ts")}
	case "pi":
		return []string{filepath.Join(home, ".pi", "extensions", "superopen")}
	case "copilot-cli":
		return []string{filepath.Join(home, ".github", "hooks", "superopen.json")}
	default:
		return nil
	}
}
