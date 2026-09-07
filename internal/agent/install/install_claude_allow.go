package install

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ishanjainn/superopen/internal/paths"
)

// Claude Code is the only shipped host with a command allowlist
// (settings.json permissions.allow). Codex, Gemini, Copilot CLI, Cursor,
// OpenCode, and Pi were inspected: they have no equivalent Bash(so:*)
// surface, so Phase 0 for those hosts is argv pinning only.
//
// StripClaudeAllowlist removes Superopen-owned Bash allow rules from Claude
// user settings. Foreign allow entries are kept. Unparseable JSON is left.
func StripClaudeAllowlist(dryRun bool) ([]string, error) {
	targets, err := claudeSettingsPaths()
	if err != nil {
		return nil, err
	}
	var touched []string
	for _, path := range targets {
		changed, err := stripClaudeAllowlistFile(path, dryRun)
		if err != nil {
			return touched, err
		}
		if changed {
			touched = append(touched, path)
		}
	}
	return touched, nil
}

func stripClaudeAllowlistFile(path string, dryRun bool) (bool, error) {
	previous, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if len(previous) == 0 {
		return false, nil
	}
	doc := map[string]any{}
	if err := json.Unmarshal(previous, &doc); err != nil {
		return false, fmt.Errorf("parse %s: %w (leaving the file untouched)", path, err)
	}
	perms, _ := doc["permissions"].(map[string]any)
	if perms == nil {
		return false, nil
	}
	allow := jsonStringSlice(perms["allow"])
	kept := stripSuperopenAllowRules(allow)
	if len(kept) == len(allow) {
		return false, nil
	}
	if dryRun {
		return true, nil
	}
	if len(kept) == 0 {
		delete(perms, "allow")
	} else {
		perms["allow"] = kept
	}
	if len(perms) == 0 {
		delete(doc, "permissions")
	} else {
		doc["permissions"] = perms
	}
	body, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func installClaudeAllowlist(soBin string, dryRun bool) ([]string, error) {
	targets, err := claudeSettingsPaths()
	if err != nil {
		return nil, err
	}
	if dryRun {
		return targets, nil
	}
	var written []string
	for _, path := range targets {
		if err := mergeClaudeAllowlist(path, soBin); err != nil {
			return written, err
		}
		written = append(written, path)
	}
	return written, nil
}

func claudeSettingsPaths() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	var out []string
	add := func(p string) {
		p = filepath.Clean(p)
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	add(filepath.Join(home, ".claude", "settings.json"))
	if cfg, err := paths.ClaudeConfigDir(); err == nil {
		add(filepath.Join(cfg, "settings.json"))
	}
	if runtime.GOOS == "windows" {
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			add(filepath.Join(local, "claude", "settings.json"))
		}
	}
	return out, nil
}

func mergeClaudeAllowlist(path, soBin string) error {
	doc := map[string]any{}
	previous, err := os.ReadFile(path)
	if err == nil && len(previous) > 0 {
		if err := json.Unmarshal(previous, &doc); err != nil {
			return fmt.Errorf("parse existing %s: %w (refusing to overwrite invalid JSON)", path, err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	perms, _ := doc["permissions"].(map[string]any)
	if perms == nil {
		perms = map[string]any{}
	}
	allow := jsonStringSlice(perms["allow"])
	allow = stripSuperopenAllowRules(allow)
	for _, rule := range soAllowRules(soBin) {
		if !containsStringFold(allow, rule) {
			allow = append(allow, rule)
		}
	}
	perms["allow"] = allow
	doc["permissions"] = perms
	body, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0o644)
}

func soAllowRules(soBin string) []string {
	rules := []string{"Bash(so:*)", "Bash(so.exe:*)"}
	bin := strings.TrimSpace(soBin)
	if bin == "" {
		return rules
	}
	slash := filepath.ToSlash(bin)
	rules = append(rules, "Bash("+slash+":*)")
	if slash != bin {
		rules = append(rules, "Bash("+bin+":*)")
	}
	return uniqueStrings(rules)
}

func stripSuperopenAllowRules(allow []string) []string {
	out := make([]string, 0, len(allow))
	for _, rule := range allow {
		if isSuperopenAllowRule(rule) {
			continue
		}
		out = append(out, rule)
	}
	return out
}

func isSuperopenAllowRule(rule string) bool {
	rule = strings.TrimSpace(rule)
	switch rule {
	case "Bash(so:*)", "Bash(so.exe:*)":
		return true
	}
	inner, ok := strings.CutPrefix(rule, "Bash(")
	if !ok || !strings.HasSuffix(inner, ":*)") {
		return false
	}
	path := strings.TrimSuffix(inner, ":*)")
	base := filepath.Base(filepath.ToSlash(path))
	return base == "so" || base == "so.exe"
}

func jsonStringSlice(raw any) []string {
	switch v := raw.(type) {
	case []string:
		return append([]string{}, v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func containsStringFold(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
