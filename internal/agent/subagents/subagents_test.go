package subagents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallWritesAgentsForPresentVendorsOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	// Only Claude Code is present; Cursor is not.
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}

	written, err := InstallAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != len(Names) {
		t.Fatalf("expected %d files, got %d: %v", len(Names), len(written), written)
	}
	for _, name := range Names {
		if _, err := os.Stat(filepath.Join(home, ".claude", "agents", name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".cursor", "agents")); !os.IsNotExist(err) {
		t.Fatal("must not create agent dirs for absent vendors")
	}
}

func TestRemoveLeavesForeignAgentsAlone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	agentDir := filepath.Join(home, ".claude", "agents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(agentDir, "someone-elses-agent.md")
	if err := os.WriteFile(foreign, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallAll(); err != nil {
		t.Fatal(err)
	}

	RemoveAll()

	if _, err := os.Stat(foreign); err != nil {
		t.Fatalf("uninstall removed a foreign agent: %v", err)
	}
	for _, name := range Names {
		if _, err := os.Stat(filepath.Join(agentDir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s survived uninstall", name)
		}
	}
}

func TestEachAgentDeclaresToolsAndABudget(t *testing.T) {
	for _, name := range Names {
		body, err := agentFS.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		if !strings.HasPrefix(text, "---\n") {
			t.Fatalf("%s: missing frontmatter", name)
		}
		if !strings.Contains(text, "mcp__superopen-graph__graph_query") {
			t.Fatalf("%s: does not allowlist the graph tools", name)
		}
		if !strings.Contains(text, "permissionMode: plan") {
			t.Fatalf("%s: graph subagents must be read-only", name)
		}
		// The stop discipline is the reason these exist; without it the
		// child burns the same turns the parent would have.
		if !strings.Contains(text, "Stop") && !strings.Contains(text, "stop") {
			t.Fatalf("%s: no stop rule", name)
		}
	}
}
