package graph

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestQueryRemovesGraphifyRepositoryCache(t *testing.T) {
	repo := t.TempDir()
	graphDir := filepath.Join(repo, ".so", "graph")
	cache := filepath.Join(graphDir, "cache")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "last_query_stamp"), []byte("legacy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(graphDir, "graph.json"), []byte(`{"nodes":[],"edges":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cacheHome := filepath.Join(t.TempDir(), "cache")
	t.Setenv("XDG_CACHE_HOME", cacheHome)
	t.Setenv("LOCALAPPDATA", cacheHome)
	if _, err := Query(repo, "anything"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Fatalf("Graphify query cache remained in .so: %v", err)
	}
}

func TestEnsureGraphifyCommunitySidecars(t *testing.T) {
	dir := t.TempDir()
	g := map[string]any{
		"nodes": []map[string]any{
			{"id": "a", "community": 1, "community_name": "Auth"},
			{"id": "b", "community": 1, "community_name": "Auth"},
			{"id": "c", "community": 2},
		},
	}
	raw, _ := json.Marshal(g)
	if err := os.WriteFile(filepath.Join(dir, "graph.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureGraphifyCommunitySidecars(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".graphify_labels.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".graphify_analysis.json")); err != nil {
		t.Fatal(err)
	}
	var labels map[string]string
	b, _ := os.ReadFile(filepath.Join(dir, ".graphify_labels.json"))
	_ = json.Unmarshal(b, &labels)
	if labels["1"] != "Auth" || labels["2"] != "Community 2" {
		t.Fatalf("labels: %+v", labels)
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
