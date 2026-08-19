package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/ishanjainn/superopen/internal/graph/format"
)

func TestQuerySeedingPrefersDottedSymbol(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		foo, err := builder.PutNode(api.Node{Project: "fixture", Label: "Class", Name: "Foo", QualifiedName: "pkg.Foo", Location: api.Location{File: "pkg/foo.go", StartLine: 1}})
		if err != nil {
			return err
		}
		bar, err := builder.PutNode(api.Node{Project: "fixture", Label: "Method", Name: "bar", QualifiedName: "pkg.Foo.bar", Location: api.Location{File: "pkg/foo.go", StartLine: 10, EndLine: 20}, Properties: api.Properties{"is_exported": true}})
		if err != nil {
			return err
		}
		other, err := builder.PutNode(api.Node{Project: "fixture", Label: "Method", Name: "enable", QualifiedName: "pkg.Other.enable", Location: api.Location{File: "pkg/other.go", StartLine: 5}})
		if err != nil {
			return err
		}
		helper, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "setup", QualifiedName: "pkg.setup", Location: api.Location{File: "pkg/setup.go", StartLine: 1}})
		if err != nil {
			return err
		}
		_, _ = foo, other
		_, err = builder.PutEdge(api.Edge{Project: "fixture", SourceID: bar, TargetID: helper, Type: "CALLS", Evidence: &api.Evidence{Strategy: "import_map", Confidence: 0.9, Location: &api.Location{File: "pkg/foo.go", StartLine: 12}}})
		return err
	})
	defer store.Close()

	result, err := store.Query(ctx, api.QueryRequest{
		Project:  "fixture",
		Question: "How does Foo.bar work and what does it configure?",
		Budget:   2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Seeds) == 0 {
		t.Fatal("expected seeds")
	}
	if result.Seeds[0].QualifiedName != "pkg.Foo.bar" {
		t.Fatalf("expected pkg.Foo.bar first seed, got %#v", result.Seeds)
	}
	if !strings.Contains(result.Text, "NODE bar") {
		t.Fatalf("expected NODE line, got %q", result.Text)
	}
	if !strings.Contains(result.Text, "EDGE") || !strings.Contains(result.Text, "CALLS") {
		t.Fatalf("expected CALLS EDGE line, got %q", result.Text)
	}
	if !strings.Contains(result.Text, "Traversal: BFS") {
		t.Fatalf("expected traversal header, got %q", result.Text)
	}
}

func TestTraceAmbiguousShortName(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "Method", Name: "init", QualifiedName: "pkg.A.init", Location: api.Location{File: "a.go", StartLine: 1}}); err != nil {
			return err
		}
		if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "Method", Name: "init", QualifiedName: "pkg.B.init", Location: api.Location{File: "b.go", StartLine: 1}}); err != nil {
			return err
		}
		return nil
	})
	defer store.Close()

	result, err := store.Trace(ctx, api.TraceRequest{Project: "fixture", Start: "init", Direction: "outgoing", Depth: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "ambiguous" {
		t.Fatalf("status=%q suggestions=%d", result.Status, len(result.Suggestions))
	}
	if len(result.Suggestions) < 2 {
		t.Fatalf("expected suggestions, got %d", len(result.Suggestions))
	}
	text := format.TraceCompact(result)
	if !strings.Contains(text, "status: ambiguous") {
		t.Fatalf("compact=%q", text)
	}
}

func TestCompactTraceCalleesSubsetOfJSON(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		caller, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "Caller", QualifiedName: "pkg.Caller", Location: api.Location{File: "caller.go", StartLine: 1}})
		if err != nil {
			return err
		}
		called, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "Called", QualifiedName: "pkg.Called", Location: api.Location{File: "called.go", StartLine: 1}, Properties: api.Properties{"bt": "noise", "fp": "deadbeef", "sp": "1,2,3"}})
		if err != nil {
			return err
		}
		_, err = builder.PutEdge(api.Edge{Project: "fixture", SourceID: caller, TargetID: called, Type: "CALLS"})
		return err
	})
	defer store.Close()

	full, err := store.Trace(ctx, api.TraceRequest{Project: "fixture", Start: "pkg.Caller", Direction: "outgoing", Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Paths) == 0 {
		t.Fatal("expected paths")
	}
	// Full JSON path still carries properties.
	foundProps := false
	for _, path := range full.Paths {
		for _, step := range path {
			if step.Node.QualifiedName == "pkg.Called" && step.Node.Properties["bt"] == "noise" {
				foundProps = true
			}
		}
	}
	if !foundProps {
		t.Fatal("expected full trace to retain properties")
	}
	compact := format.TraceCompact(full)
	if strings.Contains(compact, "deadbeef") || strings.Contains(compact, `"bt"`) {
		t.Fatalf("compact leaked properties: %q", compact)
	}
	if !strings.Contains(compact, "Called") {
		t.Fatalf("compact missing callee: %q", compact)
	}
	if len(compact) > 2000 {
		t.Fatalf("compact too large: %d", len(compact))
	}
}

