package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// clineRenderedArgv extracts the argv the installer substituted into the plugin.
//
// Parsed back out of the source rather than compared as a string: argv is the whole point of this
// install shape, so the test asserts on the decoded array a host would spawn.
//
// The optional carriage return is load-bearing. The plugin source is a checked-in file, and a
// Windows checkout converts its line endings, so a bare `$` anchor matched on POSIX and failed on
// Windows -- for a file whose content was byte-for-byte correct. Line endings are not what this
// test is about.
func clineRenderedArgv(t *testing.T, source string) []string {
	t.Helper()
	match := regexp.MustCompile(`(?m)^const superopenArgv: string\[\] = (\[.*?\])\r?$`).FindStringSubmatch(source)
	if len(match) != 2 {
		t.Fatalf("no superopenArgv assignment in rendered plugin:\n%s", source)
	}
	var argv []string
	if err := json.Unmarshal([]byte(match[1]), &argv); err != nil {
		t.Fatalf("decode superopenArgv %q: %v", match[1], err)
	}
	return argv
}

func TestInstallClinePluginWritesManagedPlugin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "superopen.ts")
	if err := installClinePlugin(path, "/tmp/so", "/tmp/runtime.jsonl", "/tmp/config.json"); err != nil {
		t.Fatalf("installClinePlugin returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read plugin: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		clineManagedPluginMarker,
		"SuperopenEndpointPlugin",
		"SO_CLINE_DEBUG",
		"beforeTool",
		"afterTool",
		"beforeRun",
		"afterRun",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("plugin missing %q:\n%s", want, text)
		}
	}

	argv := clineRenderedArgv(t, text)
	if argv[0] != "/tmp/so" {
		t.Errorf("argv[0] = %q, want the hook binary", argv[0])
	}
	// The subcommand is the whole point of the invocation: without it the binary runs its root
	// command, exits, and every Cline event is silently dropped.
	joined := strings.Join(argv, " ")
	if !strings.Contains(joined, "sessions hook --vendor=cline") {
		t.Errorf("argv = %v, want sessions hook --vendor=cline", argv)
	}
}

func clineArgvHasFlag(argv []string, flag, value string) bool {
	for i, arg := range argv {
		if arg == flag {
			return i+1 < len(argv) && argv[i+1] == value
		}
	}
	return false
}

// Every path reaches the plugin through JSON encoding, which is why a Windows path needs no
// per-shell quoting: the backslashes and spaces that break a POSIX-quoted command line survive a
// round trip unchanged.
func TestRenderClinePluginEncodesWindowsPaths(t *testing.T) {
	binary := `C:\Program Files\Superopen\hooks\so.exe`
	logPath := `C:\ProgramData\Superopen\logs\runtime.jsonl`
	source, err := renderClinePlugin(binary, logPath, `C:\ProgramData\Superopen\config.json`)
	if err != nil {
		t.Fatalf("renderClinePlugin returned error: %v", err)
	}
	argv := clineRenderedArgv(t, source)
	if argv[0] != binary {
		t.Errorf("argv[0] = %q, want %q", argv[0], binary)
	}
	if !strings.Contains(strings.Join(argv, " "), "--vendor=cline") {
		t.Errorf("argv = %v, want sessions hook --vendor=cline", argv)
	}
}

