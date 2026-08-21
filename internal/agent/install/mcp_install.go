package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/paths"
)

const mcpServerName = "superopen"
const mcpLegacyServerName = "superopen-graph"

// RemoveUserMCP strips leftover Superopen-owned MCP entries from user-global
// configs. Superopen no longer installs MCP; this exists so `so uninstall`
// (and a later `so install`) can drop stale `superopen` / `superopen-graph`
// servers from Claude, Cursor, Codex, Gemini, OpenCode, Copilot, and Pi.
func RemoveUserMCP() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var removed []string
	for _, fn := range []func(string) (string, error){
		stripCursorMCP,
		stripClaudeUserMCP,
		stripCodexMCP,
		stripGeminiMCP,
		stripOpenCodeMCP,
		stripCopilotMCP,
		stripPiMCP,
	} {
		if path, err := fn(home); err == nil && path != "" {
			removed = append(removed, path)
		}
	}
	return removed
}

func stripCursorMCP(home string) (string, error) {
	return stripJSONMCP(filepath.Join(home, ".cursor", "mcp.json"), "mcpServers")
}

func stripClaudeUserMCP(home string) (string, error) {
	return stripJSONMCP(filepath.Join(home, ".claude.json"), "mcpServers")
}

func stripCopilotMCP(home string) (string, error) {
	base, err := paths.CopilotHome()
	if err != nil {
		return "", err
	}
	_ = home
	return stripJSONMCP(filepath.Join(base, "mcp-config.json"), "mcpServers")
}

func stripPiMCP(home string) (string, error) {
	return stripJSONMCP(filepath.Join(home, ".pi", "agent", "mcp.json"), "mcpServers")
}

func stripGeminiMCP(home string) (string, error) {
	return stripJSONMCP(filepath.Join(home, ".gemini", "settings.json"), "mcpServers")
}

func stripOpenCodeMCP(home string) (string, error) {
	base, err := paths.OpenCodeConfigDir()
	if err != nil {
		return "", err
	}
	_ = home
	path := filepath.Join(base, "opencode.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return "", err
	}
	mcp, _ := data["mcp"].(map[string]any)
	if mcp == nil {
		return "", nil
	}
	_, current := mcp[mcpServerName]
	_, legacy := mcp[mcpLegacyServerName]
	if !current && !legacy {
		return "", nil
	}
	delete(mcp, mcpServerName)
	delete(mcp, mcpLegacyServerName)
	if len(mcp) == 0 {
		delete(data, "mcp")
	} else {
		data["mcp"] = mcp
	}
	if _, err := writeJSONFile(path, data); err != nil {
		return path, err
	}
	return path, nil
}

func stripCodexMCP(home string) (string, error) {
	base, err := paths.CodexHome()
	if err != nil {
		return "", err
	}
	_ = home
	path := filepath.Join(base, "config.toml")
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	next, changed := stripCodexMCPSection(string(raw))
	if !changed {
		return "", nil
	}
	if err := os.WriteFile(path, []byte(next), 0o600); err != nil {
		return path, err
	}
	return path, nil
}

func stripCodexMCPSection(src string) (string, bool) {
	changed := false
	next := src
	for _, name := range []string{mcpServerName, mcpLegacyServerName} {
		stripped, did := stripCodexMCPHeader(next, name)
		next = stripped
		changed = changed || did
	}
	return next, changed
}

func stripCodexMCPHeader(src, name string) (string, bool) {
	header := "[mcp_servers." + name + "]"
	sep := "\n"
	if strings.Contains(src, "\r\n") {
		sep = "\r\n"
	}
	lines := strings.Split(src, sep)
	out := make([]string, 0, len(lines))
	drop := false
	changed := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if trimmed == header {
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
	if !strings.HasSuffix(joined, sep) && joined != "" {
		joined += sep
	}
	return joined, changed
}

func stripJSONMCP(path, serversKey string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return "", err
	}
	servers, _ := data[serversKey].(map[string]any)
	if servers == nil {
		return "", nil
	}
	_, current := servers[mcpServerName]
	_, legacy := servers[mcpLegacyServerName]
	if !current && !legacy {
		return "", nil
	}
	delete(servers, mcpServerName)
	delete(servers, mcpLegacyServerName)
	if len(servers) == 0 {
		delete(data, serversKey)
	} else {
		data[serversKey] = servers
	}
	if _, err := writeJSONFile(path, data); err != nil {
		return path, err
	}
	return path, nil
}

func writeJSONFile(path string, data map[string]any) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	body, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return "", err
	}
	return path, nil
}