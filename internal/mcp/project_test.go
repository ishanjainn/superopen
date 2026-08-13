package mcp_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/mcp"
)

func TestProjectMergesWithoutClobber(t *testing.T) {
	dir := t.TempDir()
	existing := map[string]any{
		"mcpServers": map[string]any{
			"custom":   map[string]any{"command": "node", "args": []any{"server.js"}},
			"context7": map[string]any{"command": "npx", "args": []any{"-y", "old"}, "env": map[string]any{"TOKEN": "keep"}},
		},
	}
	raw, _ := json.MarshalIndent(existing, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, ".mcp.json"), raw, 0o644)

	cfg := config.Default()
	cfg.Vendors.Enabled = []string{"claude", "cursor"}
	cfg.MCP.Servers = []config.MCPServer{
		{Name: "context7", Command: "npx", Args: []string{"-y", "@upstash/context7-mcp@1.0.0"}},
		{Name: "playwright", Command: "npx", Args: []string{"-y", "@playwright/mcp@0.0.10"}},
	}
	if err := mcp.Project(dir, cfg); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	servers := doc["mcpServers"].(map[string]any)
	if _, ok := servers["custom"]; !ok {
		t.Fatal("expected custom server preserved")
	}
	ctx := servers["context7"].(map[string]any)
	if env, ok := ctx["env"].(map[string]any); !ok || env["TOKEN"] != "keep" {
		t.Fatalf("expected existing env preserved, got %#v", ctx)
	}
	args := ctx["args"].([]any)
	if len(args) == 0 || !strings.Contains(args[len(args)-1].(string), "context7-mcp@1.0.0") {
		t.Fatalf("expected updated args, got %#v", args)
	}
	if _, ok := servers["playwright"]; !ok {
		t.Fatal("expected playwright added")
	}

	cursorPath := filepath.Join(dir, ".cursor", "mcp.json")
	if _, err := os.Stat(cursorPath); err != nil {
		t.Fatalf("expected cursor mcp.json: %v", err)
	}
}

func TestProjectAllowsRepoUnderHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home")
	}
	dir := filepath.Join(home, ".superopen-mcp-test-"+strings.ReplaceAll(t.Name(), "/", "-"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	cfg := config.Default()
	cfg.Vendors.Enabled = []string{"claude", "cursor"}
	cfg.MCP.Servers = []config.MCPServer{{Name: "context7", Command: "npx", Args: []string{"-y", "@upstash/context7-mcp@1.0.0"}}}
	if err := mcp.Project(dir, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".mcp.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".cursor", "mcp.json")); err != nil {
		t.Fatal(err)
	}
}

func TestProjectRefusesHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home")
	}
	cfg := config.Default()
	cfg.Vendors.Enabled = []string{"claude"}
	cfg.MCP.Servers = []config.MCPServer{{Name: "x", Command: "echo"}}
	if err := mcp.Project(home, cfg); err == nil {
		t.Fatal("expected refuse when repoRoot is $HOME")
	}
}

func TestMergeServersAdditive(t *testing.T) {
	a := []config.MCPServer{{Name: "a", Command: "echo"}}
	b := []config.MCPServer{{Name: "b", Command: "echo"}, {Name: "a", Command: "npx"}}
	got := mcp.MergeServers(a, b)
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].Name != "a" || got[0].Command != "npx" {
		t.Fatalf("expected a updated: %#v", got[0])
	}
}

func TestProjectSkipsLatest(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Vendors.Enabled = []string{"claude"}
	cfg.MCP.Servers = []config.MCPServer{
		{Name: "bad", Command: "npx", Args: []string{"-y", "pkg@latest"}},
		{Name: "memory", Command: "npx", Args: []string{"-y", "memory"}},
	}
	if err := mcp.Project(dir, cfg); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	var doc map[string]any
	_ = json.Unmarshal(data, &doc)
	servers, _ := doc["mcpServers"].(map[string]any)
	if _, ok := servers["memory"]; ok {
		t.Fatal("memory mcp must be skipped")
	}
	if _, ok := servers["bad"]; ok {
		t.Fatal("expected @latest server skipped entirely")
	}
}
