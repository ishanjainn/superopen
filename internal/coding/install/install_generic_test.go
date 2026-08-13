package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstallOpenCodeAndPiUseHostDiscoveryPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	// resolveSoBin looks on PATH — plant a fake so.
	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	soName := "so"
	if runtime.GOOS == "windows" {
		soName += ".exe"
	}
	so := filepath.Join(binDir, soName)
	if err := os.WriteFile(so, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// Seed stale wrong-path installs that used to fool doctor.
	staleOC := filepath.Join(home, ".opencode", "plugins", "superopen.ts")
	stalePi := filepath.Join(home, ".pi", "extensions", "superopen", "index.ts")
	for _, p := range []string{staleOC, stalePi} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("// stale\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writtenOC, err := installGenericVendor("opencode", false)
	if err != nil {
		t.Fatal(err)
	}
	wantOC := filepath.Join(home, ".config", "opencode", "plugins", "superopen.ts")
	if _, err := os.Stat(wantOC); err != nil {
		t.Fatalf("opencode plugin missing at host path %s: %v (wrote %v)", wantOC, err, writtenOC)
	}
	if _, err := os.Stat(staleOC); !os.IsNotExist(err) {
		t.Fatalf("stale ~/.opencode/plugins/superopen.ts should be removed")
	}
	body, err := os.ReadFile(wantOC)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "client.session.message") {
		t.Fatalf("opencode plugin should hydrate assistant parts via client.session.message")
	}

	writtenPi, err := installGenericVendor("pi", false)
	if err != nil {
		t.Fatal(err)
	}
	wantPi := filepath.Join(home, ".pi", "agent", "extensions", "superopen", "index.ts")
	if _, err := os.Stat(wantPi); err != nil {
		t.Fatalf("pi extension missing at host path %s: %v (wrote %v)", wantPi, err, writtenPi)
	}
	if _, err := os.Stat(filepath.Join(home, ".pi", "extensions", "superopen")); !os.IsNotExist(err) {
		t.Fatalf("stale ~/.pi/extensions/superopen should be removed")
	}
}

func TestInstallGeminiMergesSettingsAndUsesCurrentEvents(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	path := filepath.Join(home, ".gemini", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	seed := `{"theme":"dark","hooks":{"BeforeTool":[{"matcher":"foreign","hooks":[{"type":"command","command":"foreign-tool"}]}],"PreToolUse":[{"hooks":[{"type":"command","command":"/old/so coding hook --vendor=gemini --event=PreToolUse"}]}]}}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := installGeminiHooks(path, filepath.Join(home, "bin with space", "so")); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["theme"] != "dark" {
		t.Fatalf("existing setting was lost: %s", body)
	}
	hooks := doc["hooks"].(map[string]any)
	for _, event := range []string{"SessionStart", "SessionEnd", "BeforeAgent", "AfterAgent", "BeforeTool", "AfterTool"} {
		if _, ok := hooks[event]; !ok {
			t.Fatalf("missing current Gemini event %s", event)
		}
	}
	if _, ok := hooks["PreToolUse"]; ok {
		t.Fatalf("obsolete Superopen event remains: %s", body)
	}
	if !strings.Contains(string(body), "foreign-tool") {
		t.Fatalf("foreign hook was lost: %s", body)
	}
}

func TestGenericVendorHonorsConfigOverrides(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	t.Setenv("COPILOT_HOME", filepath.Join(home, "copilot-home"))
	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	soName := "so"
	if runtime.GOOS == "windows" {
		soName += ".exe"
	}
	so := filepath.Join(binDir, soName)
	if err := os.WriteFile(so, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if _, err := installGenericVendor("opencode", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "xdg", "opencode", "plugins", "superopen.ts")); err != nil {
		t.Fatal(err)
	}
	if _, err := installGenericVendor("copilot-cli", false); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(home, "copilot-home", "hooks", "superopen.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"version": 1`) {
		t.Fatalf("Copilot hook manifest lacks version: %s", body)
	}
	for _, field := range []string{`"bash":`, `"powershell":`, `"timeoutSec": 5`, `"userPromptTransformed":`} {
		if !strings.Contains(string(body), field) {
			t.Fatalf("Copilot hook manifest lacks %s: %s", field, body)
		}
	}
}
