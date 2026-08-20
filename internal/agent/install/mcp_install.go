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

const mcpServerName = "superopen"
const mcpLegacyServerName = "superopen-graph"

// InstallUserMCP merges repo-neutral Superopen MCP entries into each agent's
// user-global config. Entries launch `so graph mcp serve` without a baked
// --root; the server resolves the active repo from cwd / SUPEROPEN_ROOT / FindRoot.
func InstallUserMCP() ([]string, error) {
	soBin, err := resolveSoBin()
	if err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	var written []string
	writers := []func(string, string) (string, error){
		writeCursorMCP,
		writeClaudeUserMCP,
		writeCodexMCP,
		writeGeminiMCP,
		writeOpenCodeMCP,
		writeCopilotMCP,
		writePiMCP,
	}
	for _, write := range writers {
		path, err := write(home, soBin)
		if err != nil {
			return written, err
		}
		if path != "" {
			written = append(written, path)
		}
	}
	return written, nil
}

// RemoveUserMCP strips Superopen-owned MCP entries from user-global configs.
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

func mcpEntry(soBin string) map[string]any {
	return map[string]any{
		"command": soBin,
		"args":    []string{"graph", "mcp", "serve"},
	}
}

func writeCursorMCP(home, soBin string) (string, error) {
	path := filepath.Join(home, ".cursor", "mcp.json")
	return mergeJSONMCP(path, soBin, "mcpServers")
}

func stripCursorMCP(home string) (string, error) {
	return stripJSONMCP(filepath.Join(home, ".cursor", "mcp.json"), "mcpServers")
}

func writeClaudeUserMCP(home, soBin string) (string, error) {
	// User-scope MCP lives in ~/.claude.json under mcpServers.
	path := filepath.Join(home, ".claude.json")
	if runtime.GOOS == "windows" {
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			// Prefer classic home file; Claude on Windows still uses ~/.claude.json via UserHomeDir.
			_ = local
		}
	}
	return mergeJSONMCP(path, soBin, "mcpServers")
}

func stripClaudeUserMCP(home string) (string, error) {
	return stripJSONMCP(filepath.Join(home, ".claude.json"), "mcpServers")
}

func writeCopilotMCP(home, soBin string) (string, error) {
	base, err := paths.CopilotHome()
	if err != nil {
		return "", err
	}
	_ = home
	return mergeJSONMCP(filepath.Join(base, "mcp-config.json"), soBin, "mcpServers")
}

func stripCopilotMCP(home string) (string, error) {
	base, err := paths.CopilotHome()
	if err != nil {
		return "", err
	}
	_ = home
	return stripJSONMCP(filepath.Join(base, "mcp-config.json"), "mcpServers")
}

func writePiMCP(home, soBin string) (string, error) {
	// Pi has no native MCP; write a shared config some Pi MCP extensions read.
	path := filepath.Join(home, ".pi", "agent", "mcp.json")
	return mergeJSONMCP(path, soBin, "mcpServers")
}

func stripPiMCP(home string) (string, error) {
	return stripJSONMCP(filepath.Join(home, ".pi", "agent", "mcp.json"), "mcpServers")
}

func writeGeminiMCP(home, soBin string) (string, error) {
	path := filepath.Join(home, ".gemini", "settings.json")
	return mergeJSONMCP(path, soBin, "mcpServers")
}

func stripGeminiMCP(home string) (string, error) {
	return stripJSONMCP(filepath.Join(home, ".gemini", "settings.json"), "mcpServers")
}

func writeOpenCodeMCP(home, soBin string) (string, error) {
	base, err := paths.OpenCodeConfigDir()
	if err != nil {
		return "", err
	}
	_ = home
	path := filepath.Join(base, "opencode.json")
	raw, err := os.ReadFile(path)
	data := map[string]any{}
	if err == nil {
		if uerr := json.Unmarshal(raw, &data); uerr != nil {
			return "", fmt.Errorf("parse %s: %w", path, uerr)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	mcp, _ := data["mcp"].(map[string]any)
	if mcp == nil {
		mcp = map[string]any{}
	}
	delete(mcp, mcpLegacyServerName)
	mcp[mcpServerName] = map[string]any{
		"type":    "local",
		"command": []any{soBin, "graph", "mcp", "serve"},
		"enabled": true,
	}
	data["mcp"] = mcp
	return writeJSONFile(path, data)
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

func writeCodexMCP(home, soBin string) (string, error) {
	base, err := paths.CodexHome()
	if err != nil {
		return "", err
	}
	_ = home
	path := filepath.Join(base, "config.toml")
	raw, err := os.ReadFile(path)
	prev := ""
	if err == nil {
		prev = string(raw)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	next := upsertCodexMCPSection(prev, soBin)
	if next == prev {
		return path, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(next), 0o600); err != nil {
		return "", err
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

func upsertCodexMCPSection(src, soBin string) string {
	section := fmt.Sprintf("[mcp_servers.%s]\ncommand = %q\nargs = [\"graph\", \"mcp\", \"serve\"]\n", mcpServerName, soBin)
	stripped, _ := stripCodexMCPSection(src)
	stripped = strings.TrimRight(stripped, " \t\r\n")
	if stripped == "" {
		return section
	}
	return stripped + "\n\n" + section
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

func mergeJSONMCP(path, soBin, serversKey string) (string, error) {
	raw, err := os.ReadFile(path)
	data := map[string]any{}
	if err == nil {
		if uerr := json.Unmarshal(raw, &data); uerr != nil {
			return "", fmt.Errorf("parse %s: %w (refusing to overwrite)", path, uerr)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	servers, _ := data[serversKey].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	delete(servers, mcpLegacyServerName)
	servers[mcpServerName] = mcpEntry(soBin)
	data[serversKey] = servers
	return writeJSONFile(path, data)
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