func TestQueryTextTruncationBanner(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		root, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "Root", QualifiedName: "pkg.Root", Location: api.Location{File: "root.go", StartLine: 1}})
		if err != nil {
			return err
		}
		for i := 0; i < 30; i++ {
			child, err := builder.PutNode(api.Node{
				Project: "fixture", Label: "Function", Name: "Child",
				QualifiedName: "pkg.Child" + string(rune('A'+i%26)) + string(rune('0'+i/26)),
				Location:      api.Location{File: "child.go", StartLine: i + 1},
			})
			if err != nil {
				return err
			}
			if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: root, TargetID: child, Type: "CALLS"}); err != nil {
				return err
			}
		}
		return nil
	})
	defer store.Close()

	result, err := store.Query(ctx, api.QueryRequest{
		Project:  "fixture",
		Question: "How does Root work?",
		Budget:   40, // force truncation
		Depth:    2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Budget.Truncated && !strings.Contains(result.Text, "TRUNCATED") {
		// Tiny budgets may truncate; accept either banner or truncated flag.
		t.Fatalf("expected truncation signal, text=%q budget=%+v", result.Text, result.Budget)
	}
}

func TestCommunityLabelConsistentWithArchitecture(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		a, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "A", QualifiedName: "pkg.A", Location: api.Location{File: "a.go", StartLine: 1}})
		if err != nil {
			return err
		}
		b, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "B", QualifiedName: "pkg.B", Location: api.Location{File: "b.go", StartLine: 1}})
		if err != nil {
			return err
		}
		c, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "C", QualifiedName: "pkg.C", Location: api.Location{File: "c.go", StartLine: 1}})
		if err != nil {
			return err
		}
		for _, edge := range []api.Edge{
			{Project: "fixture", SourceID: a, TargetID: b, Type: "CALLS"},
			{Project: "fixture", SourceID: b, TargetID: c, Type: "CALLS"},
			{Project: "fixture", SourceID: c, TargetID: a, Type: "CALLS"},
		} {
			if _, err := builder.PutEdge(edge); err != nil {
				return err
			}
		}
		return nil
	})
	defer store.Close()

	labels, err := store.communityLabelByNodeID(ctx, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	arch, err := store.Architecture(ctx, api.ArchitectureRequest{Project: "fixture", Aspects: []string{"clusters"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(arch.Communities) == 0 {
		t.Fatal("expected communities")
	}
	// Every labeled node should map to one of the architecture community names.
	names := map[string]struct{}{}
	for _, community := range arch.Communities {
		names[community.Name] = struct{}{}
	}
	for _, name := range labels {
		if _, ok := names[name]; !ok && name != "" {
			t.Fatalf("label %q not in architecture communities %#v", name, names)
		}
	}
}

func fixtureGraph(t *testing.T, build func(*Builder) error) *Store {
	t.Helper()
	path := t.TempDir() + "/graph.db"
	store, err := OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(context.Background(), func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{
			Name: "fixture", RootPath: t.TempDir(), Generation: "one",
			EngineVersion: "test", IndexedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		return build(builder)
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
	return store
}
