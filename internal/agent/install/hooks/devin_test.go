package hooks

import (
	"encoding/json"
	"github.com/ishanjainn/superopen/internal/agent/install/hooks/testenv"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallDevinProjectHooksPreservesNonSuperopenHooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.v1.json")
	existing := `{"PreToolUse":[{"matcher":"exec","hooks":[{"type":"command","command":"echo keep"}]}]}`
	if err := os.WriteFile(path, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	if err := installDevinHooks(path, "/tmp/superopen hooks", "/tmp/runtime.jsonl", "/tmp/config.json"); err != nil {
		t.Fatalf("installDevinHooks returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read hooks: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		"echo keep",
		"'/tmp/superopen hooks' sessions hook --vendor=devin",
		"PermissionRequest",
		"PreToolUse",
		"PostToolUse",
		"SessionStart",
		"SessionEnd",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Devin hooks missing %q:\n%s", want, text)
		}
	}
}

func TestInstallDevinUserConfigPreservesUnrelatedKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	existing := `{"theme":"dark","hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo keep"}]}]}}`
	if err := os.WriteFile(path, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	if err := installDevinHooks(path, "/tmp/so", "/tmp/runtime.jsonl", "/tmp/config.json"); err != nil {
		t.Fatalf("installDevinHooks returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `"theme"`) || !strings.Contains(text, "echo keep") || !strings.Contains(text, "--vendor=devin") {
		t.Fatalf("user config was not preserved and updated:\n%s", text)
	}
}

func TestInstallDevinUserConfigPreservesImportConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	existing := `{"theme":"dark","read_config_from":{"cursor":true,"windsurf":false,"claude":true}}`
	if err := os.WriteFile(path, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	if err := installDevinCLIHooks(path, "/tmp/so", "/tmp/runtime.jsonl", "/tmp/config.json"); err != nil {
		t.Fatalf("installDevinCLIHooks returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("unmarshal config: %v\n%s", err, data)
	}
	readConfigFrom := config["read_config_from"].(map[string]interface{})
	if readConfigFrom["claude"] != true {
		t.Fatalf("read_config_from.claude = %#v, want preserved true", readConfigFrom["claude"])
	}
	if readConfigFrom["cursor"] != true || readConfigFrom["windsurf"] != false {
		t.Fatalf("read_config_from did not preserve other values: %#v", readConfigFrom)
	}
	if config["theme"] != "dark" {
		t.Fatalf("theme = %#v, want preserved dark", config["theme"])
	}
}

func TestInstallDevinDesktopHooksUsesCascadeEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	if err := installDevinDesktopHooks(path, "/tmp/so", "/tmp/runtime.jsonl", "/tmp/config.json"); err != nil {
		t.Fatalf("installDevinDesktopHooks returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read hooks: %v", err)
	}
	text := string(data)
	for _, want := range []string{"pre_user_prompt", "post_write_code", "post_run_command", "post_mcp_tool_use", "post_read_code", "--vendor=devin-desktop", "--event=UserPromptSubmit", "--event=PostToolUse"} {
		if !strings.Contains(text, want) {
			t.Fatalf("Devin Desktop hooks missing %q:\n%s", want, text)
		}
	}
	for _, notWant := range []string{`"PreToolUse"`, `"PostToolUse"`, `"SessionStart"`, "read_config_from"} {
		if strings.Contains(text, notWant) {
			t.Fatalf("Devin Desktop hooks should not contain %q:\n%s", notWant, text)
		}
	}
}

func TestInstallDevinHooksReplacesOldSuperopenHooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.v1.json")
	existing := `{"PostToolUse":[{"hooks":[{"type":"command","command":"sessions hook old-so sessions hook --vendor=devin post-tool"}]},{"hooks":[{"type":"command","command":"echo keep"}]}]}`
	if err := os.WriteFile(path, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	if err := installDevinHooks(path, "/tmp/new-so", "/tmp/runtime.jsonl", "/tmp/config.json"); err != nil {
		t.Fatalf("installDevinHooks returned error: %v", err)
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
		t.Fatalf("expected preserved hook and new hook:\n%s", text)
	}
}

func TestInstallDevinCLIHooksReplacesLegacyHooksWithExplicitPlatform(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.v1.json")
	existing := `{"PostToolUse":[{"hooks":[{"type":"command","command":"sessions hook old-so sessions hook --vendor=devin post-tool"}]},{"hooks":[{"type":"command","command":"echo keep"}]}]}`
	if err := os.WriteFile(path, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	if err := installDevinCLIHooks(path, "/tmp/new-so", "/tmp/runtime.jsonl", "/tmp/config.json"); err != nil {
		t.Fatalf("installDevinCLIHooks returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read hooks: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "--vendor=devin post-tool") || strings.Contains(text, "old-so") {
		t.Fatalf("legacy devin hook was not replaced:\n%s", text)
	}
	if !strings.Contains(text, "echo keep") || !strings.Contains(text, "--vendor=devin-cli") {
		t.Fatalf("expected preserved hook and explicit devin-cli hook:\n%s", text)
	}
}

func TestInstallDevinDesktopHooksDoesNotRemoveCLIHooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	existing := `{"hooks":{"post_write_code":[{"command":"sessions hook so sessions hook --vendor=devin-cli post-tool"}]}}`
	if err := os.WriteFile(path, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	if err := installDevinDesktopHooks(path, "/tmp/so", "/tmp/runtime.jsonl", "/tmp/config.json"); err != nil {
		t.Fatalf("installDevinDesktopHooks returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read hooks: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "--vendor=devin-cli") || !strings.Contains(text, "--vendor=devin-desktop") {
		t.Fatalf("expected CLI and Desktop hooks to coexist:\n%s", text)
	}
}

func TestRemoveDevinEndpointHooksPreservesOtherHooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.v1.json")
	existing := `{"SessionStart":[{"hooks":[{"type":"command","command":"echo keep"}]},{"hooks":[{"type":"command","command":"sessions hook so sessions hook --vendor=devin session-start"}]}]}`
	if err := os.WriteFile(path, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	changed, err := removeDevinEndpointHooks(path)
	if err != nil {
		t.Fatalf("removeDevinEndpointHooks returned error: %v", err)
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
		t.Fatalf("non-Superopen hook was not preserved:\n%s", text)
	}
	if strings.Contains(text, "sessions hook") {
		t.Fatalf("endpoint hook was not removed:\n%s", text)
	}
}

func TestRemoveDevinDesktopEndpointHooksPreservesCLIHooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	existing := `{"hooks":{"post_write_code":[{"command":"sessions hook so sessions hook --vendor=devin-cli post-tool"},{"command":"sessions hook so sessions hook --vendor=devin-desktop post-tool"}]}}`
	if err := os.WriteFile(path, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	changed, err := removeDevinDesktopEndpointHooks(path)
	if err != nil {
		t.Fatalf("removeDevinDesktopEndpointHooks returned error: %v", err)
	}
	if !changed {
		t.Fatal("expected desktop endpoint hook removal")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read hooks: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "--vendor=devin-cli") {
		t.Fatalf("Devin CLI hook was not preserved:\n%s", text)
	}
	if strings.Contains(text, "--vendor=devin-desktop") {
		t.Fatalf("Devin Desktop hook was not removed:\n%s", text)
	}
}

func TestReadDevinConfigReturnsCorruptJSONError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
		t.Fatalf("write corrupt config: %v", err)
	}

	if _, err := readDevinConfig(path); err == nil {
		t.Fatal("expected corrupt config error")
	}
}

func TestDevinConfigPathProjectLevel(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	target, err := devinConfigPath(LevelProject)
	if err != nil {
		t.Fatalf("devinConfigPath returned error: %v", err)
	}
	if got, want := target, filepath.Join(dir, ".devin", "hooks.v1.json"); got != want {
		t.Fatalf("project config path = %q, want %q", got, want)
	}
}

func TestDevinHookStatusDetectsInstalled(t *testing.T) {
	home := t.TempDir()
	testenv.SetHome(t, home)
	path := filepath.Join(home, ".config", "devin", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"sessions hook so sessions hook --vendor=devin stop"}]}]}}`), 0600); err != nil {
		t.Fatal(err)
	}

	status := DevinHookStatus(DevinOptions{Level: LevelUser, UserMode: true})
	if !status.Installed {
		t.Fatalf("DevinHookStatus installed = false, status=%#v", status)
	}
	if status.ConfigPath != path {
		t.Fatalf("ConfigPath = %q, want %q", status.ConfigPath, path)
	}
}

func TestDevinDesktopHookStatusDetectsInstalled(t *testing.T) {
	home := t.TempDir()
	testenv.SetHome(t, home)
	path := filepath.Join(home, ".codeium", "windsurf", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"hooks":{"post_write_code":[{"command":"sessions hook so sessions hook --vendor=devin-desktop post-tool"}]}}`), 0600); err != nil {
		t.Fatal(err)
	}

	status := DevinDesktopHookStatus(DevinDesktopOptions{Level: LevelUser, UserMode: true})
	if !status.Installed {
		t.Fatalf("DevinDesktopHookStatus installed = false, status=%#v", status)
	}
	if status.ConfigPath != path {
		t.Fatalf("ConfigPath = %q, want %q", status.ConfigPath, path)
	}
}

func TestDevinDesktopConfigPathProjectLevel(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	target, err := devinDesktopConfigPath(LevelProject)
	if err != nil {
		t.Fatalf("devinDesktopConfigPath returned error: %v", err)
	}
	if got, want := target, filepath.Join(dir, ".windsurf", "hooks.json"); got != want {
		t.Fatalf("project config path = %q, want %q", got, want)
	}
}
