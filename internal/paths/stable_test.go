package paths_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/paths"
)

func TestUserBinDirHonorsInstallDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SUPEROPEN_INSTALL_DIR", dir)
	got, err := paths.UserBinDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("UserBinDir = %q want %q", got, dir)
	}
}

func TestUserBinDirDefaultIsCurlPrefix(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("SUPEROPEN_INSTALL_DIR", "")
	got, err := paths.UserBinDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(home, ".superopen", "bin") {
		t.Fatalf("UserBinDir = %q", got)
	}
}

func TestIsPackageManagedPath(t *testing.T) {
	managed := []string{
		"/opt/homebrew/bin/so",
		"/opt/homebrew/Cellar/so/1.0.0/bin/so",
		`C:\Users\me\scoop\apps\so\current\so.exe`,
		`C:\ProgramData\chocolatey\bin\so.exe`,
		`C:\Users\me\AppData\Local\Microsoft\WinGet\Packages\so\so.exe`,
	}
	for _, p := range managed {
		if !paths.IsPackageManagedPath(p) {
			t.Fatalf("expected managed: %s", p)
		}
	}
	if paths.IsPackageManagedPath(`/Users/me/.superopen/bin/so`) {
		t.Fatal("release prefix is not package-managed")
	}
	if paths.IsPackageManagedPath(`C:\Users\me\.superopen\bin\so.exe`) {
		t.Fatal("windows release prefix is not package-managed")
	}
}

func TestPathUnder(t *testing.T) {
	root := filepath.Join("Users", "me", ".superopen")
	bin := filepath.Join(root, "bin", "so")
	if !paths.PathUnder(bin, root) {
		t.Fatal("bin should be under root")
	}
	if paths.PathUnder(root, bin) {
		t.Fatal("root is not under bin")
	}
}

func TestPackageManagerUninstallHint(t *testing.T) {
	if got := paths.PackageManagerUninstallHint("/opt/homebrew/bin/so"); got != "brew uninstall so" {
		t.Fatalf("got %q", got)
	}
	if got := paths.PackageManagerUninstallHint(`C:\Users\me\scoop\apps\so\current\so.exe`); got != "scoop uninstall so" {
		t.Fatalf("got %q", got)
	}
	if paths.PackageManagerUninstallHint(`/Users/me/.superopen/bin/so`) != "" {
		t.Fatal("release installer should have empty hint")
	}
}

func TestIsHomebrewPath(t *testing.T) {
	if !paths.IsHomebrewPath("/opt/homebrew/bin/so") {
		t.Fatal("expected homebrew bin")
	}
	if !paths.IsHomebrewPath("/opt/homebrew/Cellar/so/1.0.0/bin/so") {
		t.Fatal("expected cellar")
	}
	if paths.IsHomebrewPath("/Users/me/.superopen/bin/so") {
		t.Fatal("curl prefix is not homebrew")
	}
	if paths.IsHomebrewPath("/Users/me/work/superopen/bin/so") {
		t.Fatal("checkout build is not homebrew")
	}
}

func TestRemoveUserBinFromPATH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell rc PATH is Unix-only")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	zprofile := filepath.Join(home, ".zprofile")
	body := "eval brew\n# Superopen CLI\nexport PATH=\"$HOME/.superopen/bin:$PATH\"\n"
	if err := os.WriteFile(zprofile, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	removed := paths.RemoveUserBinFromPATH()
	if len(removed) == 0 {
		t.Fatal("expected PATH snippet removal")
	}
	got, err := os.ReadFile(zprofile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), ".superopen/bin") {
		t.Fatalf("snippet survived: %s", got)
	}
	if !strings.Contains(string(got), "eval brew") {
		t.Fatalf("unrelated line dropped: %s", got)
	}
}

func TestIsSuperopenHookCommand(t *testing.T) {
	ours := []string{
		"/opt/homebrew/bin/so coding hook --vendor=cursor --event=sessionStart",
		"/Users/me/.superopen/bin/so graph refresh --detach",
		"/tmp/so sessions finalize",
		"so sessions refresh --detach",
	}
	for _, cmd := range ours {
		if !paths.IsSuperopenHookCommand(cmd) {
			t.Fatalf("expected ours: %s", cmd)
		}
	}
	if paths.IsSuperopenHookCommand("other-tool") {
		t.Fatal("foreign command matched")
	}
}
