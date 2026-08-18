package engine

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestArchitectureHotspotsMatchPinnedCallFanInSemantics(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "graph.db")
	store, err := OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{
			Name: "fixture", RootPath: t.TempDir(), Generation: "one",
			EngineVersion: "test", IndexedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		put := func(node api.Node) (int64, error) {
			node.Project = "fixture"
			return builder.PutNode(node)
		}
		if _, err := put(api.Node{Label: "File", Name: "caller.go", QualifiedName: "fixture.file.caller", Location: api.Location{File: "src/caller.go"}}); err != nil {
			return err
		}
		caller, err := put(api.Node{Label: "Function", Name: "Caller", QualifiedName: "fixture.Caller", Location: api.Location{File: "src/caller.go"}, Properties: api.Properties{"is_entry_point": true}})
		if err != nil {
			return err
		}
		called, err := put(api.Node{Label: "Function", Name: "Called", QualifiedName: "fixture.Called", Location: api.Location{File: "src/called.go"}})
		if err != nil {
			return err
		}
		testTarget, err := put(api.Node{Label: "Method", Name: "TestTarget", QualifiedName: "fixture.TestTarget", Location: api.Location{File: "src/contest.go"}, Properties: api.Properties{"is_test": true}})
		if err != nil {
			return err
		}
		usageOnly, err := put(api.Node{Label: "Function", Name: "UsageOnly", QualifiedName: "fixture.UsageOnly", Location: api.Location{File: "src/usage.go"}})
		if err != nil {
			return err
		}
		if _, err := put(api.Node{Label: "Route", Name: "/fallback", QualifiedName: "fixture.route", Location: api.Location{File: "src/http.go"}, Properties: api.Properties{"method": "POST", "path": "/orders", "handler": "Caller"}}); err != nil {
			return err
		}
		if _, err := put(api.Node{Label: "Class", Name: "Outside", QualifiedName: "fixture.docs.Outside", Location: api.Location{File: "docs/outside.py"}}); err != nil {
			return err
		}
		for _, edge := range []api.Edge{
			{Project: "fixture", SourceID: caller, TargetID: called, Type: "CALLS"},
			{Project: "fixture", SourceID: caller, TargetID: testTarget, Type: "CALLS"},
			{Project: "fixture", SourceID: caller, TargetID: usageOnly, Type: "USAGE"},
		} {
			if _, err := builder.PutEdge(edge); err != nil {
				return err
			}
		}
		return nil
	})
	if closeErr := store.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}

	store, err = OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	defaultView, err := store.Architecture(ctx, api.ArchitectureRequest{Project: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if len(defaultView.Languages) != 1 || len(defaultView.Packages) == 0 || len(defaultView.EntryPoints) != 1 || len(defaultView.Routes) != 0 || len(defaultView.Hotspots) != 0 || len(defaultView.FileTree) != 0 {
		t.Fatalf("default compact architecture=%+v", defaultView)
	}
	hotspots, err := store.Architecture(ctx, api.ArchitectureRequest{
		Project: "fixture", Path: "./src/", Aspects: []string{"hotspots"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hotspots.Hotspots) != 1 || hotspots.Hotspots[0].QualifiedName != "fixture.Called" || hotspots.Hotspots[0].Score != 1 {
		t.Fatalf("hotspots=%+v", hotspots.Hotspots)
	}
	if len(hotspots.Languages) != 0 || len(hotspots.EntryPoints) != 0 || len(hotspots.Routes) != 0 || len(hotspots.Boundaries) != 0 || len(hotspots.Layers) != 0 {
		t.Fatalf("unrequested aspects were populated: %+v", hotspots)
	}
	overview, err := store.Architecture(ctx, api.ArchitectureRequest{Project: "fixture", Path: "src", Aspects: []string{"overview"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Languages) != 1 || overview.Languages[0].Language != "Go" || overview.Languages[0].FileCount != 1 {
		t.Fatalf("languages=%+v", overview.Languages)
	}
	if labels, ok := overview.Aspects["node_labels"].(map[string]int); !ok || labels["Class"] != 0 {
		t.Fatalf("path-scoped label counts=%+v", overview.Aspects["node_labels"])
	}
	if overview.Path != "src" || overview.TotalNodes <= 0 || overview.RootTotalNodes <= overview.TotalNodes || overview.RootTotalEdges != overview.TotalEdges {
		t.Fatalf("scoped totals=%+v", overview)
	}
	if len(overview.EntryPoints) != 1 || overview.EntryPoints[0].QualifiedName != "fixture.Caller" {
		t.Fatalf("entry points=%+v", overview.EntryPoints)
	}
	if len(overview.Routes) != 1 || overview.Routes[0] != (api.Route{Method: "POST", Path: "/orders", Handler: "Caller"}) {
		t.Fatalf("routes=%+v", overview.Routes)
	}
	if len(overview.Boundaries) != 2 || overview.Boundaries[0] != (api.Boundary{From: "Caller", To: "Called", CallCount: 1}) || overview.Boundaries[1] != (api.Boundary{From: "Caller", To: "TestTarget", CallCount: 1}) {
		t.Fatalf("boundaries=%+v", overview.Boundaries)
	}
	wantLayers := map[string]string{"Caller": "entry", "Called": "leaf", "TestTarget": "leaf", "route": "api"}
	for _, layer := range overview.Layers {
		if want, ok := wantLayers[layer.Name]; ok {
			if layer.Layer != want {
				t.Errorf("layer %s=%s, want %s", layer.Name, layer.Layer, want)
			}
			delete(wantLayers, layer.Name)
		}
	}
	if len(wantLayers) != 0 {
		t.Fatalf("missing layers: %v from %+v", wantLayers, overview.Layers)
	}
	if len(overview.FileTree) != 0 {
		t.Fatalf("overview must omit the large file tree: %+v", overview.FileTree)
	}
	tree, err := store.Architecture(ctx, api.ArchitectureRequest{Project: "fixture", Aspects: []string{"file_tree"}})
	if err != nil {
		t.Fatal(err)
	}
	wantTree := []api.FileTreeEntry{{Path: "src", Type: "dir", Children: 1}, {Path: "src/caller.go", Type: "file", Children: 0}}
	if len(tree.FileTree) != len(wantTree) {
		t.Fatalf("file tree=%+v", tree.FileTree)
	}
	for index := range wantTree {
		if tree.FileTree[index] != wantTree[index] {
			t.Fatalf("file tree=%+v", tree.FileTree)
		}
	}
	cyclesOnly, err := store.Architecture(ctx, api.ArchitectureRequest{Project: "fixture", Aspects: []string{"cycles"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(cyclesOnly.Hotspots) != 0 {
		t.Fatalf("unrequested hotspots=%+v", cyclesOnly.Hotspots)
	}
	clusters, err := store.Architecture(ctx, api.ArchitectureRequest{Project: "fixture", Aspects: []string{"clusters"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters.Communities) != 1 || clusters.Communities[0].Members != 3 || clusters.Communities[0].Cohesion != 1 || len(clusters.Communities[0].TopNodes) == 0 || clusters.Communities[0].TopNodes[0] != "Caller" {
		t.Fatalf("clusters=%+v", clusters.Communities)
	}
	if _, err := store.Architecture(ctx, api.ArchitectureRequest{Project: "fixture", Aspects: []string{"hotpot"}}); err == nil {
		t.Fatal("unknown architecture aspect was accepted")
	}
}

func TestPinnedPackageQualifiedNameFallback(t *testing.T) {
	for qualifiedName, want := range map[string]string{
		"project.dir.sub.Symbol": "sub",
		"project.pkg.Symbol":     "pkg",
		"project.Symbol":         "Symbol",
		"Symbol":                 "",
	} {
		if got := pinnedPackageFromQualifiedName(qualifiedName); got != want {
			t.Errorf("pinnedPackageFromQualifiedName(%q)=%q, want %q", qualifiedName, got, want)
		}
	}
}
