package install

// Additional vendors beyond the embedded marketplace trees get full
// hook manifests written to the user's config dirs (Gemini settings,
// OpenCode/Pi TypeScript plugins, Copilot CLI hooks).

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ishanjainn/superopen/internal/userpaths"
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
	var path string
	var body string

	switch vendor {
	case "gemini":
		path = filepath.Join(home, ".gemini", "settings.json")
		if dryRun {
			return []string{path}, nil
		}
		return installGeminiHooks(path, soBin)

	case "opencode":
		// OpenCode auto-loads ~/.config/opencode/plugins/*.ts
		// https://opencode.ai/docs/plugins/
		// Host event model: OpenCode plugin events (session.*, message.*, tool.*).
		base, pathErr := userpaths.OpenCodeConfigDir()
		if pathErr != nil {
			return nil, pathErr
		}
		path = filepath.Join(base, "plugins", "superopen.ts")
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
		base, pathErr := userpaths.CopilotHome()
		if pathErr != nil {
			return nil, pathErr
		}
		path = filepath.Join(base, "hooks", "superopen.json")
		body, err = copilotManifest(soBin)
		if err != nil {
			return nil, err
		}

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

func hookCommand(soBin, args string) string {
	return shellQuote(soBin) + " " + args
}

func copilotManifest(soBin string) (string, error) {
	hook := func(args string) map[string]any {
		posixPath := userpaths.ShellPath(soBin)
		bash := posixQuote(posixPath) + " " + args
		powershell := "& '" + strings.ReplaceAll(soBin, "'", "''") + "' " + args
		return map[string]any{
			"type": "command", "bash": bash, "powershell": powershell, "timeoutSec": 15,
		}
	}
	doc := map[string]any{
		"version": 1,
		"hooks": map[string]any{
			"sessionStart":          []any{hook("coding hook --vendor=copilot-cli --event=sessionStart")},
			"sessionEnd":            []any{hook("coding hook --vendor=copilot-cli --event=sessionEnd"), hook("sessions finalize --detach")},
			"userPromptSubmitted":   []any{hook("coding hook --vendor=copilot-cli --event=userPromptSubmitted")},
			"userPromptTransformed": []any{hook("coding hook --vendor=copilot-cli --event=userPromptTransformed")},
			"preToolUse":            []any{hook("coding hook --vendor=copilot-cli --event=preToolUse")},
			"postToolUse":           []any{hook("coding hook --vendor=copilot-cli --event=postToolUse")},
			"postToolUseFailure":    []any{hook("coding hook --vendor=copilot-cli --event=postToolUseFailure")},
			"errorOccurred":         []any{hook("coding hook --vendor=copilot-cli --event=errorOccurred")},
			"agentStop":             []any{hook("coding hook --vendor=copilot-cli --event=agentStop")},
		},
	}
	body, err := json.MarshalIndent(doc, "", "  ")
	return string(append(body, '\n')), err
}

func posixQuote(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t\n'\"\\$`") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func installGeminiHooks(path, soBin string) ([]string, error) {
	doc := map[string]any{}
	if previous, err := os.ReadFile(path); err == nil && len(previous) > 0 {
		if err := json.Unmarshal(previous, &doc); err != nil {
			return nil, fmt.Errorf("parse existing %s: %w (refusing to overwrite invalid JSON)", path, err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	hooks, _ := doc["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	for event, commands := range geminiCommands(soBin) {
		existing, _ := hooks[event].([]any)
		existing = stripOwnedGeminiGroups(existing)
		entries := make([]any, 0, len(commands))
		for _, command := range commands {
			entry := map[string]any{"type": "command", "command": command, "timeout": 15000}
			entries = append(entries, entry)
		}
		existing = append(existing, map[string]any{"hooks": entries, "sequential": true})
		hooks[event] = existing
	}
	// Remove obsolete Superopen event groups written by older releases.
	for _, event := range []string{"UserPromptSubmit", "PreToolUse"} {
		if groups, ok := hooks[event].([]any); ok {
			groups = stripOwnedGeminiGroups(groups)
			if len(groups) == 0 {
				delete(hooks, event)
			} else {
				hooks[event] = groups
			}
		}
	}
	doc["hooks"] = hooks
	body, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
		return nil, err
	}
	return []string{path}, nil
}

func geminiCommands(soBin string) map[string][]string {
	command := func(event string) string {
		return hookCommand(soBin, "coding hook --vendor=gemini --event="+event)
	}
	return map[string][]string{
		"SessionStart": {command("SessionStart")},
		"SessionEnd":   {command("SessionEnd"), hookCommand(soBin, "sessions finalize --detach")},
		"BeforeAgent":  {command("BeforeAgent")},
		"AfterAgent":   {command("AfterAgent")},
		"BeforeTool":   {command("BeforeTool")},
		"AfterTool":    {command("AfterTool")},
	}
}

func stripOwnedGeminiGroups(groups []any) []any {
	out := make([]any, 0, len(groups))
	for _, rawGroup := range groups {
		group, ok := rawGroup.(map[string]any)
		if !ok {
			out = append(out, rawGroup)
			continue
		}
		entries, ok := group["hooks"].([]any)
		if !ok {
			out = append(out, rawGroup)
			continue
		}
		kept := make([]any, 0, len(entries))
		for _, rawEntry := range entries {
			entry, _ := rawEntry.(map[string]any)
			command, _ := entry["command"].(string)
			if strings.Contains(command, "coding hook --vendor=gemini") || strings.Contains(command, "sessions finalize") {
				continue
			}
			kept = append(kept, rawEntry)
		}
		if len(kept) > 0 {
			clone := make(map[string]any, len(group))
			for key, value := range group {
				clone[key] = value
			}
			clone["hooks"] = kept
			out = append(out, clone)
		}
	}
	return out
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
