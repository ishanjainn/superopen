package engine

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func buildLayoutFixture(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "graph.db")
	store, err := OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{
			Name: "fixture", RootPath: t.TempDir(), Generation: "one",
			EngineVersion: "test", IndexedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		put := func(node api.Node) int64 {
			node.Project = "fixture"
			id, err := builder.PutNode(node)
			if err != nil {
				t.Fatal(err)
			}
			return id
		}
		folder := put(api.Node{Label: "Folder", Name: "src", QualifiedName: "fixture.folder.src"})
		hub := put(api.Node{Label: "Function", Name: "Hub", QualifiedName: "fixture.Hub", Location: api.Location{File: "src/core/hub.go"}})
		var spokes []int64
		for i := 0; i < 8; i++ {
			spokes = append(spokes, put(api.Node{
				Label: "Function", Name: fmt.Sprintf("Spoke%d", i),
				QualifiedName: fmt.Sprintf("fixture.Spoke%d", i),
				Location:      api.Location{File: "src/util/spokes.go"},
			}))
		}
		isolated := put(api.Node{Label: "Function", Name: "Isolated", QualifiedName: "fixture.Isolated", Location: api.Location{File: "src/lonely.go"}})
		_ = isolated
		edges := []api.Edge{
			// Containment edges must not count toward degree or appear in output.
			{Project: "fixture", SourceID: folder, TargetID: hub, Type: "CONTAINS_FILE"},
		}
		for _, spoke := range spokes {
			edges = append(edges, api.Edge{Project: "fixture", SourceID: hub, TargetID: spoke, Type: "CALLS"})
		}
		for _, edge := range edges {
			if _, err := builder.PutEdge(edge); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestLayoutRanksByDegreeAndPreservesGraphEdges(t *testing.T) {
	store := buildLayoutFixture(t)
	result, err := store.Layout(context.Background(), api.LayoutRequest{Project: "fixture", MaxNodes: 100})
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalNodes != 11 {
		t.Fatalf("total_nodes = %d, want 11", result.TotalNodes)
	}
	if got := len(result.Edges); got != 9 {
		t.Fatalf("edges = %d, want all 9 graph edges", got)
	}
	// Hub has the highest degree and must rank first.
	if result.Nodes[0].QualifiedName != "fixture.Hub" {
		t.Fatalf("first node = %q, want fixture.Hub", result.Nodes[0].QualifiedName)
	}
	if result.Nodes[0].Degree != 9 {
		t.Fatalf("hub degree = %d, want 9", result.Nodes[0].Degree)
	}
	if result.Nodes[0].Color == "" || result.Nodes[0].Size <= 0 {
		t.Fatal("stellar render metadata missing")
	}
}

func TestLayoutCoordinatesAreBoundedDeterministicAndSpread(t *testing.T) {
	store := buildLayoutFixture(t)
	first, err := store.Layout(context.Background(), api.LayoutRequest{Project: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Layout(context.Background(), api.LayoutRequest{Project: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Nodes) != len(second.Nodes) {
		t.Fatalf("node counts differ across runs: %d vs %d", len(first.Nodes), len(second.Nodes))
	}
	for i := range first.Nodes {
		if first.Nodes[i].X != second.Nodes[i].X || first.Nodes[i].Y != second.Nodes[i].Y {
			t.Fatalf("layout not deterministic at node %d", i)
		}
		if math.IsNaN(first.Nodes[i].X) || math.IsNaN(first.Nodes[i].Y) || math.IsNaN(first.Nodes[i].Z) {
			t.Fatalf("node %d has non-finite position", i)
		}
	}
	for i := 0; i < len(first.Nodes); i++ {
		for j := i + 1; j < len(first.Nodes); j++ {
			dist := math.Hypot(first.Nodes[i].X-first.Nodes[j].X, first.Nodes[i].Y-first.Nodes[j].Y)
			if dist < 1 {
				t.Fatalf("nodes %d and %d overlap (dist=%f)", i, j, dist)
			}
		}
	}
}

func TestLayoutClampsMaxNodes(t *testing.T) {
	store := buildLayoutFixture(t)
	result, err := store.Layout(context.Background(), api.LayoutRequest{Project: "fixture", MaxNodes: 999999})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) > layoutMaxMaxNodes {
		t.Fatalf("node cap not enforced: %d", len(result.Nodes))
	}
}