func TestInstallClinePluginRefusesToOverwriteUnmanagedPlugin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "superopen.ts")
	original := "export default { name: \"someone-elses-plugin\" }"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	err := installClinePlugin(path, "/tmp/so", "/tmp/runtime.jsonl", "/tmp/config.json")
	if err == nil || !strings.Contains(err.Error(), "refusing to overwrite unmanaged") {
		t.Fatalf("install error = %v, want a refusal", err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil || string(data) != original {
		t.Fatalf("unmanaged plugin changed: err=%v body=%q", readErr, data)
	}
}

func TestRemoveClinePluginOnlyRemovesManagedPlugin(t *testing.T) {
	dir := t.TempDir()
	userPlugin := filepath.Join(dir, "user.ts")
	if err := os.WriteFile(userPlugin, []byte("export default { name: \"mine\" }"), 0644); err != nil {
		t.Fatalf("write user plugin: %v", err)
	}
	changed, err := removeClinePlugin(userPlugin)
	if err != nil {
		t.Fatalf("removeClinePlugin returned error: %v", err)
	}
	if changed {
		t.Fatal("user plugin should not be removed")
	}
	if _, err := os.Stat(userPlugin); err != nil {
		t.Fatalf("user plugin was removed: %v", err)
	}

	managed := filepath.Join(dir, "superopen.ts")
	if err := installClinePlugin(managed, "/tmp/so", "/tmp/runtime.jsonl", "/tmp/config.json"); err != nil {
		t.Fatalf("installClinePlugin returned error: %v", err)
	}
	changed, err = removeClinePlugin(managed)
	if err != nil || !changed {
		t.Fatalf("removeClinePlugin(managed) = %t, %v", changed, err)
	}
	if _, err := os.Stat(managed); !os.IsNotExist(err) {
		t.Fatalf("managed plugin still present: %v", err)
	}
}

func TestRemoveClinePluginIsQuietWhenNothingIsInstalled(t *testing.T) {
	changed, err := removeClinePlugin(filepath.Join(t.TempDir(), "superopen.ts"))
	if err != nil {
		t.Fatalf("removeClinePlugin returned error: %v", err)
	}
	if changed {
		t.Fatal("changed = true with no plugin present")
	}
}

func TestRenderClinePluginRejectsUnresolvedPlaceholders(t *testing.T) {
	if _, err := renderClinePluginTemplate("const x = \"__SO_UNKNOWN__\"", "/tmp/so", "/tmp/runtime.jsonl", "/tmp/config.json"); err == nil {
		t.Fatal("expected an error for a template with no argv placeholder")
	}
	template := clineArgvPlaceholder + "\nconst leftover = \"__SO_SOMETHING__\"\n"
	if _, err := renderClinePluginTemplate(template, "/tmp/so", "/tmp/runtime.jsonl", "/tmp/config.json"); err == nil {
		t.Fatal("expected an error for a leftover Superopen placeholder")
	}
}

func TestClineEmbeddedPluginMatchesRootSource(t *testing.T) {
	embedded, err := os.ReadFile(clineEmbeddedPluginSourcePath())
	if err != nil {
		t.Fatalf("read embedded plugin source: %v", err)
	}
	root, err := os.ReadFile(clineRootPluginSourcePath())
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("plugin source tree is not part of this repo")
		}
		t.Fatalf("read root plugin source: %v", err)
	}
	if string(embedded) != string(root) {
		t.Fatalf("embedded Cline plugin source drifted from root package source")
	}
}

func TestClinePluginPathsPerLevel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	userPath, err := clinePluginPath(LevelUser)
	if err != nil {
		t.Fatalf("clinePluginPath(user) returned error: %v", err)
	}
	if want := filepath.Join(home, ".cline", "plugins", "superopen.ts"); userPath != want {
		t.Errorf("user plugin path = %q, want %q", userPath, want)
	}

	projectPath, err := clinePluginPath(LevelProject)
	if err != nil {
		t.Fatalf("clinePluginPath(project) returned error: %v", err)
	}
	if !strings.HasSuffix(projectPath, filepath.Join(".cline", "plugins", "superopen.ts")) {
		t.Errorf("project plugin path = %q, want it under .cline/plugins", projectPath)
	}

	if _, err := clinePluginPath("nonsense"); err == nil {
		t.Error("clinePluginPath accepted an unknown level")
	}
}

