package userpaths_test

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/superopen/so/internal/userpaths"
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
