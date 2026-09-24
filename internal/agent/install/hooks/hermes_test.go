package hooks

import (
	"github.com/ishanjainn/superopen/internal/agent/install/hooks/testenv"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallHermesConfigPreservesExistingSettingsAndHooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	existing := `model: anthropic/claude-sonnet-4.6
hooks_auto_accept: true
hooks:
  pre_tool_call:
    - matcher: terminal
      command: ~/.hermes/agent-hooks/user-policy.sh
      timeout: 5
`
	if err := os.WriteFile(path, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	if err := installHermesConfig(path, "/tmp/superopen hooks", "/tmp/runtime.jsonl", "/tmp/config.json"); err != nil {
		t.Fatalf("installHermesConfig returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	text := string(data)
	// The command now begins with a quote, so the YAML emitter single-quotes the whole scalar and
	// doubles the quotes inside it. The value a parser hands back is unchanged, which is what the hook
	// assertions below are about -- so they are made against that rather than against the on-disk
	// escaping. The preserved-settings assertions stay on the raw text, where the escaping is the point.
	parsed := strings.ReplaceAll(text, "''", "'")
	for _, want := range []string{
		"'/tmp/superopen hooks' sessions hook --vendor=hermes",
	} {
		if !strings.Contains(parsed, want) {
			t.Fatalf("Hermes hook command missing %q:\n%s", want, text)
		}
	}
	for _, want := range []string{
		"model:",
		"anthropic/claude-sonnet-4.6",
		"hooks_auto_accept: true",
		"~/.hermes/agent-hooks/user-policy.sh",
		"on_session_start:",
		"pre_llm_call:",
		"pre_tool_call:",
		"post_tool_call:",
		"pre_approval_request:",
		"post_approval_response:",
		"subagent_stop:",
		"on_session_end:",
		"on_session_finalize:",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Hermes config missing %q:\n%s", want, text)
		}
	}
}

func TestInstallHermesConfigReplacesOnlyHermesSuperopenHooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	existing := `hooks:
  post_tool_call:
    - command: env sessions hook old-so sessions hook --vendor=hermes post-tool
    - command: /tmp/so sessions hook --vendor=hermes post-tool
    - command: env sessions hook so sessions hook --vendor=factory post-tool
    - command: echo keep
`
	if err := os.WriteFile(path, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	if err := installHermesConfig(path, "/tmp/new-so", "/tmp/runtime.jsonl", "/tmp/config.json"); err != nil {
		t.Fatalf("installHermesConfig returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "old-so") || strings.Contains(text, "/tmp/so ") {
		t.Fatalf("old Hermes endpoint hook was not replaced:\n%s", text)
	}
	for _, want := range []string{"--vendor=factory", "echo keep", "/tmp/new-so"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected preserved hooks and new hook %q:\n%s", want, text)
		}
	}
}

func TestRemoveHermesEndpointHooksPreservesOtherHooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	existing := `hooks:
  pre_tool_call:
    - matcher: terminal
      command: echo keep
    - command: env sessions hook so sessions hook --vendor=hermes pre-tool
    - command: env sessions hook so sessions hook --vendor=factory pre-tool
`
	if err := os.WriteFile(path, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}

	changed, err := removeHermesEndpointHooks(path)
	if err != nil {
		t.Fatalf("removeHermesEndpointHooks returned error: %v", err)
	}
	if !changed {
		t.Fatal("expected endpoint hook removal")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "--vendor=hermes") {
		t.Fatalf("Hermes endpoint hook was not removed:\n%s", text)
	}
	if !strings.Contains(text, "echo keep") || !strings.Contains(text, "--vendor=factory") {
		t.Fatalf("non-Hermes hooks were not preserved:\n%s", text)
	}
}

func TestHermesHookStatusDetectsInstalled(t *testing.T) {
	home := t.TempDir()
	testenv.SetHome(t, home)
	path := filepath.Join(home, ".hermes", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`hooks:
  on_session_end:
    - command: env sessions hook so sessions hook --vendor=hermes session-end
`), 0600); err != nil {
		t.Fatal(err)
	}

	status := HermesHookStatus(HermesOptions{Level: LevelUser, UserMode: true})
	if !status.Installed {
		t.Fatalf("HermesHookStatus installed = false, status=%#v", status)
	}
	if status.ConfigPath != path {
		t.Fatalf("ConfigPath = %q, want %q", status.ConfigPath, path)
	}
}

func TestHermesProjectLevelReturnsClearError(t *testing.T) {
	if _, err := hermesConfigPath(LevelProject); err == nil || !strings.Contains(err.Error(), "user-level config only") {
		t.Fatalf("hermesConfigPath project err = %v, want user-level config only", err)
	}
}
