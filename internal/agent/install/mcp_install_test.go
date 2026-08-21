package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestStripJSONMCPMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	got, err := stripJSONMCP(path, "mcpServers")
	if err != nil || got != "" {
		t.Fatalf("missing file should be a no-op, got %q err=%v", got, err)
	}
}

func TestStripCodexMCPSection(t *testing.T) {
	prev := "[features]\nfoo = true\n\n[mcp_servers.superopen]\ncommand = \"/tmp/so\"\nargs = [\"graph\", \"mcp\", \"serve\"]\n\n[mcp_servers.other]\ncommand = \"x\"\n"
	next, changed := stripCodexMCPSection(prev)
	if !changed {
		t.Fatal("expected strip to change the file")
	}
	if strings.Contains(next, "mcp_servers.superopen") {
		t.Fatalf("superopen section survived: %s", next)
	}
	if !strings.Contains(next, "[mcp_servers.other]") {
		t.Fatalf("lost other section: %s", next)
	}
	if !strings.Contains(next, "[features]") {
		t.Fatalf("lost features: %s", next)
	}
}
