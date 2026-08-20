package subagents

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// isolateUserDirs redirects every vendor-home env var InstallAll reads so a
// machine (or Windows CI runner) with Claude already under LOCALAPPDATA cannot
// leak extra agent files into the assertion.
func isolateUserDirs(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	t.Setenv("COPILOT_HOME", "")
	return home
}

func TestInstallWritesAgentsForPresentVendorsOnly(t *testing.T) {
	home := isolateUserDirs(t)
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
	if _, err := os.Stat(filepath.Join(os.Getenv("LOCALAPPDATA"), "claude", "agents")); !os.IsNotExist(err) {
		t.Fatal("must not create agent dirs for absent LOCALAPPDATA/claude")
	}
}

func TestInstallWritesWindowsLocalAppDataClaudeWhenPresent(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("LOCALAPPDATA/claude is only a vendor home on Windows")
	}
	home := isolateUserDirs(t)
	localClaude := filepath.Join(os.Getenv("LOCALAPPDATA"), "claude")
	if err := os.MkdirAll(localClaude, 0o755); err != nil {
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
		if _, err := os.Stat(filepath.Join(localClaude, "agents", name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "agents")); !os.IsNotExist(err) {
		t.Fatal("must not create agent dirs for absent ~/.claude")
	}
}

func TestRemoveLeavesForeignAgentsAlone(t *testing.T) {
	home := isolateUserDirs(t)
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
		// Git for Windows may check out CRLF; go:embed then bakes those
		// bytes in, so compare against normalized newlines.
		text := strings.ReplaceAll(string(body), "\r\n", "\n")
		if !strings.HasPrefix(text, "---\n") {
			t.Fatalf("%s: missing frontmatter", name)
		}
		if !strings.Contains(text, "mcp__superopen__graph_query") {
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
