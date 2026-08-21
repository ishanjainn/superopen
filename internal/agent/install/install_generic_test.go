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
	if !strings.Contains(string(body), "prependBashNudge") || !strings.Contains(string(body), `" ; "`) {
		t.Fatalf("opencode plugin should apply Windows-safe bash echo nudge")
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
	piBody, err := os.ReadFile(wantPi)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(piBody), "registerGraphTools") {
		t.Fatalf("pi plugin should keep native graph_* tools")
	}
	if !strings.Contains(string(piBody), "prependBashNudge") {
		t.Fatalf("pi plugin should apply bash echo nudge")
	}
	if !strings.Contains(string(piBody), "startsWith(\"graph_\")") {
		t.Fatalf("pi plugin must skip graph_* tools when rewriting commands")
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

func TestCopilotAgentStopDoesNotFinalize(t *testing.T) {
	body, err := copilotManifest("/usr/bin/so")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, `"sessionEnd"`) || !strings.Contains(body, "sessions finalize --detach") {
		t.Fatalf("sessionEnd must detach finalize:\n%s", body)
	}
	idx := strings.Index(body, `"agentStop"`)
	if idx < 0 {
		t.Fatal("missing agentStop")
	}
	block := body[idx:]
	if end := strings.Index(block, "]"); end > 0 {
		block = block[:end]
	}
	if strings.Contains(block, "sessions finalize") {
		t.Fatalf("agentStop must not run sessions finalize:\n%s", block)
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
	for _, field := range []string{`"bash":`, `"powershell":`, `"timeoutSec": 15`, `"userPromptTransformed":`} {
		if !strings.Contains(string(body), field) {
			t.Fatalf("Copilot hook manifest lacks %s: %s", field, body)
		}
	}
	if !strings.Contains(string(body), `"agentStop"`) {
		t.Fatalf("Copilot hook manifest missing agentStop: %s", body)
	}
	agentStopIdx := strings.Index(string(body), `"agentStop"`)
	sessionEndIdx := strings.Index(string(body), `"sessionEnd"`)
	if agentStopIdx < 0 || sessionEndIdx < 0 {
		t.Fatal("expected sessionEnd and agentStop")
	}
	agentStopBlock := string(body)[agentStopIdx:]
	if end := strings.Index(agentStopBlock, "]"); end > 0 {
		agentStopBlock = agentStopBlock[:end]
	}
	if strings.Contains(agentStopBlock, "sessions finalize") {
		t.Fatalf("Copilot agentStop must not finalize (that closes every turn): %s", agentStopBlock)
	}
	if !strings.Contains(string(body)[sessionEndIdx:], "sessions finalize --detach") {
		t.Fatal("Copilot sessionEnd must still detach finalize")
	}
}

func TestPatchPluginSoBinWindowsExe(t *testing.T) {
	src := `function soBin(): string {
  return process.env.SUPEROPEN_SO_BIN?.trim() || "so";
}`
	got := patchPluginSoBin(src, `C:\Users\me\AppData\Local\superopen\so.exe`)
	if strings.Contains(got, `|| "so";`) {
		t.Fatalf("windows so.exe must replace fallback so, got:\n%s", got)
	}
	if !strings.Contains(got, "so.exe") {
		t.Fatalf("patched plugin must pin so.exe, got:\n%s", got)
	}
	// strconv.Quote JSON-escapes backslashes so the TS string stays valid.
	if !strings.Contains(got, `C:\\Users\\me\\AppData\\Local\\superopen\\so.exe`) {
		t.Fatalf("windows path must be JS-quoted, got:\n%s", got)
	}
}
