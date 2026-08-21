package engine

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestQuerySeedsFilePathFromToken(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "Read", QualifiedName: "pkg.Read", Location: api.Location{File: "pkg/read.go", StartLine: 1}}); err != nil {
			return err
		}
		if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "ReadDashboardConfig", QualifiedName: "pkg.dashboards.ReadDashboardConfig", Location: api.Location{File: "pkg/services/provisioning/dashboards/config_reader.go", StartLine: 23}}); err != nil {
			return err
		}
		if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "File", Name: "config_reader.go", QualifiedName: "pkg.dashboards.config_reader.go.__file__", Location: api.Location{File: "pkg/services/provisioning/dashboards/config_reader.go", StartLine: 1}}); err != nil {
			return err
		}
		return nil
	})
	defer store.Close()

	result, err := store.Query(ctx, api.QueryRequest{
		Project:  "fixture",
		Question: "Failed to read dashboards config error",
		Budget:   2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Text, "dashboards/config_reader.go") {
		t.Fatalf("expected path-shaped File NODE in query text, got %q", result.Text)
	}
	foundFile := false
	for _, seed := range result.Seeds {
		if seed.Label == "File" && strings.Contains(seed.Location.File, "config_reader.go") {
			foundFile = true
			break
		}
	}
	if !foundFile {
		t.Fatalf("expected File seed for config_reader.go, seeds=%#v", result.Seeds)
	}
}

func TestQueryNodeDisplayNameUsesPath(t *testing.T) {
	got := queryNodeDisplayName(api.Node{
		Label: "File", Name: "config_reader.go",
		Location: api.Location{File: "pkg/services/provisioning/dashboards/config_reader.go"},
	})
	if got != "dashboards/config_reader.go" {
		t.Fatalf("got %q", got)
	}
	fn := queryNodeDisplayName(api.Node{Label: "Function", Name: "ReadDashboardConfig"})
	if fn != "ReadDashboardConfig" {
		t.Fatalf("function name should stay, got %q", fn)
	}
}

func TestFilePathTokenScoreIgnoresReadmeSubstring(t *testing.T) {
	if filePathTokenScore("README.md", "README.md", "read") != 0 {
		t.Fatal("README.md must not seed from the token read")
	}
	if filePathTokenScore("pkg/services/provisioning/dashboards/config_reader.go", "config_reader.go", "dashboards") <= filePathTokenScore(".github/ISSUE_TEMPLATE/config.yml", "config.yml", "config") {
		t.Fatal("dashboards/config_reader.go must outrank generic config.yml")
	}
	if filePathTokenScore("pkg/services/provisioning/dashboards/config_reader.go", "config_reader.go", "config") < 10 {
		t.Fatal("config_reader.go should score for config")
	}
	files := rankFilePathSeeds([]api.Node{
		{Label: "File", Name: "config.json", Location: api.Location{File: "e2e-playwright/dashboards/cujs/config.json"}},
		{Label: "File", Name: "errors.go", Location: api.Location{File: "pkg/services/dashboards/errors.go"}},
		{Label: "File", Name: "config_reader.go", Location: api.Location{File: "pkg/services/provisioning/dashboards/config_reader.go"}},
	}, []string{"failed", "read", "dashboards", "config", "error"})
	if len(files) == 0 || files[0].Name != "config_reader.go" {
		t.Fatalf("expected config_reader.go first, got %#v", files)
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
		t.Fatalf("expected truncation signal, text=%q budget=%+v", result.Text, result.Budget)
	}
	if strings.Contains(strings.ToLower(result.Text), "grep") {
		t.Fatalf("truncation banner must not steer to Grep: %q", result.Text)
	}
	if !strings.Contains(result.Text, "raise the token budget") && !strings.Contains(result.Text, "--budget") {
		t.Fatalf("truncation banner should allow raising --budget, got %q", result.Text)
	}
	if strings.Contains(result.Text, "graph_search spray") {
		t.Fatalf("truncation banner must not invite a search spray: %q", result.Text)
	}
}

func TestQueryHubSkipDoesNotExpandTransitHubs(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		leaf, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "Leaf", QualifiedName: "pkg.Leaf", Location: api.Location{File: "leaf.go", StartLine: 1}})
		if err != nil {
			return err
		}
		hub, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "Hub", QualifiedName: "pkg.Hub", Location: api.Location{File: "hub.go", StartLine: 1}})
		if err != nil {
			return err
		}
		if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: leaf, TargetID: hub, Type: "CALLS"}); err != nil {
			return err
		}
		for i := 0; i < 80; i++ {
			other, err := builder.PutNode(api.Node{
				Project: "fixture", Label: "Function",
				Name:          "Other",
				QualifiedName: "pkg.Other" + string(rune('A'+i%26)) + string(rune('0'+i/26)),
				Location:      api.Location{File: "other.go", StartLine: i + 1},
			})
			if err != nil {
				return err
			}
			if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: hub, TargetID: other, Type: "CALLS"}); err != nil {
				return err
			}
		}
		return nil
	})
	defer store.Close()

	result, err := store.Query(ctx, api.QueryRequest{
		Project:  "fixture",
		Question: "How does Leaf work?",
		Budget:   8000,
		Depth:    2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Text, "NODE Leaf") {
		t.Fatalf("expected seed Leaf, got %q", result.Text)
	}
	others := 0
	for _, node := range result.Nodes {
		if strings.HasPrefix(node.Name, "Other") || strings.HasPrefix(node.QualifiedName, "pkg.Other") {
			others++
		}
	}
	if others > 5 {
		t.Fatalf("hub skip should not pull Other* neighborhood, got %d others in %d nodes", others, len(result.Nodes))
	}
}

func TestQueryAgentJSONOmitsNodeArray(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		root, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "Root", QualifiedName: "pkg.Root", Location: api.Location{File: "root.go", StartLine: 1}})
		if err != nil {
			return err
		}
		for i := 0; i < 40; i++ {
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

	result, err := store.Query(ctx, api.QueryRequest{Project: "fixture", Question: "How does Root work?", Budget: 2000})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) < 2 {
		t.Fatalf("internal result should still carry nodes, got %d", len(result.Nodes))
	}
	raw, err := json.Marshal(format.QueryAgentJSON(result))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(`"nodes":`)) || bytes.Contains(raw, []byte(`"edges":`)) {
		t.Fatalf("agent JSON must not include nodes/edges arrays: %s", raw)
	}
	if len(raw) > 64*1024 {
		t.Fatalf("agent JSON too large: %d", len(raw))
	}
	if !bytes.Contains(raw, []byte(`"text"`)) {
		t.Fatalf("agent JSON missing text: %s", raw[:200])
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
