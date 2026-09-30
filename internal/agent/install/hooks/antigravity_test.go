package hooks

import (
	"github.com/ishanjainn/superopen/internal/agent/install/hooks/testenv"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallAntigravityHooksPreservesNonSuperopenBlocks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	existing := `{"my-linter-hook":{"PostToolUse":[{"matcher":"run_command","hooks":[{"type":"command","command":"echo keep"}]}]}}`
	if err := os.WriteFile(path, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	if err := installAntigravityHooks(path, "/tmp/superopen hooks", "/tmp/runtime.jsonl", "/tmp/config.json"); err != nil {
		t.Fatalf("installAntigravityHooks returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read hooks: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		"my-linter-hook",
		"echo keep",
		"superopen-endpoint",
		"'/tmp/superopen hooks' sessions hook --vendor=antigravity",
		"PreInvocation",
		"UserPromptSubmit",
		"PreToolUse",
		"PostToolUse",
		"PostInvocation",
		"Stop",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Antigravity hooks missing %q:\n%s", want, text)
		}
	}
}

func TestInstallAntigravityHooksReplacesManagedBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	existing := `{"superopen-endpoint":{"Stop":[{"hooks":[{"type":"command","command":"sessions hook old-so sessions hook --vendor=antigravity stop"}]}]},"other":{"Stop":[{"hooks":[{"command":"echo keep"}]}]}}`
	if err := os.WriteFile(path, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	if err := installAntigravityHooks(path, "/tmp/new-so", "/tmp/runtime.jsonl", "/tmp/config.json"); err != nil {
		t.Fatalf("installAntigravityHooks returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read hooks: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "old-so") {
		t.Fatalf("old endpoint hook was not replaced:\n%s", text)
	}
	if !strings.Contains(text, "echo keep") || !strings.Contains(text, "/tmp/new-so") {
		t.Fatalf("expected preserved block and new hook:\n%s", text)
	}
}

func TestRemoveAntigravityEndpointHooksPreservesOtherBlocks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	existing := `{"superopen-endpoint":{"Stop":[{"hooks":[{"type":"command","command":"sessions hook so sessions hook --vendor=antigravity stop"}]}]},"other":{"Stop":[{"hooks":[{"command":"echo keep"}]}]}}`
	if err := os.WriteFile(path, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	changed, err := removeAntigravityEndpointHooks(path)
	if err != nil {
		t.Fatalf("removeAntigravityEndpointHooks returned error: %v", err)
	}
	if !changed {
		t.Fatal("expected endpoint hook removal")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read hooks: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "echo keep") {
		t.Fatalf("non-Superopen block was not preserved:\n%s", text)
	}
	if strings.Contains(text, "superopen-endpoint") || strings.Contains(text, "sessions hook") {
		t.Fatalf("endpoint hook was not removed:\n%s", text)
	}
}

func TestReadAntigravityConfigReturnsCorruptJSONError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
		t.Fatalf("write corrupt config: %v", err)
	}

	if _, err := readAntigravityConfig(path); err == nil {
		t.Fatal("expected corrupt config error")
	}
}

func TestAntigravityConfigPathLevels(t *testing.T) {
	home := t.TempDir()
	testenv.SetHome(t, home)
	dir := t.TempDir()
	t.Chdir(dir)

	userPath, err := antigravityConfigPath(LevelUser)
	if err != nil {
		t.Fatalf("user antigravityConfigPath returned error: %v", err)
	}
	if got, want := userPath, filepath.Join(home, ".gemini", "config", "hooks.json"); got != want {
		t.Fatalf("user config path = %q, want %q", got, want)
	}
	projectPath, err := antigravityConfigPath(LevelProject)
	if err != nil {
		t.Fatalf("project antigravityConfigPath returned error: %v", err)
	}
	if got, want := projectPath, filepath.Join(dir, ".agents", "hooks.json"); got != want {
		t.Fatalf("project config path = %q, want %q", got, want)
	}
}

func TestAntigravityHookStatusDetectsInstalled(t *testing.T) {
	home := t.TempDir()
	testenv.SetHome(t, home)
	path := filepath.Join(home, ".gemini", "config", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"superopen-endpoint":{"Stop":[{"hooks":[{"type":"command","command":"sessions hook so sessions hook --vendor=antigravity stop"}]}]}}`), 0600); err != nil {
		t.Fatal(err)
	}

	status := AntigravityHookStatus(AntigravityOptions{Level: LevelUser, UserMode: true})
	if !status.Installed {
		t.Fatalf("AntigravityHookStatus installed = false, status=%#v", status)
	}
	if status.ConfigPath != path {
		t.Fatalf("ConfigPath = %q, want %q", status.ConfigPath, path)
	}
}
