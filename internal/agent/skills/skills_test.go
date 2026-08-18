package skills_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/agent/skills"
)

func TestInstallAllWritesSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))

	written, err := skills.InstallAll("/usr/local/bin/so")
	if err != nil {
		t.Fatal(err)
	}
	if len(written) == 0 {
		t.Fatal("expected skill paths")
	}
	for _, path := range written {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("missing %s: %v", path, err)
		}
		if !bytes.Contains(body, []byte("/usr/local/bin/so")) {
			t.Fatalf("%s missing absolute so path", path)
		}
		if bytes.Contains(body, []byte("__SO_BIN__")) {
			t.Fatalf("%s still has placeholder", path)
		}
	}
}
