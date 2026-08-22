package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

func TestQuerySeedsCallableNotJSONFile(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		fn, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "registerPlugin", QualifiedName: "src.module.plugin.registerPlugin", Location: api.Location{File: "src/module.ts", StartLine: 10}})
		if err != nil {
			return err
		}
		dash, err := builder.PutNode(api.Node{Project: "fixture", Label: "File", Name: "dashboard.json", QualifiedName: "dashboard.json.__file__", Location: api.Location{File: "dashboard.json", StartLine: 1}})
		if err != nil {
			return err
		}
		for i := 0; i < 30; i++ {
			v, err := builder.PutNode(api.Node{
				Project: "fixture", Label: "Variable", Name: "title",
				QualifiedName: "dashboard.json.panel" + strconv.Itoa(i),
				Location:      api.Location{File: "dashboard.json", StartLine: i + 1},
			})
			if err != nil {
				return err
			}
			if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: dash, TargetID: v, Type: "DEFINES"}); err != nil {
				return err
			}
		}
		_ = fn
		return nil
	})
	defer store.Close()

	result, err := store.Query(ctx, api.QueryRequest{
		Project:  "fixture",
		Question: "How does this plugin register and load the dashboard?",
		Budget:   2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Seeds) == 0 {
		t.Fatal("expected seeds")
	}
	for i, seed := range result.Seeds {
		if i >= 3 {
			break
		}
		if seed.Label == "Variable" {
			t.Fatalf("primary seeds must not be JSON Variables: %#v", result.Seeds)
		}
		if seed.Label == "File" {
			t.Fatalf("query must not File-seed: %#v", result.Seeds)
		}
	}
	if result.Seeds[0].QualifiedName != "src.module.plugin.registerPlugin" && result.Seeds[0].Name != "registerPlugin" {
		t.Fatalf("expected registerPlugin seed, got %#v", result.Seeds)
	}
	variableLines := 0
	for _, line := range strings.Split(result.Text, "\n") {
		if strings.HasPrefix(line, "NODE ") && strings.Contains(line, "title") && strings.Contains(line, "dashboard.json") {
			variableLines++
		}
	}
	if variableLines > 5 {
		t.Fatalf("query body dominated by JSON keys: %d variable NODE lines, text=%q", variableLines, result.Text)
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

func TestQueryNodeLineIncludesQualifiedName(t *testing.T) {
	hit := queryNodeHit{node: api.Node{Label: "Function", Name: "bar", QualifiedName: "pkg.Foo.bar", Location: api.Location{File: "foo.go", StartLine: 10}}}
	line := formatQueryNodeLine(hit, nil)
	if !strings.Contains(line, "qn=pkg.Foo.bar") || !strings.Contains(line, "src=foo.go") {
		t.Fatalf("NODE line missing qn/src: %q", line)
	}
}

func TestQueryNodeLineFileSpanNotStartOnly(t *testing.T) {
	hit := queryNodeHit{node: api.Node{
		Label: "File", Name: "app.ts",
		Location: api.Location{File: "src/app.ts", StartLine: 1, EndLine: 75},
	}}
	line := formatQueryNodeLine(hit, nil)
	if !strings.Contains(line, "loc=L1-75") {
		t.Fatalf("File NODE must print full span, got %q", line)
	}
	if strings.Contains(line, "loc=L1 ") || strings.HasSuffix(strings.TrimSpace(line), "loc=L1]") {
		t.Fatalf("File NODE collapsed to start line: %q", line)
	}
	fn := formatQueryNodeLine(queryNodeHit{node: api.Node{
		Label: "Function", Name: "main", QualifiedName: "src.app.main",
		Location: api.Location{File: "src/app.ts", StartLine: 23, EndLine: 75},
	}}, nil)
	if !strings.Contains(fn, "loc=L23-75") {
		t.Fatalf("Function span: %q", fn)
	}
	one := formatQueryNodeLine(queryNodeHit{node: api.Node{
		Label: "Variable", Name: "FOO", QualifiedName: "src.app.FOO",
		Location: api.Location{File: "src/app.ts", StartLine: 3, EndLine: 3},
	}}, nil)
	if !strings.Contains(one, "loc=L3") || strings.Contains(one, "L3-3") {
		t.Fatalf("one-line symbol: %q", one)
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
	if strings.Contains(result.Text, "--budget") {
		t.Fatalf("truncation banner must not lead with --budget: %q", result.Text)
	}
	if !strings.Contains(result.Text, "so graph snippet") {
		t.Fatalf("truncation banner should suggest snippet, got %q", result.Text)
	}
	if strings.Contains(strings.ToLower(result.Text), "cypher") {
		t.Fatalf("truncation banner must not mention cypher: %q", result.Text)
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

func TestSnippetAmbiguousQualifiedAndFilePath(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "export function App() { return null }\n"
	if err := os.WriteFile(filepath.Join(root, "src", "module.tsx"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/graph.db"
	store, err := OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{
			Name: "fixture", RootPath: root, Generation: "one",
			EngineVersion: "test", IndexedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "App", QualifiedName: "src.module.App", Location: api.Location{File: "src/module.tsx", StartLine: 1, EndLine: 1}}); err != nil {
			return err
		}
		if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "App", QualifiedName: "src.pages.App", Location: api.Location{File: "src/pages.tsx", StartLine: 1, EndLine: 1}}); err != nil {
			return err
		}
		if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "File", Name: "module.tsx", QualifiedName: "src.module.tsx.__file__", Location: api.Location{File: "src/module.tsx", StartLine: 1, EndLine: 1}}); err != nil {
			return err
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

	ambiguous, err := store.Snippet(ctx, api.SnippetRequest{Project: "fixture", QualifiedName: "App"})
	if err != nil {
		t.Fatal(err)
	}
	if ambiguous.Status != "ambiguous" || len(ambiguous.Suggestions) < 2 {
		t.Fatalf("snippet App status=%q suggestions=%d", ambiguous.Status, len(ambiguous.Suggestions))
	}
	text := format.SnippetCompact(ambiguous)
	if !strings.Contains(text, "status: ambiguous") {
		t.Fatalf("compact=%q", text)
	}
	help := format.HelpForSnippet(ambiguous)
	if len(help) == 0 || !strings.Contains(help[0], "so graph snippet") {
		t.Fatalf("help=%v", help)
	}

	qualified, err := store.Snippet(ctx, api.SnippetRequest{Project: "fixture", QualifiedName: "src.module.App"})
	if err != nil {
		t.Fatal(err)
	}
	if qualified.Status == "ambiguous" || !strings.Contains(qualified.Code, "export function App") {
		t.Fatalf("qualified snippet: status=%q code=%q", qualified.Status, qualified.Code)
	}

	byPath, err := store.Snippet(ctx, api.SnippetRequest{Project: "fixture", QualifiedName: "src/module.tsx"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(byPath.Message), "symbol not found") || !strings.Contains(byPath.Code, "export function App") {
		t.Fatalf("path snippet failed: %#v", byPath)
	}
}

func TestTraceInboundUnresolvedCalls(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "statPanel", QualifiedName: "src.panels.statPanel", Location: api.Location{File: "src/panels.ts", StartLine: 1}}); err != nil {
			return err
		}
		return builder.PutUnresolved(api.UnresolvedRelationship{
			Project: "fixture", Source: "src.panels.statPanel", TargetText: "re.test", Type: "CALL_REFERENCE",
		})
	})
	defer store.Close()

	result, err := store.Trace(ctx, api.TraceRequest{Project: "fixture", Start: "src.panels.statPanel", Direction: "incoming", Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Paths) != 0 {
		t.Fatalf("expected empty inbound paths, got %d", len(result.Paths))
	}
	if result.UnresolvedCalls < 1 {
		t.Fatalf("expected unresolved_calls, got %d compact=%q", result.UnresolvedCalls, format.TraceCompact(result))
	}
	text := format.TraceCompact(result)
	if !strings.Contains(text, "unresolved_calls:") {
		t.Fatalf("compact missing unresolved_calls: %q", text)
	}
}

func TestTraceIncomingLabelsCallers(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		leaf, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "leaf", QualifiedName: "a.leaf", Location: api.Location{File: "a.ts", StartLine: 1, EndLine: 3}})
		if err != nil {
			return err
		}
		caller, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "useLeaf", QualifiedName: "b.useLeaf", Location: api.Location{File: "b.ts", StartLine: 1, EndLine: 5}})
		if err != nil {
			return err
		}
		_, err = builder.PutEdge(api.Edge{Project: "fixture", SourceID: caller, TargetID: leaf, Type: "CALLS"})
		return err
	})
	defer store.Close()

	result, err := store.Trace(ctx, api.TraceRequest{Project: "fixture", Start: "leaf", Direction: "incoming", Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if result.Direction != "incoming" {
		t.Fatalf("direction=%q", result.Direction)
	}
	text := format.TraceCompact(result)
	if !strings.Contains(text, "direction: incoming") {
		t.Fatalf("compact missing incoming: %q", text)
	}
	if !strings.Contains(text, "callers:") {
		t.Fatalf("incoming compact must say callers: %q", text)
	}
	if strings.Contains(text, "callees:") {
		t.Fatalf("incoming compact must not say callees: %q", text)
	}
	if !strings.Contains(text, "useLeaf") {
		t.Fatalf("expected caller useLeaf: %q", text)
	}
}

func TestSnippetPrefersFunctionOverModuleAndPathPicksFile(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	lines := make([]string, 80)
	for i := range lines {
		lines[i] = fmt.Sprintf("// line %d", i+1)
	}
	lines[19] = "export function foo() { return 1 }"
	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(root, "src", "foo.ts"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/graph.db"
	store, err := OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{
			Name: "fixture", RootPath: root, Generation: "one",
			EngineVersion: "test", IndexedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "Module", Name: "src/foo.ts", QualifiedName: "src.foo", Location: api.Location{File: "src/foo.ts", StartLine: 1, EndLine: 80}}); err != nil {
			return err
		}
		if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "File", Name: "foo.ts", QualifiedName: "src.foo.ts.__file__", Location: api.Location{File: "src/foo.ts", StartLine: 1, EndLine: 80}}); err != nil {
			return err
		}
		if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "foo", QualifiedName: "src.foo.foo", Location: api.Location{File: "src/foo.ts", StartLine: 20, EndLine: 20}}); err != nil {
			return err
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

	symbol, err := store.Snippet(ctx, api.SnippetRequest{Project: "fixture", QualifiedName: "foo"})
	if err != nil {
		t.Fatal(err)
	}
	if symbol.Status == "ambiguous" || symbol.Label != "Function" {
		t.Fatalf("symbol foo: status=%q label=%q qn=%q", symbol.Status, symbol.Label, symbol.QualifiedName)
	}
	if !strings.Contains(symbol.Code, "export function foo") {
		t.Fatalf("expected function body, got %q", symbol.Code)
	}

	byPath, err := store.Snippet(ctx, api.SnippetRequest{Project: "fixture", QualifiedName: "src/foo.ts"})
	if err != nil {
		t.Fatal(err)
	}
	if byPath.Label != "File" {
		t.Fatalf("path snippet label=%q qn=%q", byPath.Label, byPath.QualifiedName)
	}
	if byPath.Location.EndLine <= 1 {
		t.Fatalf("file snippet must not be L1-only: %#v", byPath.Location)
	}

	help := format.HelpForSnippet(symbol)
	joined := strings.Join(help, "\n")
	if strings.Contains(joined, "--direction both") {
		t.Fatalf("AXI help must not default to both: %v", help)
	}
	if !strings.Contains(joined, "incoming") || !strings.Contains(joined, "outgoing") {
		t.Fatalf("AXI help should suggest incoming and outgoing: %v", help)
	}
}

