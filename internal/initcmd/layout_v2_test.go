package initcmd_test

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/artifactmeta"
	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/initcmd"
)

func TestFreshInitCreatesOnlyDescribedV2HarnessFiles(t *testing.T) {
	repo := t.TempDir()
	emptyPath := t.TempDir()
	t.Setenv("PATH", emptyPath)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	graphify := filepath.Join(t.TempDir(), "graphify")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then echo 'graphify 0.9.44'; exit 0; fi
if [ "$1" = "query" ]; then echo 'query result from graph'; exit 0; fi
/bin/mkdir -p "$GRAPHIFY_OUT"
if [ "$1" = "extract" ]; then printf '%s\n' '{"nodes":[{"id":"a","community":0,"community_name":"Core"},{"id":"b","community":0,"community_name":"Core"}],"edges":[{"source":"a","target":"b"}]}' > "$GRAPHIFY_OUT/graph.json"; fi
if [ "$1" = "cluster-only" ]; then printf '%s\n' '{"0":"Core"}' > "$GRAPHIFY_OUT/.graphify_labels.json"; printf '%s\n' '{"communities":{"0":["a","b"]}}' > "$GRAPHIFY_OUT/.graphify_analysis.json"; fi
if [ "$1" = "export" ]; then printf '%s\n' '<!doctype html><script>const LEGEND = [{"cid":0,"label":"Core","count":2}];</script>' > "$GRAPHIFY_OUT/graph.html"; fi
exit 0
`
	if runtime.GOOS == "windows" {
		graphify += ".cmd"
		script = `@echo off
if "%1"=="--version" echo graphify 0.9.44& exit /b 0
if "%1"=="query" echo query result from graph& exit /b 0
if not exist "%GRAPHIFY_OUT%" mkdir "%GRAPHIFY_OUT%"
if "%1"=="extract" echo {"nodes":[{"id":"a","community":0},{"id":"b","community":0}],"edges":[{"source":"a","target":"b"}]} > "%GRAPHIFY_OUT%\graph.json"
if "%1"=="cluster-only" echo {"0":"Core"} > "%GRAPHIFY_OUT%\.graphify_labels.json"
if "%1"=="cluster-only" echo {"communities":{"0":["a","b"]}} > "%GRAPHIFY_OUT%\.graphify_analysis.json"
if "%1"=="export" echo ^<!doctype html^>^<script^>const LEGEND = [{"cid":0,"label":"Core","count":2}];^</script^> > "%GRAPHIFY_OUT%\graph.html"
exit /b 0
`
	}
	if err := os.WriteFile(graphify, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SUPEROPEN_GRAPHIFY_BIN", graphify)

	_, err := initcmd.Run(initcmd.Options{
		RepoRoot: repo, CodeOnly: true, NoLLM: true, SkipHooks: true, SkipInject: true,
		TemplateRoot: filepath.Join("..", "..", "templates"),
	})
	if err != nil {
		t.Fatal(err)
	}

	var files []string
	err = filepath.WalkDir(filepath.Join(repo, ".so"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(filepath.Join(repo, ".so"), path)
		files = append(files, filepath.ToSlash(rel))
		if !strings.HasPrefix(filepath.Base(path), ".graphify_") {
			if err := artifactmeta.Validate(path); err != nil {
				t.Errorf("description contract: %v", err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	want := []string{
		".gitignore", "audit/events.jsonl", "config.yaml", "evals.yaml", "graph/.graphify_analysis.json",
		"graph/.graphify_labels.json", "graph/corpus.json", "graph/graph.html", "graph/graph.json", "graph/state.json", "guardrails.yaml",
		"memory/context.md", "memory/state.json", "sessions/index.json",
	}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("fresh .so tree:\n got %v\nwant %v", files, want)
	}
	cfg, err := config.Load(filepath.Join(repo, ".so", "config.yaml"))
	if err != nil || cfg.LayoutVersion != 2 {
		t.Fatalf("layout version: %d (%v)", cfg.LayoutVersion, err)
	}
}

func TestInitRejectsUnsupportedLayoutWithoutAddingV2Files(t *testing.T) {
	repo := t.TempDir()
	soDir := filepath.Join(repo, ".so")
	if err := os.MkdirAll(soDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(soDir, "config.yaml"), []byte("vendors: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := initcmd.Run(initcmd.Options{RepoRoot: repo, CodeOnly: true, NoLLM: true, SkipHooks: true, SkipInject: true})
	if err == nil || !strings.Contains(err.Error(), "layout_version: 2") {
		t.Fatalf("expected unsupported-layout error, got %v", err)
	}
	entries, readErr := os.ReadDir(soDir)
	if readErr != nil || len(entries) != 1 || entries[0].Name() != "config.yaml" {
		t.Fatalf("unsupported layout was mutated: entries=%v err=%v", entries, readErr)
	}
}
