package initcmd_test

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/ishanjainn/superopen/internal/artifactmeta"
	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/initcmd"
)

func TestFreshInitCreatesOnlyDescribedV2HarnessFiles(t *testing.T) {
	repo := t.TempDir()
	emptyPath := t.TempDir()
	t.Setenv("PATH", emptyPath)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

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
		if err := artifactmeta.Validate(path); err != nil {
			t.Errorf("description contract: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	want := []string{
		".gitignore", "audit/events.jsonl", "config.yaml", "evals.yaml", "graph/corpus.json",
		"graph/graph.html", "graph/graph.json", "graph/state.json", "guardrails.yaml",
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
