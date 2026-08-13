package userpaths_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/userpaths"
)

func TestConfigAndDataDirs(t *testing.T) {
	cfg, err := userpaths.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filepath.ToSlash(cfg), "superopen") {
		t.Fatalf("config dir %q missing superopen", cfg)
	}
	data, err := userpaths.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filepath.ToSlash(data), "superopen") {
		t.Fatalf("data dir %q missing superopen", data)
	}
	mp, err := userpaths.CodexMarketplaceDir()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(filepath.ToSlash(mp), "codex-marketplace") {
		t.Fatalf("marketplace %q", mp)
	}
}

func TestVendorHomeOverrides(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(root, "codex"))
	t.Setenv("COPILOT_HOME", filepath.Join(root, "copilot"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))

	tests := []struct {
		name string
		got  func() (string, error)
		want string
	}{
		{"codex", userpaths.CodexHome, filepath.Join(root, "codex")},
		{"copilot", userpaths.CopilotHome, filepath.Join(root, "copilot")},
		{"opencode config", userpaths.OpenCodeConfigDir, filepath.Join(root, "config", "opencode")},
		{"opencode data", userpaths.OpenCodeDataDir, filepath.Join(root, "data", "opencode")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.got()
			if err != nil || got != tt.want {
				t.Fatalf("got %q, %v; want %q", got, err, tt.want)
			}
		})
	}

	// Ensure the test does not accidentally depend on inherited values.
	if os.Getenv("CODEX_HOME") == "" {
		t.Fatal("CODEX_HOME override was not installed")
	}
}

func TestEscapeJSONString(t *testing.T) {
	// The escaped result must round-trip through a JSON string literal, which
	// is how hook manifests embed the binary path.
	for _, in := range []string{
		`C:\Users\RUNNER~1\AppData\Local\Temp\so.exe`,
		`C:\Program Files\Superopen\so.exe`,
		`/tmp/bin/so`,
		`weird "quoted" path`,
		``,
	} {
		var got string
		literal := `"` + userpaths.EscapeJSONString(in) + `"`
		if err := json.Unmarshal([]byte(literal), &got); err != nil {
			t.Fatalf("EscapeJSONString(%q) -> %s is not a valid JSON string: %v", in, literal, err)
		}
		if got != in {
			t.Fatalf("round-trip mismatch: got %q, want %q", got, in)
		}
	}
}

func TestQuoteAndShellPath(t *testing.T) {
	q := userpaths.QuoteForHook(`/tmp/so`)
	if q == "" {
		t.Fatal("empty quote")
	}
	if runtime.GOOS == "windows" {
		sp := userpaths.ShellPath(`C:\Users\me\so.exe`)
		if strings.Contains(sp, `\`) {
			t.Fatalf("expected forward slashes, got %q", sp)
		}
	}
	if !userpaths.IsSoBinary("so") || !userpaths.IsSoBinary("so.exe") {
		t.Fatal("IsSoBinary")
	}
}
