package testenv

import (
	"path/filepath"
	"runtime"
	"testing"
)

func SetHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	if runtime.GOOS == "windows" {
		t.Setenv("APPDATA", filepath.Join(dir, "AppData", "Roaming"))
		t.Setenv("LOCALAPPDATA", filepath.Join(dir, "AppData", "Local"))
	}
}

func RequirePOSIXFileModes(t *testing.T) {
	t.Helper()
	if !HasPOSIXFileModes() {
		t.Skip("Unix permission bits do not exist on Windows")
	}
}

func HasPOSIXFileModes() bool {
	return runtime.GOOS != "windows"
}
