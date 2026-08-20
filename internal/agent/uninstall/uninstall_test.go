package uninstall

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/paths"
)

// TestRemovePath covers the three states the helper has to handle:
// missing path (no-op, no error), existing file (removed unless
// dry-run), existing directory (recursive remove unless dry-run).
//
// These are the failure modes that bit us during install: a stale
// re-run had to be safe, and a missing path on a fresh box couldn't
// surface as a noisy error.
func TestRemovePath(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	t.Run("missing path is a no-op", func(t *testing.T) {
		t.Parallel()
		missing := filepath.Join(t.TempDir(), "does-not-exist")
		path, err := removePath(missing, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if path != "" {
			t.Fatalf("expected empty path for missing target, got %q", path)
		}
	})

	t.Run("existing directory is removed", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(tmp, "vendor-dir")
		nested := filepath.Join(dir, "hooks", "hooks.json")
		if err := os.MkdirAll(filepath.Dir(nested), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(nested, []byte("{}"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}

		path, err := removePath(dir, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if path != dir {
			t.Fatalf("expected returned path %q, got %q", dir, path)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("expected directory removed, stat returned %v", err)
		}
	})

	t.Run("dry-run leaves disk untouched", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(tmp, "dry-run-dir")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		path, err := removePath(dir, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if path != dir {
			t.Fatalf("dry-run should report the path that would be removed; got %q", path)
		}
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("dry-run must not touch disk; stat error: %v", err)
		}
	})
}

// TestVendorsFromArg is a regression guard for the inverse-of-install
// vendor parser. If install/ ever adds a new vendor, this test should
// be updated in the same patch so uninstall stays symmetric.
func TestVendorsFromArg(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want []string
	}{
		{"all", []string{"claude-code", "cursor", "codex", "gemini", "opencode", "copilot-cli", "pi"}},
		{"cc", []string{"claude-code"}},
		{"claude-code", []string{"claude-code"}},
		{"cursor", []string{"cursor"}},
		{"codex", []string{"codex"}},
	}
	for _, tc := range cases {
		got, err := vendorsFromArg(tc.in)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.in, err)
		}
		if !equalSlice(got, tc.want) {
			t.Fatalf("%s: got %v, want %v", tc.in, got, tc.want)
		}
	}

	if _, err := vendorsFromArg("nope"); err == nil {
		t.Fatalf("unknown vendor should error")
	}
}

func TestPurgeSharedRemovesProductStateNotHomebrew(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("SUPEROPEN_INSTALL_DIR", "")

	mustDir := func(p string) {
		t.Helper()
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := paths.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	data, err := paths.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	curlRoot, err := paths.CurlInstallRoot()
	if err != nil {
		t.Fatal(err)
	}
	mustDir(cfg)
	mustDir(filepath.Join(data, "codex-marketplace"))
	mustDir(filepath.Join(curlRoot, "bin"))
	if err := os.WriteFile(filepath.Join(cfg, "projects.json"), []byte(`{"projects":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if cache, err := os.UserCacheDir(); err == nil {
		mustDir(filepath.Join(cache, "so"))
		mustDir(filepath.Join(cache, "superopen"))
	}

	removed, errs := purgeShared(false, true)
	if len(errs) > 0 {
		t.Fatalf("purge errors: %v", errs)
	}
	if len(removed) == 0 {
		t.Fatal("expected paths removed")
	}
	for _, p := range []string{cfg, data, curlRoot} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("%s should be gone, stat=%v", p, err)
		}
	}
}

func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
