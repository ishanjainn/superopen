package hook

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ishanjainn/superopen/internal/guards"
)

func TestGuardDenyBeforeShell(t *testing.T) {
	t.Setenv(guards.ProviderEnv, denyProvider(t))
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".so"), 0o755); err != nil {
		t.Fatal(err)
	}
	decision, ok := steerDecisionFor("cursor", "beforeShellExecution", "", shellPayload(t, "echo hi | bash", root))
	if !ok || !decision.deny || decision.text != "blocked" {
		t.Fatalf("decision: ok=%v %+v", ok, decision)
	}
}

func TestGuardUnsetAllows(t *testing.T) {
	t.Setenv(guards.ProviderEnv, "")
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".so"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ok := steerDecisionFor("cursor", "beforeShellExecution", "", shellPayload(t, "echo hi", root)); ok {
		t.Fatal("unset provider must not deny")
	}
}

func denyProvider(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		return compileDenyProvider(t, dir)
	}
	path := filepath.Join(dir, "deny.sh")
	body := "#!/bin/sh\nprintf '%s\\n' '{\"decision\":\"deny\",\"reason\":\"blocked\"}'\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func compileDenyProvider(t *testing.T, dir string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module deny\n\ngo 1.25.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := "package main\nimport \"fmt\"\nfunc main() {\n\tfmt.Println(`{\"decision\":\"deny\",\"reason\":\"blocked\"}`)\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "deny.exe")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build deny provider: %v\n%s", err, out)
	}
	return bin
}

func shellPayload(t *testing.T, command, cwd string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"command": command, "cwd": cwd})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
