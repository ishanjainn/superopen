package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestCrossLanguageRelationshipsUseUnifiedGoPackageIdentity(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	runtime := testSyntaxGrammarRuntime(t)
	root := t.TempDir()
	files := map[string]string{
		"internal/session/store.go": "package session\nfunc path() string { return \"\" }\n",
		"internal/sync/error.go":    "package sync\ntype GraphContinuationError struct{}\nfunc (*GraphContinuationError) Error() string { return \"bad\" }\n",
		"web/app.ts":                "import { join } from \"path\";\nexport function value() { throw new Error(\"bad\"); }\n",
	}
	var paths []string
	for path, body := range files {
		abs := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	repository, err := ParseSyntaxRepository(ctx, runtime, root, "fixture", paths, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	wantEdge := func(edges []pendingEdge, target string) bool {
		for _, edge := range edges {
			if edge.source == "web.app.ts.__file__" && edge.kind == "IMPORTS" && edge.target == target {
				return true
			}
		}
		return false
	}
	if !wantEdge(graph.edges, "internal.session.path") {
		t.Fatalf("assembled cross-language import missing: %#v", graph.edges)
	}
	if !hasPendingEdge(graph.edges, "web.app.value", "RAISES", "internal.sync.Error") {
		t.Fatalf("assembled cross-language raise missing: %#v", graph.edges)
	}
}

func hasPendingEdge(edges []pendingEdge, source, kind, target string) bool {
	for _, edge := range edges {
		if edge.source == source && edge.kind == kind && edge.target == target {
			return true
		}
	}
	return false
}

func TestAssembleSyntaxGraphUsesPinnedRegistryResolution(t *testing.T) {
	repository := SyntaxRepository{
		Generation: "generation",
		Coverage:   api.Coverage{Generation: "generation", IndexMode: "tree-sitter-wasm"},
		Files: []ParsedSyntaxFile{
			{File: FileRecord{Project: "fixture", Path: "src/main.py", Language: "python"}, Extraction: SyntaxExtraction{
				Definitions: []SyntaxFact{{Kind: "function", Name: "main", StartLine: 1, IsEntryPoint: true}, {Kind: "function", Name: "helper", StartLine: 4}},
				Calls:       []SyntaxFact{{Kind: "call", Name: "helper", Scope: "main", StartLine: 2}, {Kind: "call", Name: "dynamic.run", Scope: "main", StartLine: 3}},
				Imports:     []SyntaxFact{{Kind: "import", Name: "json", StartLine: 1}},
				Decorators:  []SyntaxFact{{Kind: "decorator", Name: "cache", Scope: "main", StartLine: 1}},
			}},
			{File: FileRecord{Project: "fixture", Path: "other.py", Language: "python"}, Extraction: SyntaxExtraction{
				Definitions: []SyntaxFact{{Kind: "function", Name: "duplicate", StartLine: 1}},
			}},
			{File: FileRecord{Project: "fixture", Path: "third.py", Language: "python"}, Extraction: SyntaxExtraction{
				Definitions: []SyntaxFact{{Kind: "function", Name: "duplicate", StartLine: 1}},
			}},
			{File: FileRecord{Project: "fixture", Path: "caller.py", Language: "python"}, Extraction: SyntaxExtraction{
				Calls: []SyntaxFact{{Kind: "call", Name: "duplicate", Scope: "caller", StartLine: 3}},
			}},
		},
	}
	graph, coverage := AssembleSyntaxGraph(repository, "fixture")
	if hasNode(graph.nodes, "external:module:json") {
		t.Fatalf("unexpected external import node: %#v", graph.nodes)
	}
	wantUnresolved := false
	for _, edge := range graph.unresolved {
		if edge.source == "src.main.main" && edge.kind == "CALL_REFERENCE" && edge.target == "dynamic.run" {
			wantUnresolved = true
		}
	}
	if !wantUnresolved {
		t.Fatalf("missing unresolved call reference: %#v", graph.unresolved)
	}
	wantEdges := map[string]bool{
		"src.main.main\x00CALLS\x00src.main.helper":       false,
		"src.main.main\x00DECORATES\x00<decorator:cache>": false,
	}
	for _, edge := range graph.edges {
		key := edge.source + "\x00" + edge.kind + "\x00" + edge.target
		if _, ok := wantEdges[key]; ok {
			wantEdges[key] = true
		}
	}
	for edge, found := range wantEdges {
		if !found {
			t.Errorf("missing edge %q in %#v", edge, graph.edges)
		}
	}
	if len(coverage.Rows) != 0 {
		t.Fatalf("coverage = %#v", coverage.Rows)
	}
	resolvedDuplicate := false
	for _, edge := range graph.edges {
		if edge.source == "caller.py.__file__" && edge.kind == "CALLS" && edge.target == "other.duplicate" && edge.evidence != nil {
			resolvedDuplicate = edge.evidence.Strategy == "suffix_match"
		}
	}
	if !resolvedDuplicate {
		t.Fatalf("pinned suffix resolution missing: %#v", graph.edges)
	}
}

func TestAssembleSyntaxGraphUsesPinnedQualifiedNameCollisionBehavior(t *testing.T) {
	repository := SyntaxRepository{Files: []ParsedSyntaxFile{{
		File: FileRecord{Project: "fixture", Path: "duplicate.py", Language: "python"},
		Extraction: SyntaxExtraction{Definitions: []SyntaxFact{
			{Kind: "function", Name: "same", StartLine: 1, StartColumn: 1},
			{Kind: "function", Name: "same", StartLine: 4, StartColumn: 1},
		}},
	}}}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	if !hasNode(graph.nodes, "duplicate.same") || len(nodesWithQN(graph.nodes, "duplicate.same")) != 1 {
		t.Fatalf("duplicate qualified names were not collapsed: %#v", graph.nodes)
	}
}

func TestAssembleSyntaxGraphLinksMethodsToOwningType(t *testing.T) {
	repository := SyntaxRepository{Files: []ParsedSyntaxFile{{
		File: FileRecord{Project: "fixture", Path: "client.py", Language: "python"},
		Extraction: SyntaxExtraction{Definitions: []SyntaxFact{
			{Kind: "class", Name: "Client", StartLine: 1},
			{Kind: "function", Name: "get", Scope: "Client", StartLine: 2},
		}},
	}}}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	for _, edge := range graph.edges {
		if edge.source == "client.Client" && edge.kind == "DEFINES_METHOD" && edge.target == "client.Client.get" {
			return
		}
	}
	t.Fatalf("missing method ownership edge: %#v", graph.edges)
}

func TestSortGraphPreservesImportSpecifiersOnly(t *testing.T) {
	graph := goGraph{edges: []pendingEdge{
		{source: "file", target: "module", kind: "IMPORTS", properties: api.Properties{"local_name": "One"}},
		{source: "file", target: "module", kind: "IMPORTS", properties: api.Properties{"local_name": "Two"}},
		{source: "caller", target: "callee", kind: "CALLS", properties: api.Properties{"local_name": "one"}},
		{source: "caller", target: "callee", kind: "CALLS", properties: api.Properties{"local_name": "two"}},
	}}
	sortGraph(&graph)
	if len(graph.edges) != 3 {
		t.Fatalf("edges=%#v, want two imports and one call", graph.edges)
	}
}

func TestLocalSyntaxImportTargetPrefersExactFolderOverIndexModule(t *testing.T) {
	nodes := []api.Node{
		{Label: "Folder", QualifiedName: "web.src.components.session-rail", Location: api.Location{File: "web/src/components/session-rail"}},
		{Label: "Module", QualifiedName: "web.src.components.session-rail.index", Location: api.Location{File: "web/src/components/session-rail/index.ts"}},
	}
	if got := localSyntaxImportTarget("web/src/app.ts", "./components/session-rail", nodes); got != "web.src.components.session-rail" {
		t.Fatalf("target=%q, want folder", got)
	}
	if got := localSyntaxImportTarget("web/src/app.ts", "@/components/session-rail", nodes); got != "web.src.components.session-rail" {
		t.Fatalf("aliased target=%q, want folder", got)
	}
}

func TestLocalSyntaxImportTargetFallsBackToPinnedSymbolLookup(t *testing.T) {
	nodes := []api.Node{
		{Label: "Function", Name: "path", QualifiedName: "z.path", Location: api.Location{File: "z.go"}},
		{Label: "Function", Name: "path", QualifiedName: "internal.coding.sessionstate.path", Location: api.Location{File: "internal/agent/sessionstate/store.go"}},
	}
	if got := localSyntaxImportTarget("web/src/app.ts", "node:path", nodes); got != "internal.coding.sessionstate.path" {
		t.Fatalf("target=%q, want lexicographically first symbol", got)
	}
}

func TestLocalSyntaxImportTargetAllowsSymbolInSourceFile(t *testing.T) {
	nodes := []api.Node{
		{Label: "File", Name: "install.sh", QualifiedName: "scripts.install.sh.__file__", Location: api.Location{File: "scripts/install.sh"}},
		{Label: "Function", Name: "need", QualifiedName: "scripts.install.need", Location: api.Location{File: "scripts/install.sh"}},
	}
	if got := localSyntaxImportTarget("scripts/install.sh", "need", nodes); got != "scripts.install.need" {
		t.Fatalf("target=%q, want same-file symbol", got)
	}
}

func TestReplaceGenericGoGraphPreservesCrossLanguageEdgesToSurvivingSymbols(t *testing.T) {
	generic := goGraph{
		nodes: []api.Node{
			{Label: "File", QualifiedName: "web.app.__file__", Location: api.Location{File: "web/app.ts"}},
			{Label: "Function", QualifiedName: "internal.session.path", Location: api.Location{File: "internal/session/store.go"}},
		},
		edges: []pendingEdge{{source: "web.app.__file__", target: "internal.session.path", kind: "IMPORTS"}},
	}
	resolved := goGraph{nodes: []api.Node{
		{Label: "Function", QualifiedName: "internal.session.path", Location: api.Location{File: "internal/session/store.go"}},
	}}
	got := replaceGenericGoGraph(generic, resolved, map[string]bool{"internal/session/store.go": true})
	if len(got.edges) != 1 || got.edges[0].target != "internal.session.path" {
		t.Fatalf("cross-language edge was discarded: %#v", got.edges)
	}
}

func nodesWithQN(nodes []api.Node, qn string) []api.Node {
	result := []api.Node{}
	for _, node := range nodes {
		if node.QualifiedName == qn {
			result = append(result, node)
		}
	}
	return result
}

func TestAssembleSyntaxGraphUsesPinnedModuleAndTypeIdentities(t *testing.T) {
	repository := SyntaxRepository{Files: []ParsedSyntaxFile{{
		File: FileRecord{Project: "fixture", Path: "src/types.ts", Language: "typescript"},
		Extraction: SyntaxExtraction{RootModule: true, Definitions: []SyntaxFact{
			{Kind: "class", NodeType: "interface_declaration", Name: "Shape", StartLine: 1},
			{Kind: "class", NodeType: "type_alias_declaration", Name: "ID", StartLine: 2},
		}},
	}}}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	want := map[string]string{
		"src.types":       "Module",
		"src.types.Shape": "Interface",
		"src.types.ID":    "Type",
	}
	for _, node := range graph.nodes {
		if label, ok := want[node.QualifiedName]; ok {
			if node.Label != label {
				t.Errorf("%s label=%s, want %s", node.QualifiedName, node.Label, label)
			}
			if node.Label == "Module" && node.Name != "src/types.ts" {
				t.Errorf("%s name=%s, want source path", node.QualifiedName, node.Name)
			}
			delete(want, node.QualifiedName)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing nodes: %#v", want)
	}
}

func TestAssembleCompactUsagesPreserveEdgeHistogram(t *testing.T) {
	repository := SyntaxRepository{Files: []ParsedSyntaxFile{{
		File: FileRecord{Project: "fixture", Path: "app.py", Language: "python"},
		Extraction: SyntaxExtraction{
			Definitions: []SyntaxFact{
				{Kind: "function", Name: "use", StartLine: 1},
				{Kind: "function", Name: "helper", StartLine: 8},
				{Kind: "variable", Name: "config", StartLine: 10},
			},
			Calls: []SyntaxFact{{Kind: "call", Name: "helper", Scope: "use", StartLine: 2}},
			Usages: []OccurrenceFact{
				{Name: "helper", Scope: "use", StartLine: 3, Confidence: 0.7},
				{Name: "config", Scope: "use", StartLine: 4, Confidence: 0.7},
			},
			Writes:  []OccurrenceFact{{Name: "config", Scope: "use", StartLine: 5, Confidence: 1}},
			Imports: []SyntaxFact{{Kind: "import", Name: "json", LocalName: "json", StartLine: 1}},
		},
	}}}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	counts := map[string]int{}
	for _, edge := range graph.edges {
		counts[edge.kind]++
	}
	if counts["CALLS"] < 1 {
		t.Fatalf("CALLS missing: %#v %#v", counts, graph.edges)
	}
	if counts["USAGE"] < 1 {
		t.Fatalf("USAGE missing: %#v %#v", counts, graph.edges)
	}
	if counts["WRITES"] < 1 {
		t.Fatalf("WRITES missing: %#v %#v", counts, graph.edges)
	}
	found := false
	for _, edge := range graph.edges {
		if edge.kind == "USAGE" && edge.Callee() != "" && edge.evidence != nil && edge.evidence.Location != nil && edge.evidence.Location.StartLine > 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("USAGE missing callee/evidence: %#v", graph.edges)
	}
}
