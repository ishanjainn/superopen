package graph

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/harness"
)

func TestHtmlHasGraphifyCommunities(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.html")
	_ = os.WriteFile(empty, []byte(`<!doctype html><script>const LEGEND = [];</script>`), 0o644)
	if ok, reason := htmlHasGraphifyCommunities(empty); ok || reason == "" {
		t.Fatalf("empty LEGEND must fail, ok=%v reason=%q", ok, reason)
	}

	good := filepath.Join(dir, "good.html")
	_ = os.WriteFile(good, []byte(`<!doctype html><script>const LEGEND = [{"cid":0,"label":"Core","count":3}];</script>`), 0o644)
	if ok, reason := htmlHasGraphifyCommunities(good); !ok {
		t.Fatalf("non-empty LEGEND must pass: %s", reason)
	}
}

func TestQueryPreservesGraphifyRepositoryCache(t *testing.T) {
	repo := t.TempDir()
	graphDir := filepath.Join(repo, ".so", "graph")
	cache := filepath.Join(graphDir, "cache")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "last_query_stamp"), []byte("legacy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(graphDir, "graph.json"), []byte(`{"nodes":[{"id":"a"},{"id":"b"}],"edges":[{"source":"a","target":"b"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(graphDir, "state.json"), []byte(`{"schema_version":3,"engine":"graphify","engine_version":"0.9.44","graph_sha256":"abc"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SUPEROPEN_GRAPHIFY_BIN", fakeGraphify(t))
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cacheHome := filepath.Join(t.TempDir(), "cache")
	t.Setenv("XDG_CACHE_HOME", cacheHome)
	t.Setenv("LOCALAPPDATA", cacheHome)
	if _, err := Query(repo, "anything"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cache); err != nil {
		t.Fatalf("Graphify query cache was removed: %v", err)
	}
	stamp, err := os.ReadFile(filepath.Join(cache, "last_query_stamp"))
	if err != nil || !strings.Contains(string(stamp), `"graph_sha256": "abc"`) {
		t.Fatalf("successful query did not write current hash stamp: %s err=%v", stamp, err)
	}
}

func TestWriteGraphVocabularyIsCompactAndExcludesHarnessSources(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`{"nodes":[{"id":"service_run","label":"Run","community_name":"Core","source_file":"internal/service.go"},{"id":"bad","source_file":".so/memory/context.md"}],"links":[{"source":"service_run","target":"bad","relation":"calls"}]}`)
	if err := writeGraphVocabulary(dir, data); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "cache", "vocab.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	for _, want := range []string{"service_run", "Run", "Core", "internal/service.go", "calls"} {
		if !strings.Contains(got, want+"\n") {
			t.Errorf("vocabulary missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, ".so/") {
		t.Fatalf("harness source leaked into vocabulary: %s", got)
	}
}

func TestValidateNoHarnessSourcesRejectsEveryManagedSource(t *testing.T) {
	for _, source := range []string{".so/memory/context.md", ".mcp.json", ".cursor/mcp.json", "AGENTS.md", ".codex/skills/so/SKILL.md", "/tmp/repo/.factory/skills/so/SKILL.md"} {
		data, _ := json.Marshal(map[string]any{"nodes": []map[string]any{{"id": "bad", "source_file": source}}})
		if err := validateNoHarnessSources(data); err == nil {
			t.Errorf("managed source %q was accepted", source)
		}
	}
	if err := validateNoHarnessSources([]byte(`{"nodes":[{"id":"ok","source_file":"internal/service.go"}]}`)); err != nil {
		t.Fatalf("ordinary source rejected: %v", err)
	}
}

func TestValidateManifestSourcesRejectsManagedEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte(`{"internal/service.go":{},".mcp.json":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateManifestSources(path); err == nil {
		t.Fatal("managed manifest entry was accepted")
	}
}

func TestSanitizeManagedGraphArtifactsPrunesNodesEdgesAndManifest(t *testing.T) {
	dir := t.TempDir()
	graphBody := `{"nodes":[{"id":"good","source_file":"internal/service.go"},{"id":"bad","source_file":".mcp.json"}],"links":[{"source":"good","target":"bad","source_file":"internal/service.go"},{"source":"good","target":"good","source_file":"internal/service.go"}]}`
	if err := os.WriteFile(filepath.Join(dir, "graph.json"), []byte(graphBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"internal/service.go":{},".mcp.json":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := sanitizeManagedGraphArtifacts(dir)
	if err != nil || !changed {
		t.Fatalf("sanitize changed=%v err=%v", changed, err)
	}
	body, _ := os.ReadFile(filepath.Join(dir, "graph.json"))
	if strings.Contains(string(body), `"bad"`) || strings.Contains(string(body), ".mcp.json") {
		t.Fatalf("managed graph content survived sanitization: %s", body)
	}
	manifest, _ := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if strings.Contains(string(manifest), ".mcp.json") {
		t.Fatalf("managed manifest content survived sanitization: %s", manifest)
	}
}

func TestInstallGraphStagingSwapsDirectoryAndPreservesDurableArtifacts(t *testing.T) {
	repo := t.TempDir()
	paths := harness.Resolve(repo)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{
		filepath.Join(paths.GraphDir, "reflections", "LESSONS.md"): "durable lesson",
		filepath.Join(paths.GraphDir, "exports", "saved.svg"):      "durable export",
		filepath.Join(paths.GraphDir, ".graphify_learning.json"):   `{"durable":true}`,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	stage := filepath.Join(paths.GraphDir, ".staging-build")
	if err := os.MkdirAll(stage, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "graph.json"), []byte(`{"nodes":[{"id":"new"}],"edges":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installGraphStaging(stage, paths); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(paths.GraphDir, "reflections", "LESSONS.md"),
		filepath.Join(paths.GraphDir, "exports", "saved.svg"),
		filepath.Join(paths.GraphDir, ".graphify_learning.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("durable graph artifact was not preserved: %s: %v", path, err)
		}
	}
	if _, err := os.Stat(stage); !os.IsNotExist(err) {
		t.Fatalf("ordinary build staging leaked into publication: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.Root, "graph.previous")); !os.IsNotExist(err) {
		t.Fatalf("previous publication was not cleaned up: %v", err)
	}
}

func TestQueryStampCanBeScopedToSessionStart(t *testing.T) {
	repo := t.TempDir()
	paths := harness.Resolve(repo)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.GraphState, []byte(`{"graph_sha256":"abc"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	before := time.Now().UTC().Add(-time.Second)
	if err := RecordQueryStamp(repo, "query"); err != nil {
		t.Fatal(err)
	}
	if !HasCurrentQueryStampSince(repo, before) {
		t.Fatal("current-session query stamp was not accepted")
	}
	if HasCurrentQueryStampSince(repo, time.Now().UTC().Add(time.Second)) {
		t.Fatal("query stamp from an earlier session was accepted")
	}
}

func TestSweepStaleGraphWorkRemovesSiblingTempDirs(t *testing.T) {
	repo := t.TempDir()
	paths := harness.Resolve(repo)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	leak := filepath.Join(paths.Root, ".graph-v2-leftover")
	if err := os.Mkdir(leak, 0o700); err != nil {
		t.Fatal(err)
	}
	prev := filepath.Join(paths.Root, "graph.previous")
	if err := os.Mkdir(prev, 0o700); err != nil {
		t.Fatal(err)
	}
	staging := filepath.Join(paths.GraphDir, ".staging-old")
	if err := os.Mkdir(staging, 0o700); err != nil {
		t.Fatal(err)
	}
	SweepStaleGraphWork(paths)
	if _, err := os.Stat(leak); !os.IsNotExist(err) {
		t.Fatal("expected .graph-v2-* sibling to be removed")
	}
	if _, err := os.Stat(prev); !os.IsNotExist(err) {
		t.Fatal("expected graph.previous sibling to be removed")
	}
	if _, err := os.Stat(staging); !os.IsNotExist(err) {
		t.Fatal("expected graph/.staging-* to be removed")
	}
}

func TestRefreshAtomicDoesNotLeaveSiblingGraphDirs(t *testing.T) {
	repo := t.TempDir()
	paths := harness.Resolve(repo)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cacheHome := filepath.Join(t.TempDir(), "cache")
	t.Setenv("XDG_CACHE_HOME", cacheHome)
	t.Setenv("LOCALAPPDATA", cacheHome)
	t.Setenv("SUPEROPEN_GRAPHIFY_BIN", fakeGraphify(t))
	leak := filepath.Join(paths.Root, ".graph-v2-851391172")
	if err := os.Mkdir(leak, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := RefreshAtomic(repo, paths, true, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(leak); !os.IsNotExist(err) {
		t.Fatal("refresh must sweep leftover .graph-v2-* siblings")
	}
	ents, err := os.ReadDir(paths.Root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".graph-v2-") || e.Name() == "graph.previous" || e.Name() == "graph.failed" {
			t.Fatalf("graph work leaked beside graph/: %s", e.Name())
		}
	}
	if _, err := os.Stat(paths.GraphJSON); err != nil {
		t.Fatal(err)
	}
}

func fakeGraphify(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "graphify")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then echo 'graphify 0.9.44'; exit 0; fi
if [ "$1" = "query" ]; then echo 'query result from graph'; exit 0; fi
/bin/mkdir -p "$GRAPHIFY_OUT"
if [ "$1" = "extract" ]; then
  printf '%s\n' '{"nodes":[{"id":"a","label":"A","community":0,"community_name":"Core"},{"id":"b","label":"B","community":0,"community_name":"Core"}],"edges":[{"source":"a","target":"b","relation":"calls"}]}' > "$GRAPHIFY_OUT/graph.json"
  exit 0
fi
if [ "$1" = "cluster-only" ]; then printf '%s\n' '{"0":"Core"}' > "$GRAPHIFY_OUT/.graphify_labels.json"; printf '%s\n' '{"communities":{"0":["a","b"]}}' > "$GRAPHIFY_OUT/.graphify_analysis.json"; exit 0; fi
if [ "$1" = "export" ] && [ "$2" = "html" ]; then
  printf '%s\n' '<!doctype html><script>const LEGEND = [{"cid":0,"label":"Core","count":2}];</script>' > "$GRAPHIFY_OUT/graph.html"
  exit 0
fi
exit 0
`
	if runtime.GOOS == "windows" {
		bin += ".cmd"
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
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}
