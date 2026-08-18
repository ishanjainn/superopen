package engine

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGraphIgnoreOrderingAndDiscoveryBoundary(t *testing.T) {
	repo := t.TempDir()
	writeFixtureFile(t, repo, ".soignore", "generated/**\n!important.go\n")
	writeFixtureFile(t, repo, "generated/drop.go", "package generated")
	writeFixtureFile(t, repo, "important.go", "package sample")
	writeFixtureFile(t, repo, "keep.go", "package sample")
	files, err := discoverTrackedFiles(context.Background(), repo, []string{"*.go", "!keep.go", "!important.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 || files[0] != ".soignore" || files[1] != "important.go" || files[2] != "keep.go" {
		t.Fatalf("unexpected discovery: %v", files)
	}
	if runtime.GOOS != "windows" {
		outside := filepath.Join(t.TempDir(), "outside.go")
		if err := os.WriteFile(outside, []byte("package outside"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(repo, "escape.go")); err != nil {
			t.Fatal(err)
		}
		files, err = discoverTrackedFiles(context.Background(), repo, []string{"!escape.go"})
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			if file == "escape.go" {
				t.Fatal("repository-escaping symlink was discovered")
			}
		}
	}
}

func TestDiscoveryAppliesPinnedHardExclusionsAndSuffixes(t *testing.T) {
	repo := t.TempDir()
	writeFixtureFile(t, repo, "keep.go", "package keep")
	writeFixtureFile(t, repo, "vendor/drop.go", "package drop")
	writeFixtureFile(t, repo, "node_modules/drop.js", "drop()")
	writeFixtureFile(t, repo, "build.wasm", "binary")
	writeFixtureFile(t, repo, "package.json", `{"scripts":{}}`)
	files, err := discoverTrackedFiles(context.Background(), repo, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != "keep.go" {
		t.Fatalf("hard exclusions diverged: %v", files)
	}
}
