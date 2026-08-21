package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindWebDirStandalone(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "server.js"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SUPEROPEN_WEB_DIR", dir)
	if got := findWebDir(""); got != dir {
		t.Fatalf("findWebDir() = %q, want %q", got, dir)
	}
}

func TestFindWebDirPackageJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SUPEROPEN_WEB_DIR", dir)
	if got := findWebDir(""); got != dir {
		t.Fatalf("findWebDir() = %q, want %q", got, dir)
	}
}

func TestUiServeCommandStandalone(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "server.js"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	command, err := uiServeCommand(dir, "/repo", "/bin/so", 4444, false)
	if err != nil {
		t.Fatal(err)
	}
	if command.Dir != dir {
		t.Fatalf("Dir = %q, want %q", command.Dir, dir)
	}
	if !strings.Contains(strings.Join(command.Args, " "), "server.js") {
		t.Fatalf("args = %v, want node server.js", command.Args)
	}
	env := strings.Join(command.Env, "\n")
	for _, needle := range []string{"PORT=4444", "HOSTNAME=127.0.0.1", "SUPEROPEN_ROOT=/repo", "SUPEROPEN_SO_BIN=/bin/so"} {
		if !strings.Contains(env, needle) {
			t.Fatalf("env missing %q", needle)
		}
	}
}

func TestUiServeCommandHotRejectsStandaloneWithTracedSrc(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "server.js"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := uiServeCommand(dir, "/repo", "/bin/so", 4444, true)
	if err == nil || !strings.Contains(err.Error(), "--hot") {
		t.Fatalf("err = %v, want --hot sources error for standalone prefix", err)
	}
}
