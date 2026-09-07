package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMergeClaudeAllowlistReplaceMerge(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	path := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	stale := `{
  "permissions": {
    "allow": [
      "Bash(so:*)",
      "Bash(/old/bin/so:*)",
      "Bash(git:*)"
    ]
  },
  "theme": "dark"
}`
	if err := os.WriteFile(path, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(home, "bin", "so")
	if runtime.GOOS == "windows" {
		bin = filepath.Join(home, "bin", "so.exe")
	}
	if _, err := installClaudeAllowlist(bin, false); err != nil {
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
		t.Fatalf("foreign settings lost: %s", body)
	}
	allow := jsonStringSlice(doc["permissions"].(map[string]any)["allow"])
	joined := strings.Join(allow, "\n")
	if !strings.Contains(joined, "Bash(so:*)") || !strings.Contains(joined, "Bash(so.exe:*)") {
		t.Fatalf("missing so allow rules: %s", body)
	}
	if !strings.Contains(joined, filepath.ToSlash(bin)) {
		t.Fatalf("missing absolute bin allow: %s", body)
	}
	if strings.Contains(joined, "/old/bin/so") {
		t.Fatalf("stale Superopen allow kept: %s", body)
	}
	if !strings.Contains(joined, "Bash(git:*)") {
		t.Fatalf("foreign allow dropped: %s", body)
	}

	again, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := installClaudeAllowlist(bin, false); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != string(second) {
		t.Fatalf("re-install not idempotent:\n%s\n---\n%s", again, second)
	}

	touched, err := StripClaudeAllowlist(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(touched) == 0 {
		t.Fatal("expected settings path stripped")
	}
	stripped, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stripped), "Bash(so") {
		t.Fatalf("superopen allow remained: %s", stripped)
	}
	if !strings.Contains(string(stripped), "Bash(git:*)") {
		t.Fatalf("foreign allow removed on uninstall: %s", stripped)
	}
}

func TestMergeClaudeAllowlistRefusesInvalidJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	path := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := mergeClaudeAllowlist(path, "/tmp/so")
	if err == nil {
		t.Fatal("expected parse error")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "{not json" {
		t.Fatalf("invalid JSON was overwritten: %q", got)
	}
}
