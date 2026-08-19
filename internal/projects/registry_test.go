package projects_test

import (
	"fmt"
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

func TestRegisterSkipsTempRepos(t *testing.T) {
	// The real registry (no test override): a temp repo must not be recorded.
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	projects.SetPathForTest("")
	t.Cleanup(func() { projects.SetPathForTest("") })

	p, err := projects.Register(filepath.Join(t.TempDir(), "scratch"), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "scratch" {
		t.Fatalf("caller should still get the project: %+v", p)
	}
	list, err := projects.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("temp repo must stay out of the registry, got %d", len(list))
	}
}

func TestRegisterSkipsNonGitAndPruneInvalid(t *testing.T) {
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	projects.SetPathForTest("")
	t.Cleanup(func() { projects.SetPathForTest("") })

	// Durable (non-temp) workspace so ephemeral() does not fire.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(wd, "testdata", "eligible-"+filepath.Base(t.Name()))
	t.Cleanup(func() { _ = os.RemoveAll(workspace) })
	nonGit := filepath.Join(workspace, "not-a-repo")
	gitRepo := filepath.Join(workspace, "real-repo")
	if err := os.MkdirAll(nonGit, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(gitRepo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := projects.Register(nonGit, "", ""); err != nil {
		t.Fatal(err)
	}
	got, err := projects.Register(gitRepo, "", "")
	if err != nil {
		t.Fatal(err)
	}

	list, err := projects.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].RepoRoot != gitRepo {
		t.Fatalf("want only git repo registered, got %+v", list)
	}

	path, err := projects.Path()
	if err != nil {
		t.Fatal(err)
	}
	write := fmt.Sprintf(`{
  "projects": [
    {
      "id": %q,
      "name": "real-repo",
      "repo_root": %q,
      "so_root": %q,
      "last_seen_at": "2026-01-01T00:00:00Z"
    },
    {
      "id": "deadbeefdeadbeef",
      "name": "junk",
      "repo_root": %q,
      "so_root": %q,
      "last_seen_at": "2026-01-01T00:00:00Z"
    }
  ],
  "active_project_id": %q
}
`, got.ID, gitRepo, filepath.Join(gitRepo, ".so"), nonGit, filepath.Join(nonGit, ".so"), got.ID)
	if err := os.WriteFile(path, []byte(write), 0o600); err != nil {
		t.Fatal(err)
	}

	pruned, err := projects.PruneInvalid(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(pruned) != 1 || pruned[0].Project.ID != "deadbeefdeadbeef" {
		t.Fatalf("prune: %+v", pruned)
	}
	list, _ = projects.List()
	if len(list) != 1 || list[0].RepoRoot != gitRepo {
		t.Fatalf("after prune: %+v", list)
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
	if err := os.WriteFile(filepath.Join(so, "config.yaml"), []byte("project: test\n"), 0o644); err != nil {
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
