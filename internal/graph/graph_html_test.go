package graph

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
