package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestStripPathList(t *testing.T) {
	dir := filepath.Join("Users", "me", ".superopen", "bin")
	sep := string(os.PathListSeparator)
	path := filepath.Join("usr", "bin") + sep + dir + sep + filepath.Join("usr", "local", "bin")
	got, changed := stripPathList(path, []string{dir})
	if !changed {
		t.Fatal("expected change")
	}
	if got != filepath.Join("usr", "bin")+sep+filepath.Join("usr", "local", "bin") {
		t.Fatalf("got %q", got)
	}
	again, changed := stripPathList(got, []string{dir})
	if changed || again != got {
		t.Fatal("second strip should be a no-op")
	}
}

func TestStripPathListWindowsFold(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("case-fold is Windows-only")
	}
	got, changed := stripPathList(`C:\Tools;C:\Users\Me\.superopen\bin;C:\Windows`, []string{`c:\users\me\.superopen\bin`})
	if !changed {
		t.Fatal("expected change")
	}
	if got != `C:\Tools;C:\Windows` {
		t.Fatalf("got %q", got)
	}
}
