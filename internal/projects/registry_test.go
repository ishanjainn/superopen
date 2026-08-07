package projects_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/projects"
)

func TestRegisterListUse(t *testing.T) {
	dir := t.TempDir()
	projects.SetPathForTest(filepath.Join(dir, "projects.json"))

	p, err := projects.Register(filepath.Join(dir, "repo-a"), filepath.Join(dir, "repo-a", ".so"), "")
	if err != nil {
		t.Fatal(err)
	}
	if p.ID == "" || p.Name != "repo-a" {
		t.Fatalf("unexpected project: %+v", p)
	}
	list, err := projects.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %v", list, err)
	}
	_, err = projects.Register(filepath.Join(dir, "repo-b"), "", "")
	if err != nil {
		t.Fatal(err)
	}
	list, _ = projects.List()
	if len(list) != 2 {
		t.Fatalf("want 2 projects, got %d", len(list))
	}
	active, err := projects.Use(p.ID)
	if err != nil || active.ID != p.ID {
		t.Fatalf("use: %+v %v", active, err)
	}
}

func TestRemovePurgeAndPruneMissing(t *testing.T) {
	dir := t.TempDir()
	projects.SetPathForTest(filepath.Join(dir, "projects.json"))

	repo := filepath.Join(dir, "alive")
	so := filepath.Join(repo, ".so")
	if err := os.MkdirAll(so, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(so, "config.yaml"), []byte("memory:\n  enabled: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := projects.Register(repo, so, "")
	if err != nil {
		t.Fatal(err)
	}

	goneRoot := filepath.Join(dir, "gone")
	goneSO := filepath.Join(dir, "orphan.so-data", ".so")
	if err := os.MkdirAll(goneSO, 0o755); err != nil {
		t.Fatal(err)
	}
	gp, err := projects.Register(goneRoot, goneSO, "")
	if err != nil {
		t.Fatal(err)
	}
	// Simulate deleted repo (leave .so orphan).
	_ = os.RemoveAll(goneRoot)

	res, err := projects.Remove(p.ID, projects.RemoveOptions{PurgeSO: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.PurgedSO || res.RepoMissing {
		t.Fatalf("unexpected remove result: %+v", res)
	}
	if _, err := os.Stat(so); !os.IsNotExist(err) {
		t.Fatalf(".so should be gone, stat=%v", err)
	}

	pruned, err := projects.PruneMissing(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(pruned) != 1 || pruned[0].Project.ID != gp.ID {
		t.Fatalf("prune: %+v", pruned)
	}
	if _, err := os.Stat(goneSO); !os.IsNotExist(err) {
		t.Fatalf("orphan .so should be purged, stat=%v", err)
	}
	list, _ := projects.List()
	if len(list) != 0 {
		t.Fatalf("want empty registry, got %d", len(list))
	}
}
