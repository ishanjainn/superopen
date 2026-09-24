package hooks

import (
	"encoding/json"
	"github.com/ishanjainn/superopen/internal/agent/install/hooks/testenv"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallFactorySettingsPreservesNonSuperopenHooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	existing := `{"enabledPlugins":{"core@factory-plugins":true},"logoAnimation":"off","hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo keep"}]}]}}`
	if err := os.WriteFile(path, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	if err := installFactorySettings(path, "/tmp/superopen hooks", "/tmp/runtime.jsonl", "/tmp/config.json"); err != nil {
		t.Fatalf("installFactorySettings returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		"enabledPlugins",
		"logoAnimation",
		"echo keep",
		"'/tmp/superopen hooks' sessions hook --vendor=factory",
		"SessionStart",
		"UserPromptSubmit",
		"PostToolUse",
		"Write|Edit|MultiEdit|Create",
		"Stop",
		"SessionEnd",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("settings missing %q:\n%s", want, text)
		}
	}
}

func TestInstallFactorySettingsReplacesOldSuperopenAndSuperopenHooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	existing := `{"hooks":{"PostToolUse":[{"hooks":[{"type":"command","command":"sessions hook old-so sessions hook --vendor=factory post-tool"}]},{"hooks":[{"type":"command","command":"/tmp/so sessions hook --vendor=factory post-tool"}]},{"hooks":[{"type":"command","command":"echo keep"}]}]}}`
	if err := os.WriteFile(path, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	if err := installFactorySettings(path, "/tmp/new-so", "/tmp/runtime.jsonl", "/tmp/config.json"); err != nil {
		t.Fatalf("installFactorySettings returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "old-so") || strings.Contains(text, "/tmp/so ") {
		t.Fatalf("old endpoint hook was not replaced:\n%s", text)
	}
	if !strings.Contains(text, "echo keep") || !strings.Contains(text, "/tmp/new-so") {
		t.Fatalf("expected preserved hook and new hook:\n%s", text)
	}
}

func TestInstallFactorySettingsPreservesUserHookInMixedGroup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	existing := `{"hooks":{"PostToolUse":[{"matcher":"Write|Edit","hooks":[{"type":"command","command":"echo keep"},{"type":"command","command":"sessions hook old-so sessions hook --vendor=factory post-tool"}]}]}}`
	if err := os.WriteFile(path, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	if err := installFactorySettings(path, "/tmp/new-so", "/tmp/runtime.jsonl", "/tmp/config.json"); err != nil {
		t.Fatalf("installFactorySettings returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "old-so") {
		t.Fatalf("old endpoint hook was not replaced:\n%s", text)
	}
	if !strings.Contains(text, "echo keep") {
		t.Fatalf("user hook in mixed group was not preserved:\n%s", text)
	}
	if !strings.Contains(text, "/tmp/new-so") {
		t.Fatalf("new endpoint hook was not installed:\n%s", text)
	}
}

func TestRemoveFactoryEndpointHooksPreservesOtherHooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	existing := `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo keep"}]},{"hooks":[{"type":"command","command":"sessions hook so sessions hook --vendor=factory session-start"}]}]}}`
	if err := os.WriteFile(path, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	changed, err := removeFactoryEndpointHooks(path)
	if err != nil {
		t.Fatalf("removeFactoryEndpointHooks returned error: %v", err)
	}
	if !changed {
		t.Fatal("expected endpoint hook removal")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "echo keep") {
		t.Fatalf("non-Superopen hook was not preserved:\n%s", text)
	}
	if strings.Contains(text, "sessions hook") {
		t.Fatalf("endpoint hook was not removed:\n%s", text)
	}
}

func TestRemoveFactoryEndpointHooksPreservesUserHookInMixedGroup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	existing := `{"hooks":{"PostToolUse":[{"matcher":"Write|Edit","hooks":[{"type":"command","command":"echo keep"},{"type":"command","command":"sessions hook so sessions hook --vendor=factory post-tool"}]}]}}`
	if err := os.WriteFile(path, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	changed, err := removeFactoryEndpointHooks(path)
	if err != nil {
		t.Fatalf("removeFactoryEndpointHooks returned error: %v", err)
	}
	if !changed {
		t.Fatal("expected endpoint hook removal")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "echo keep") {
		t.Fatalf("user hook in mixed group was not preserved:\n%s", text)
	}
	if strings.Contains(text, "sessions hook") {
		t.Fatalf("endpoint hook was not removed:\n%s", text)
	}
}

func TestReadFactorySettingsReturnsCorruptJSONError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
		t.Fatalf("write corrupt settings: %v", err)
	}

	if _, err := readFactorySettings(path); err == nil {
		t.Fatal("expected corrupt settings error")
	}
}

func TestInstallFactorySettingsHandlesNullHooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"hooks":null}`), 0600); err != nil {
		t.Fatal(err)
	}

	if err := installFactorySettings(path, "/tmp/so", "/tmp/runtime.jsonl", "/tmp/config.json"); err != nil {
		t.Fatalf("installFactorySettings returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if !strings.Contains(string(data), "UserPromptSubmit") {
		t.Fatalf("Factory hooks were not installed from hooks:null:\n%s", string(data))
	}
}

func TestInstallFactorySettingsDoesNotReplaceUserCommandWithSuperopenEnvOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	existing := `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"sessions hook echo keep"}]}]}}`
	if err := os.WriteFile(path, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	if err := installFactorySettings(path, "/tmp/so", "/tmp/runtime.jsonl", "/tmp/config.json"); err != nil {
		t.Fatalf("installFactorySettings returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if !strings.Contains(string(data), "sessions hook echo keep") {
		t.Fatalf("user hook with Superopen-like env token was removed:\n%s", string(data))
	}
}

func TestFactorySettingsPathProjectLevel(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	target, err := factorySettingsPath(LevelProject)
	if err != nil {
		t.Fatalf("factorySettingsPath returned error: %v", err)
	}
	if got, want := target, filepath.Join(dir, ".factory", "settings.json"); got != want {
		t.Fatalf("project settings path = %q, want %q", got, want)
	}
}

func TestFactoryHookStatusDetectsInstalled(t *testing.T) {
	home := t.TempDir()
	testenv.SetHome(t, home)
	path := filepath.Join(home, ".factory", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"sessions hook so sessions hook --vendor=factory stop"}]}]}}`), 0600); err != nil {
		t.Fatal(err)
	}

	status := FactoryHookStatus(FactoryOptions{Level: LevelUser, UserMode: true})
	if !status.Installed {
		t.Fatalf("FactoryHookStatus installed = false, status=%#v", status)
	}
	if status.SettingsPath != path {
		t.Fatalf("SettingsPath = %q, want %q", status.SettingsPath, path)
	}
}

// jsonFragment renders a value as it appears inside a JSON document, minus the surrounding quotes.
//
// Assertions against marshaled settings must search for the encoded form: a Windows path is full
// of backslashes, and JSON escapes every one of them.
func jsonFragment(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode %q: %v", value, err)
	}
	return strings.Trim(string(encoded), `"`)
}

func TestInstallFactoryWritesHookCommand(t *testing.T) {
	home := t.TempDir()
	testenv.SetHome(t, home)

	status, err := InstallFactory(FactoryOptions{
		Level:    LevelUser,
		UserMode: true,
	})
	if err != nil {
		t.Fatalf("InstallFactory returned error: %v", err)
	}
	data, err := os.ReadFile(status.SettingsPath)
	if err != nil {
		t.Fatalf("read Factory settings: %v", err)
	}
	if !strings.Contains(string(data), "sessions hook --vendor=factory") {
		t.Fatalf("Factory hook command missing:\n%s", data)
	}
}