// A plugin file outlives a Superopen uninstall, a half-applied update, or a home directory restored
// onto a machine where the binary lives elsewhere. In each case Cline loads a plugin that spawns
// nothing, and reporting that as installed tells an operator telemetry is being collected when none
// is.
func TestClineStatusRejectsAPluginWithNoBinary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "superopen.ts")
	missingBinary := filepath.Join(dir, "absent", "so")
	if err := installClinePlugin(path, missingBinary, "/tmp/runtime.jsonl", "/tmp/config.json"); err != nil {
		t.Fatalf("installClinePlugin returned error: %v", err)
	}

	status := clineStatusFromRuntime(runtimeStatus{Installed: true, BinaryPath: missingBinary, ConfigPath: path})
	if status.Installed {
		t.Error("status reports installed with no hook binary present")
	}
	if !strings.Contains(status.Message, "missing") {
		t.Errorf("message = %q, want it to name the missing binary", status.Message)
	}
}

// A plugin left behind by an older install points at a binary that has since moved. It parses, it
// loads, and it sends nothing.
func TestClineStatusRejectsAPluginPointingElsewhere(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "superopen.ts")
	currentBinary := filepath.Join(dir, "so")
	if err := os.WriteFile(currentBinary, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := installClinePlugin(path, filepath.Join(dir, "old", "so"), "/tmp/runtime.jsonl", "/tmp/config.json"); err != nil {
		t.Fatalf("installClinePlugin returned error: %v", err)
	}

	status := clineStatusFromRuntime(runtimeStatus{Installed: true, BinaryPath: currentBinary, ConfigPath: path})
	if status.Installed {
		t.Error("status reports installed while the plugin points at a different binary")
	}
}

func TestIsClineInstalledAtRequiresTheManagedMarker(t *testing.T) {
	dir := t.TempDir()
	unmanaged := filepath.Join(dir, "other.ts")
	if err := os.WriteFile(unmanaged, []byte("export default {}"), 0644); err != nil {
		t.Fatal(err)
	}
	if isClineInstalledAt(unmanaged) {
		t.Error("an unmanaged plugin was reported as Superopen's")
	}
	if isClineInstalledAt(filepath.Join(dir, "absent.ts")) {
		t.Error("a missing plugin was reported as installed")
	}
}

// The plugin holds its argv JSON-encoded, so a backslash appears doubled in the file. Comparing
// against the raw path therefore matched nothing on Windows, and every correctly installed plugin
// there was reported as not referencing its own binary. Asserted on the rendered source so the
// check is exercised on every platform, not only the one that exposed the bug.
func TestClinePluginReferencesBinaryHandlesWindowsPaths(t *testing.T) {
	binary := `C:\Program Files\Superopen\hooks\so.exe`
	source, err := renderClinePlugin(binary, `C:\ProgramData\Superopen\runtime.jsonl`, `C:\ProgramData\Superopen\config.json`)
	if err != nil {
		t.Fatalf("renderClinePlugin returned error: %v", err)
	}

	if !clinePluginReferencesBinary(source, binary) {
		t.Error("a plugin rendered for this binary was not recognized as referencing it")
	}
	// The shape of the original bug, stated so the escaping is not "simplified" back out.
	if strings.Contains(source, binary) {
		t.Error("rendered plugin contains the unescaped path; the escaping this guards has changed")
	}
	if clinePluginReferencesBinary(source, `C:\Program Files\Superopen\hooks\other-hooks.exe`) {
		t.Error("a plugin was reported as referencing a binary it does not spawn")
	}
	if clinePluginReferencesBinary(source, "") {
		t.Error("an empty binary path was treated as a match")
	}
}

func TestClinePluginReferencesBinaryOnPOSIXPaths(t *testing.T) {
	binary := "/opt/superopen/hooks/so"
	source, err := renderClinePlugin(binary, "/var/log/superopen/runtime.jsonl", "/etc/superopen/config.json")
	if err != nil {
		t.Fatalf("renderClinePlugin returned error: %v", err)
	}
	if !clinePluginReferencesBinary(source, binary) {
		t.Error("a plugin rendered for this binary was not recognized as referencing it")
	}
	if clinePluginReferencesBinary(source, "/opt/superopen/hooks/so-old") {
		t.Error("a plugin was reported as referencing a binary it does not spawn")
	}
}
