package inject

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectVendorsFindsCodexFromNativeHome(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	emptyPath := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", emptyPath)
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := DetectVendors(root)
	for _, vendor := range got {
		if vendor == "codex" {
			return
		}
	}
	t.Fatalf("DetectVendors() = %v, want codex from ~/.codex", got)
}

func TestDetectVendorsFindsCursorFromNativeHome(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	if err := os.MkdirAll(filepath.Join(home, "Library", "Application Support", "Cursor"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := DetectVendors(root)
	for _, vendor := range got {
		if vendor == "cursor" {
			return
		}
	}
	t.Fatalf("DetectVendors() = %v, want cursor from its native config home", got)
}

func TestStatusForIgnoresDisabledOptionalIntegrations(t *testing.T) {
	root := t.TempDir()
	agents := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(agents, []byte("# Superopen\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	codex := filepath.Join(root, ".codex", "skills", "so", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(codex), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(codex, []byte("name: so\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := StatusFor(root, []string{"codex"}, false)
	if len(got) != 2 || !got["AGENTS.md"] || !got["skill-codex"] {
		t.Fatalf("StatusFor() = %#v, want only healthy shared+Codex checks", got)
	}
	for _, disabled := range []string{"skill-agents", "skill-cursor", "cursor-rule"} {
		if _, exists := got[disabled]; exists {
			t.Fatalf("disabled integration %q was checked: %#v", disabled, got)
		}
	}
}

func TestVendorInstallCandidatesCoverDesktopPlatforms(t *testing.T) {
	env := func(key string) string {
		switch key {
		case "LOCALAPPDATA":
			return `C:\Users\me\AppData\Local`
		case "APPDATA":
			return `C:\Users\me\AppData\Roaming`
		default:
			return ""
		}
	}
	cases := []struct {
		vendor, goos, needle string
	}{
		{"cursor", "darwin", "Cursor.app"},
		{"codex", "darwin", "Codex.app"},
		{"cursor", "linux", "cursor.desktop"},
		{"cursor", "windows", "Cursor.exe"},
		{"codex", "windows", "Codex.exe"},
	}
	for _, tc := range cases {
		paths := vendorInstallCandidates(tc.vendor, tc.goos, "/home/me", env)
		joined := strings.Join(paths, "\n")
		if !strings.Contains(joined, tc.needle) {
			t.Errorf("%s/%s candidates %q do not contain %q", tc.goos, tc.vendor, joined, tc.needle)
		}
	}
}
