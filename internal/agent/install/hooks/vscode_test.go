package hooks

import (
	"github.com/ishanjainn/superopen/internal/agent/install/hooks/testenv"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallVSCodeHooksPreservesNonSuperopenHooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "superopen.json")
	existing := `{"hooks":{"SessionStart":[{"type":"command","command":"echo keep"}]}}`
	if err := os.WriteFile(path, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	if err := installVSCodeHooks(path, "/tmp/superopen hooks", "/tmp/runtime.jsonl", "/tmp/config.json"); err != nil {
		t.Fatalf("installVSCodeHooks returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"echo keep",
		"'/tmp/superopen hooks' sessions hook --vendor=vscode",
		"SessionStart",
		"UserPromptSubmit",
		"PreToolUse",
		"PostToolUse",
		"SubagentStart",
		"SubagentStop",
		"Stop",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("hooks missing %q:\n%s", want, text)
		}
	}
}

func TestRemoveVSCodeEndpointHooksPreservesOtherHooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "superopen.json")
	existing := `{"hooks":{"PreToolUse":[{"type":"command","command":"echo keep"},{"type":"command","command":"sessions hook so sessions hook --vendor=vscode pre-tool"}]}}`
	if err := os.WriteFile(path, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	changed, err := removeVSCodeEndpointHooks(path)
	if err != nil {
		t.Fatalf("removeVSCodeEndpointHooks returned error: %v", err)
	}
	if !changed {
		t.Fatal("expected endpoint hook removal")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "echo keep") {
		t.Fatalf("non-Superopen hook was not preserved:\n%s", text)
	}
	if strings.Contains(text, "sessions hook") {
		t.Fatalf("endpoint hook was not removed:\n%s", text)
	}
}

func TestVSCodeHooksPathLevels(t *testing.T) {
	home := t.TempDir()
	testenv.SetHome(t, home)
	userPath, err := vscodeHooksPath(LevelUser)
	if err != nil {
		t.Fatalf("user path error: %v", err)
	}
	if want := filepath.Join(home, ".copilot", "hooks", "superopen.json"); userPath != want {
		t.Fatalf("user path = %q, want %q", userPath, want)
	}

	project := t.TempDir()
	t.Chdir(project)
	projectPath, err := vscodeHooksPath(LevelProject)
	if err != nil {
		t.Fatalf("project path error: %v", err)
	}
	if want := filepath.Join(project, ".github", "hooks", "superopen.json"); projectPath != want {
		t.Fatalf("project path = %q, want %q", projectPath, want)
	}
}
