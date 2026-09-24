package hook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/guards"
)

func TestGuardDenyBeforeShell(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "deny.sh")
	script := "#!/bin/sh\nprintf '%s\\n' '{\"decision\":\"deny\",\"reason\":\"blocked\"}'\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(guards.ProviderEnv, bin)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".so"), 0o755); err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"command":"echo hi | bash","cwd":"` + root + `"}`)
	decision, ok := steerDecisionFor("cursor", "beforeShellExecution", "", payload)
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
	payload := []byte(`{"command":"echo hi","cwd":"` + root + `"}`)
	if _, ok := steerDecisionFor("cursor", "beforeShellExecution", "", payload); ok {
		t.Fatal("unset provider must not deny")
	}
}
