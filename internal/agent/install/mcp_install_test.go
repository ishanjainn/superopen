package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeJSONMCPIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	soBin := filepath.Join(dir, "so")
	if _, err := mergeJSONMCP(path, soBin, "mcpServers"); err != nil {
		t.Fatal(err)
	}
	if _, err := mergeJSONMCP(path, soBin, "mcpServers"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	servers := data["mcpServers"].(map[string]any)
	entry := servers[mcpServerName].(map[string]any)
	if entry["command"] != soBin {
		t.Fatalf("command = %v", entry["command"])
	}
	args := entry["args"].([]any)
	if len(args) != 3 || args[0] != "graph" || args[1] != "mcp" || args[2] != "serve" {
		t.Fatalf("args = %#v", args)
	}
	joined := string(raw)
	if strings.Count(joined, mcpServerName) != 1 {
		t.Fatalf("expected one server entry, got %s", joined)
	}
}

func TestStripJSONMCPPreservesOthers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	body := []byte(`{"mcpServers":{"other":{"command":"x"},"superopen":{"command":"so","args":["graph","mcp","serve"]},"superopen-graph":{"command":"old"}}}`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := stripJSONMCP(path, "mcpServers"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	var data map[string]any
	_ = json.Unmarshal(raw, &data)
	servers := data["mcpServers"].(map[string]any)
	if _, ok := servers["superopen"]; ok {
		t.Fatal("superopen should be removed")
	}
	if _, ok := servers["superopen-graph"]; ok {
		t.Fatal("legacy superopen-graph should be removed")
	}
	if _, ok := servers["other"]; !ok {
		t.Fatal("other server should remain")
	}
}

func TestMergeJSONMCPRenamesLegacy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	soBin := filepath.Join(dir, "so")
	body := []byte(`{"mcpServers":{"superopen-graph":{"command":"old-so","args":["graph","mcp","serve"]}}}`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := mergeJSONMCP(path, soBin, "mcpServers"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	servers := data["mcpServers"].(map[string]any)
	if _, ok := servers["superopen-graph"]; ok {
		t.Fatal("legacy superopen-graph should be renamed away")
	}
	entry := servers["superopen"].(map[string]any)
	if entry["command"] != soBin {
		t.Fatalf("command = %v", entry["command"])
	}
}

func TestUpsertCodexMCPSection(t *testing.T) {
	prev := "[features]\nfoo = true\n\n[mcp_servers.other]\ncommand = \"x\"\n"
	next := upsertCodexMCPSection(prev, `/tmp/so bin`)
	if !strings.Contains(next, `[mcp_servers.superopen]`) {
		t.Fatalf("missing section: %s", next)
	}
	if strings.Contains(next, `superopen-graph`) {
		t.Fatalf("legacy name survived: %s", next)
	}
	if !strings.Contains(next, `[mcp_servers.other]`) {
		t.Fatalf("lost other section: %s", next)
	}
	again := upsertCodexMCPSection(next, `/tmp/so bin`)
	if strings.Count(again, `[mcp_servers.superopen]`) != 1 {
		t.Fatalf("not idempotent: %s", again)
	}
}

func TestMergeJSONMCPRefusesMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := mergeJSONMCP(path, "so", "mcpServers"); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}
