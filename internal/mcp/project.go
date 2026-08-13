// Package mcp projects committed Superopen MCP policy into vendor config files.
package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/harness"
)

// Project merges cfg.MCP.Servers into known project-scoped vendor MCP files.
// Never writes user-global configs (~/.mcp.json, ~/.cursor/mcp.json).
func Project(repoRoot string, cfg config.Config) error {
	if len(cfg.MCP.Servers) == 0 {
		return nil
	}
	servers := toServerMap(cfg.MCP.Servers)
	enabled := map[string]bool{}
	for _, v := range cfg.Vendors.Enabled {
		if k := harness.NormalizeVendorKind(v); k != "" {
			enabled[k] = true
		}
	}
	// Always project root .mcp.json when any Claude-like agent is enabled or
	// when no vendors are listed yet (fresh init before detect finishes).
	if len(enabled) == 0 || enabled["claude"] || enabled["codex"] || enabled["gemini"] || enabled["opencode"] || enabled["pi"] || enabled["copilot"] {
		if err := mergeFile(repoRoot, filepath.Join(repoRoot, ".mcp.json"), servers); err != nil {
			return err
		}
	}
	if len(enabled) == 0 || enabled["cursor"] {
		cursorDir := filepath.Join(repoRoot, ".cursor")
		if err := os.MkdirAll(cursorDir, 0o755); err != nil {
			return err
		}
		if err := mergeFile(repoRoot, filepath.Join(cursorDir, "mcp.json"), servers); err != nil {
			return err
		}
	}
	return nil
}

func toServerMap(list []config.MCPServer) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, s := range list {
		name := strings.TrimSpace(s.Name)
		if name == "" || strings.TrimSpace(s.Command) == "" {
			continue
		}
		// Never ship memory MCP — Superopen memory replaces it.
		if strings.EqualFold(name, "memory") || strings.EqualFold(name, "memory-mcp") {
			continue
		}
		entry := map[string]any{
			"command": strings.TrimSpace(s.Command),
		}
		if len(s.Args) > 0 {
			args := make([]string, 0, len(s.Args))
			skip := false
			for _, a := range s.Args {
				a = strings.TrimSpace(a)
				if a == "" {
					continue
				}
				// Refuse unpinned @latest packages in committed policy.
				if strings.Contains(a, "@latest") {
					skip = true
					break
				}
				args = append(args, a)
			}
			if skip {
				continue
			}
			if len(args) > 0 {
				entry["args"] = args
			}
		}
		out[name] = entry
	}
	return out
}

func mergeFile(repoRoot, path string, servers map[string]map[string]any) error {
	if err := rejectUserGlobalPath(repoRoot, path); err != nil {
		return err
	}
	doc := map[string]any{}
	if data, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(data))) > 0 {
		if err := json.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
	}
	existing, _ := doc["mcpServers"].(map[string]any)
	if existing == nil {
		existing = map[string]any{}
	}
	for name, entry := range servers {
		// Preserve human-added extras: only fill missing keys / update command+args for our names.
		if prev, ok := existing[name].(map[string]any); ok {
			merged := map[string]any{}
			for k, v := range prev {
				merged[k] = v
			}
			for k, v := range entry {
				merged[k] = v
			}
			// Never copy env/secrets from policy; keep existing env if present.
			existing[name] = merged
			continue
		}
		existing[name] = entry
	}
	doc["mcpServers"] = existing
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}

// rejectUserGlobalPath blocks ~/.mcp.json and other home-scoped configs.
// Repos that live under $HOME are allowed; using $HOME itself as the repo is not.
// Containment uses filepath.Rel (Windows-safe, no /work vs /workother prefix bugs).
func rejectUserGlobalPath(repoRoot, path string) error {
	abs, err := absPath(path)
	if err != nil {
		return err
	}
	repoAbs, err := absPath(repoRoot)
	if err != nil {
		return err
	}
	homeAbs := ""
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		homeAbs, _ = absPath(home)
	}
	if homeAbs != "" && samePath(repoAbs, homeAbs) {
		return fmt.Errorf("refusing to write MCP config with repo root %s", path)
	}
	if containedIn(repoAbs, abs) {
		return nil
	}
	if homeAbs != "" && containedIn(homeAbs, abs) {
		return fmt.Errorf("refusing to write user-global MCP config: %s", path)
	}
	return nil
}

func absPath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func containedIn(parent, child string) bool {
	if samePath(parent, child) {
		return true
	}
	if runtime.GOOS == "windows" {
		parent = strings.ToLower(parent)
		child = strings.ToLower(child)
	}
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	return true
}

// MergeServers additive-merges incoming into existing by name (incoming wins on conflict).
func MergeServers(existing, incoming []config.MCPServer) []config.MCPServer {
	byName := map[string]config.MCPServer{}
	order := []string{}
	add := func(s config.MCPServer) {
		name := strings.TrimSpace(s.Name)
		if name == "" {
			return
		}
		if _, ok := byName[name]; !ok {
			order = append(order, name)
		}
		byName[name] = s
	}
	for _, s := range existing {
		add(s)
	}
	for _, s := range incoming {
		add(s)
	}
	out := make([]config.MCPServer, 0, len(order))
	for _, name := range order {
		out = append(out, byName[name])
	}
	return out
}