func TestSnippetFileNodeWithMissingEndLineReadsDisk(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	lines := make([]string, 75)
	for i := range lines {
		lines[i] = fmt.Sprintf("// line %d", i+1)
	}
	lines[22] = "export function main() { return null }"
	body := strings.Join(lines, "\n") + "\n"
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "app.ts"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/graph.db"
	store, err := OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{
			Name: "fixture", RootPath: root, Generation: "one",
			EngineVersion: "test", IndexedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		_, err := builder.PutNode(api.Node{Project: "fixture", Label: "File", Name: "app.ts", QualifiedName: "src/app.ts.__file__", Location: api.Location{File: "src/app.ts", StartLine: 1, EndLine: 1}})
		return err
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

	got, err := store.Snippet(ctx, api.SnippetRequest{Project: "fixture", QualifiedName: "src/app.ts"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Location.EndLine <= 1 {
		t.Fatalf("stale File end_line=1 must still snippet from disk, got %#v code=%q", got.Location, got.Code)
	}
	if !strings.Contains(got.Code, "export function main") {
		t.Fatalf("expected file body, got %q", got.Code)
	}
}

func TestSnippetClipsWideFileRange(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	var b strings.Builder
	for i := 1; i <= 600; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	if err := os.WriteFile(filepath.Join(root, "big.ts"), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/graph.db"
	store, err := OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{
			Name: "fixture", RootPath: root, Generation: "one",
			EngineVersion: "test", IndexedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		_, err := builder.PutNode(api.Node{Project: "fixture", Label: "File", Name: "big.ts", QualifiedName: "big.ts.__file__", Location: api.Location{File: "big.ts", StartLine: 1, EndLine: 600}})
		return err
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

	got, err := store.Snippet(ctx, api.SnippetRequest{Project: "fixture", QualifiedName: "big.ts"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Clipped {
		t.Fatalf("expected clipped file snippet, lines=%d-%d code lines=%d", got.Location.StartLine, got.Location.EndLine, strings.Count(got.Code, "\n")+1)
	}
	if strings.Count(got.Code, "\n") > 501 {
		t.Fatalf("clipped snippet still too large: %d newlines", strings.Count(got.Code, "\n"))
	}
	compact := format.SnippetCompact(got)
	if !strings.Contains(compact, "clipped: true") {
		t.Fatalf("compact missing clipped flag: %q", compact)
	}
}

func TestSnippetOneLineSymbolNotClipped(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	var b strings.Builder
	for i := 1; i <= 20; i++ {
		if i == 3 {
			fmt.Fprintf(&b, "export const FOO = 1\n")
			continue
		}
		fmt.Fprintf(&b, "// line %d\n", i)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "app.ts"), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/graph.db"
	store, err := OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Build(ctx, func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{
			Name: "fixture", RootPath: root, Generation: "one",
			EngineVersion: "test", IndexedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		_, err := builder.PutNode(api.Node{
			Project: "fixture", Label: "Variable", Name: "FOO", QualifiedName: "src.app.FOO",
			Location: api.Location{File: "src/app.ts", StartLine: 3, EndLine: 3},
		})
		return err
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
	got, err := store.Snippet(ctx, api.SnippetRequest{Project: "fixture", QualifiedName: "src.app.FOO"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Clipped {
		t.Fatalf("one-line symbol must not be clipped: %#v code=%q", got.Location, got.Code)
	}
	if !strings.Contains(got.Code, "export const FOO") {
		t.Fatalf("expected symbol body, got %q", got.Code)
	}
}

func TestQuerySkipsDataLanguageVariablesAndTraceBothSkipsJSONKeys(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		fn, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "loadConfig", QualifiedName: "src.app.loadConfig", Location: api.Location{File: "src/app.ts", StartLine: 10, EndLine: 40}})
		if err != nil {
			return err
		}
		file, err := builder.PutNode(api.Node{Project: "fixture", Label: "File", Name: "config.json", QualifiedName: "config.json.__file__", Location: api.Location{File: "config.json", StartLine: 1, EndLine: 12}})
		if err != nil {
			return err
		}
		title, err := builder.PutNode(api.Node{Project: "fixture", Label: "Variable", Name: "title", QualifiedName: "config.json.title", Location: api.Location{File: "config.json", StartLine: 2}})
		if err != nil {
			return err
		}
		foo, err := builder.PutNode(api.Node{Project: "fixture", Label: "Variable", Name: "FOO", QualifiedName: "src.app.FOO", Location: api.Location{File: "src/app.ts", StartLine: 3, EndLine: 8}})
		if err != nil {
			return err
		}
		if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: file, TargetID: title, Type: "DEFINES"}); err != nil {
			return err
		}
		if _, err := builder.PutEdge(api.Edge{Project: "fixture", SourceID: fn, TargetID: title, Type: "DEFINES"}); err != nil {
			return err
		}
		_, err = builder.PutEdge(api.Edge{Project: "fixture", SourceID: fn, TargetID: foo, Type: "DEFINES"})
		return err
	})
	defer store.Close()

	result, err := store.Query(ctx, api.QueryRequest{Project: "fixture", Question: "how does config load", Budget: 2000, Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(result.Text, "\n") {
		if strings.HasPrefix(line, "NODE ") && strings.Contains(line, "title") && strings.Contains(line, "config.json") {
			t.Fatalf("query listed JSON key NODE: %q", result.Text)
		}
	}

	traced, err := store.Trace(ctx, api.TraceRequest{Project: "fixture", Start: "loadConfig", Direction: "both", Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	compact := format.TraceCompact(traced)
	if strings.Contains(compact, "title") {
		t.Fatalf("trace both listed JSON key as neighbor: %q", compact)
	}
}

func TestSearchFindsExportedSourceConstNotJSONKey(t *testing.T) {
	ctx := context.Background()
	store := fixtureGraph(t, func(builder *Builder) error {
		if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "Variable", Name: "FOO", QualifiedName: "src.app.FOO", Location: api.Location{File: "src/app.ts", StartLine: 3, EndLine: 8}}); err != nil {
			return err
		}
		if _, err := builder.PutNode(api.Node{Project: "fixture", Label: "Variable", Name: "title", QualifiedName: "config.json.title", Location: api.Location{File: "config.json", StartLine: 2}}); err != nil {
			return err
		}
		return nil
	})
	defer store.Close()

	foo, err := store.Search(ctx, api.SearchRequest{Project: "fixture", Query: "FOO", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range foo.Matches {
		if m.QualifiedName == "src.app.FOO" {
			found = true
		}
		if m.Location.File == "config.json" {
			t.Fatalf("JSON variable leaked into FTS: %#v", m)
		}
	}
	if !found {
		t.Fatalf("exported source const FOO not searchable: %#v", foo.Matches)
	}
	title, err := store.Search(ctx, api.SearchRequest{Project: "fixture", Query: "title", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range title.Matches {
		if m.QualifiedName == "config.json.title" || (m.Label == "Variable" && m.Location.File == "config.json") {
			t.Fatalf("JSON key title was an FTS hit: %#v", m)
		}
	}
}
