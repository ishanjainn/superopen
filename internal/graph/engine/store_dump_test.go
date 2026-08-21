package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestBuildFreshInsertsNodesAndSearchIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "graph.db")
	store, err := OpenWritableFresh(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	err = store.BuildFresh(context.Background(), func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{Name: "fixture", RootPath: t.TempDir(), Generation: "gen"}); err != nil {
			return err
		}
		id, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "Hello", QualifiedName: "fixture.Hello"})
		if err != nil {
			return err
		}
		if id == 0 {
			t.Fatal("expected node id")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Seal(context.Background()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.db.QueryRow(`SELECT count(*) FROM nodes_fts`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("fts rows=%d", count)
	}
}
