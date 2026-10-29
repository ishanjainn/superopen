package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestNameGuessesDoNotCrossLanguages(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	runtime := testSyntaxGrammarRuntime(t)
	root := t.TempDir()
	files := map[string]string{
		"internal/session/store.go": "package session\nfunc path() string { return \"\" }\n",
		"internal/sync/error.go":    "package sync\ntype Error struct{}\ntype GraphContinuationError struct{}\nfunc (*GraphContinuationError) Message() string { return \"bad\" }\n",
		"web/app.ts":                "import { join } from \"path\";\nexport function value() { Message(); throw new Error(\"bad\"); }\n",
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
	// Node's path module, the JavaScript Error builtin and a local Message()
	// are not the Go symbols that share those names.
	for _, edge := range graph.edges {
		if strings.HasPrefix(edge.source, "web.") && strings.HasPrefix(edge.target, "internal.") {
			t.Fatalf("TypeScript resolved into Go by name: %#v", edge)
		}
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

func TestPythonDecoratorEdgesResolveRealSource(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{
		"pkg/fields.py":  "class Field:\n    def register_lookup(cls, value):\n        return value\n\nclass cached_property:\n    pass\n",
		"pkg/lookups.py": "from pkg.fields import Field, cached_property\n\n@Field.register_lookup\nclass Exact:\n    @cached_property\n    def value(self):\n        return 1\n",
	} {
		abs := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", []string{"pkg/fields.py", "pkg/lookups.py"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	if !hasPendingEdge(graph.edges, "pkg.lookups.Exact", "DECORATES", "pkg.fields.Field.register_lookup") ||
		!hasPendingEdge(graph.edges, "pkg.lookups.Exact.value", "DECORATES", "pkg.fields.cached_property") {
		t.Fatalf("definitions = %#v; decorators = %#v; edges = %#v", repository.Files[1].Extraction.Definitions, repository.Files[1].Extraction.Decorators, graph.edges)
	}
	if !hasPendingEdge(graph.edges, "pkg.lookups.Exact", "USAGE", "pkg.fields.Field.register_lookup") ||
		!hasPendingEdge(graph.edges, "pkg.lookups.Exact.value", "USAGE", "pkg.fields.cached_property") {
		t.Fatalf("decorator reference links missing: %#v", graph.edges)
	}
}

func TestStructuredDataDefinitionsHaveSourceNames(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"settings.toml": "[tool.pylint]\nvalue = 1\n[tool.pylint.\"messages control\"]\ndisable = \"all\"\n",
		"template.xml":  "<root>\n  <child value=\"yes\"/>\n</root>\n",
	}
	var paths []string
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, name)
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", paths, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	for _, want := range []string{"settings.tool.pylint", "settings.tool.pylint.\"messages-control\"", "template.root", "template.child"} {
		if !hasNode(graph.nodes, want) {
			t.Fatalf("missing %s, nodes = %#v", want, graph.nodes)
		}
	}
}

func TestTextDocumentationAndPythonTemplateAreIndexed(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "conf.py"), []byte("source_suffix = '.txt'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"contents.txt":  "Documentation\n=============\n\nIntro\n-----\n",
		"manage.py-tpl": "#!/usr/bin/env python\ndef main():\n    return 1\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", []string{"contents.txt", "manage.py-tpl"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	if !hasNode(graph.nodes, "manage.main") {
		t.Fatalf("Python template function missing: %#v", graph.nodes)
	}
	var sections int
	for _, node := range graph.nodes {
		if node.Label == "Section" && node.Location.File == "contents.txt" {
			sections++
		}
	}
	if sections == 0 {
		t.Fatalf("text documentation sections missing: %#v", graph.nodes)
	}
}

func TestPythonLocalConstructorCallsLinkToProductionAndTests(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	files := map[string]string{
		"pkg/backend.py":        "class Backend:\n    def open(self):\n        return True\n",
		"tests/test_backend.py": "from pkg import backend as smtp\n\ndef test_open():\n    backend = smtp.Backend()\n    backend.open()\n\ndef test_reassigned():\n    backend = smtp.Backend()\n    backend = None\n    backend.open()\n\ndef test_nested():\n    class LocalBackend(smtp.Backend):\n        pass\n    backend = LocalBackend()\n    backend.open()\n\ndef test_helper():\n    def invoke():\n        backend = smtp.Backend()\n        backend.open()\n    invoke()\n\nclass FieldTests:\n    @classmethod\n    def setUpClass(cls):\n        cls.backend = smtp.Backend()\n\n    def test_field(self):\n        self.backend.open()\n",
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
	repository, err := ParseSyntaxRepository(ctx, testSyntaxGrammarRuntime(t), root, "fixture", paths, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	appendTestRelationships(&graph)
	method := "pkg.backend.Backend.open"
	if !hasPendingEdge(graph.edges, "tests.test_backend.test_open", "CALLS", method) ||
		!hasPendingEdge(graph.edges, "tests.test_backend.test_open", "TESTS", method) {
		t.Fatalf("local constructor call did not create call and test links: %#v", graph.edges)
	}
	if hasPendingEdge(graph.edges, "tests.test_backend.test_reassigned", "CALLS", method) ||
		hasPendingEdge(graph.edges, "tests.test_backend.test_reassigned", "TESTS", method) {
		t.Fatalf("reassigned local kept a stale type: %#v", graph.edges)
	}
	for _, test := range []string{"tests.test_backend.test_nested", "tests.test_backend.FieldTests.test_field"} {
		if !hasPendingEdge(graph.edges, test, "CALLS", method) || !hasPendingEdge(graph.edges, test, "TESTS", method) {
			t.Fatalf("inherited or setup-bound call did not link from %s: %#v", test, graph.edges)
		}
	}
	if !hasNode(graph.nodes, "tests.test_backend.test_helper.invoke") ||
		!hasPendingEdge(graph.edges, "tests.test_backend.test_helper.invoke", "CALLS", method) ||
		!hasPendingEdge(graph.edges, "tests.test_backend.test_helper", "TESTS", method) {
		t.Fatalf("nested test helper should remain a callable and count toward its test: %#v", graph.edges)
	}
}

func TestAssembleSyntaxGraphUsesPinnedRegistryResolution(t *testing.T) {
	repository := SyntaxRepository{
		Generation: "generation",
		Coverage:   api.Coverage{Generation: "generation", IndexMode: "tree-sitter-wasm"},
		Files: []ParsedSyntaxFile{
			{File: FileRecord{Project: "fixture", Path: "src/main.py", Language: "python"}, Extraction: FileResult{
				Definitions: []SyntaxFact{{Kind: "function", Name: "main", StartLine: 1, IsEntryPoint: true}, {Kind: "function", Name: "helper", StartLine: 4}},
				Calls:       []SyntaxFact{{Kind: "call", Name: "helper", Scope: "main", StartLine: 2}, {Kind: "call", Name: "dynamic.run", Scope: "main", StartLine: 3}},
				Imports:     []SyntaxFact{{Kind: "import", Name: "json", StartLine: 1}},
				Decorators:  []SyntaxFact{{Kind: "decorator", Name: "cache", Scope: "main", StartLine: 1}},
			}},
			{File: FileRecord{Project: "fixture", Path: "other.py", Language: "python"}, Extraction: FileResult{
				Definitions: []SyntaxFact{{Kind: "function", Name: "duplicate", StartLine: 1}},
			}},
			{File: FileRecord{Project: "fixture", Path: "third.py", Language: "python"}, Extraction: FileResult{
				Definitions: []SyntaxFact{{Kind: "function", Name: "duplicate", StartLine: 1}},
			}},
			{File: FileRecord{Project: "fixture", Path: "caller.py", Language: "python"}, Extraction: FileResult{
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

func TestPythonNestedCallableArgumentCreatesCallReference(t *testing.T) {
	repository := SyntaxRepository{Generation: "nested", Files: []ParsedSyntaxFile{{File: FileRecord{Project: "fixture", Path: "pkg/mod.py", Language: "python"}, Extraction: FileResult{
		Definitions: []SyntaxFact{{Kind: "function", Name: "outer", StartLine: 1}, {Kind: "function", Name: "predicate", Scope: "outer", StartLine: 2}},
		Usages:      []OccurrenceFact{{Name: "predicate", Scope: "outer", StartByte: 100, StartLine: 4, MayBeCallReference: true}},
	}}}}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	if !hasPendingEdge(graph.edges, "pkg.mod.outer", "CALL_REFERENCE", "pkg.mod.outer.predicate") {
		t.Fatalf("nested callable reference: %+v", graph.edges)
	}
}

func TestPythonNestedCallableReferenceExtractsFromSource(t *testing.T) {
	root := t.TempDir()
	code := "def outer(values):\n    def predicate(item):\n        return item\n    result = list(map(predicate, values))\n    alias = predicate\n    return result, alias\n\ndef wrapped(values):\n    def predicate(item):\n        return item\n    return list(map((predicate), values))\n"
	if err := os.WriteFile(filepath.Join(root, "module.py"), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", []string{"module.py"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	if !hasPendingEdge(graph.edges, "module.outer", "CALL_REFERENCE", "module.outer.predicate") {
		t.Fatalf("source callable reference: %+v", graph.edges)
	}
	if !hasPendingEdge(graph.edges, "module.wrapped", "CALL_REFERENCE", "module.wrapped.predicate") {
		t.Fatalf("parenthesized callable reference: %+v", graph.edges)
	}
}

func TestPythonImportedCallableArgumentCreatesCallReference(t *testing.T) {
	root := t.TempDir()
	for path, body := range map[string]string{
		"pkg/callbacks.py": "def handler(value):\n    return value\n",
		"pkg/consumer.py":  "from pkg.callbacks import handler\n\ndef run(values):\n    return list(map(handler, values))\n",
	} {
		abs := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", []string{"pkg/callbacks.py", "pkg/consumer.py"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	if !hasPendingEdge(graph.edges, "pkg.consumer.run", "CALL_REFERENCE", "pkg.callbacks.handler") {
		t.Fatalf("imported callable reference: %+v", graph.edges)
	}
}

func TestPythonBoundMethodArgumentCreatesCallReference(t *testing.T) {
	root := t.TempDir()
	code := "class Worker:\n    def process(self, value):\n        return value\n\n    def run(self, values):\n        return list(map(self.process, values))\n\ndef outside(values):\n    worker = Worker()\n    return list(map(worker.process, values))\n"
	if err := os.WriteFile(filepath.Join(root, "worker.py"), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", []string{"worker.py"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	for _, source := range []string{"worker.Worker.run", "worker.outside"} {
		if !hasPendingEdge(graph.edges, source, "CALL_REFERENCE", "worker.Worker.process") {
			t.Fatalf("bound method reference from %s: %+v", source, graph.edges)
		}
	}
}

func TestPythonRegistryDictionaryLinksCallableValues(t *testing.T) {
	root := t.TempDir()
	code := "def handler(value):\n    return value\n\nHANDLERS = {'item': handler}\n\nclass Registry:\n    def process(self, value):\n        return value\n    CHECKERS = {'item': process}\n\nCONSTRUCTORS = {'registry': Registry}\n"
	if err := os.WriteFile(filepath.Join(root, "registry.py"), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", []string{"registry.py"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	if !hasPendingEdge(graph.edges, "registry", "CALL_REFERENCE", "registry.handler") ||
		!hasPendingEdge(graph.edges, "registry", "CALL_REFERENCE", "registry.Registry.process") {
		t.Fatalf("registry callback values: %+v", graph.edges)
	}
	if hasPendingEdge(graph.edges, "registry", "CALL_REFERENCE", "registry.Registry") {
		t.Fatalf("class constructor treated as function value: %+v", graph.edges)
	}
}

func TestPythonDecoratedFunctionValueRemainsUsage(t *testing.T) {
	root := t.TempDir()
	code := "def passthrough(fn):\n    return fn\n\n@passthrough\ndef decorated(value):\n    return value\n\ndef run(values):\n    return list(map(decorated, values))\n"
	if err := os.WriteFile(filepath.Join(root, "decorated.py"), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", []string{"decorated.py"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	if hasPendingEdge(graph.edges, "decorated.run", "CALL_REFERENCE", "decorated.decorated") ||
		!hasPendingEdge(graph.edges, "decorated.run", "USAGE", "decorated.decorated") {
		t.Fatalf("decorated value proof: %+v", graph.edges)
	}
}

func TestAssembleSyntaxGraphUsesPinnedQualifiedNameCollisionBehavior(t *testing.T) {
	repository := SyntaxRepository{Files: []ParsedSyntaxFile{{
		File: FileRecord{Project: "fixture", Path: "duplicate.py", Language: "python"},
		Extraction: FileResult{Definitions: []SyntaxFact{
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
		Extraction: FileResult{Definitions: []SyntaxFact{
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

func TestLocalSyntaxImportTargetFallsBackToSymbolLookupInFamily(t *testing.T) {
	nodes := []api.Node{
		{Label: "Function", Name: "path", QualifiedName: "a.path", Location: api.Location{File: "a/store.go"}},
		{Label: "Function", Name: "path", QualifiedName: "web.util.path", Location: api.Location{File: "web/util.ts"}},
		{Label: "Function", Name: "path", QualifiedName: "web.z.path", Location: api.Location{File: "web/z.ts"}},
	}
	if got := localSyntaxImportTarget("web/src/app.ts", "node:path", nodes); got != "web.util.path" {
		t.Fatalf("target=%q, want the lexicographically first TypeScript symbol", got)
	}
	if got := localSyntaxImportTarget("web/src/app.ts", "node:path", nodes[:1]); got != "" {
		t.Fatalf("a TypeScript import bound a Go function: %q", got)
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
		Extraction: FileResult{RootModule: true, Definitions: []SyntaxFact{
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
		Extraction: FileResult{
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

func TestAssembleSyntaxGraphSuppressesWeakJSDottedCalls(t *testing.T) {
	repository := SyntaxRepository{Files: []ParsedSyntaxFile{
		{
			File: FileRecord{Project: "fixture", Path: "src/unrelated.ts", Language: "typescript"},
			Extraction: FileResult{Definitions: []SyntaxFact{
				{Kind: "class", Name: "Unrelated", StartLine: 1},
				{Kind: "function", Name: "test", Scope: "Unrelated", StartLine: 2},
			}},
		},
		{
			File: FileRecord{Project: "fixture", Path: "src/caller.ts", Language: "typescript"},
			Extraction: FileResult{
				Definitions: []SyntaxFact{{Kind: "function", Name: "run", StartLine: 1}},
				Calls:       []SyntaxFact{{Kind: "call", Name: "re.test", Scope: "run", StartLine: 2}},
			},
		},
		{
			File: FileRecord{Project: "fixture", Path: "src/lib.ts", Language: "typescript"},
			Extraction: FileResult{
				RootModule:  true,
				Definitions: []SyntaxFact{{Kind: "function", Name: "helper", StartLine: 1}},
			},
		},
		{
			File: FileRecord{Project: "fixture", Path: "src/mapped.ts", Language: "typescript"},
			Extraction: FileResult{
				Definitions: []SyntaxFact{{Kind: "function", Name: "useLib", StartLine: 1}},
				Imports:     []SyntaxFact{{Kind: "import", Name: "./lib", LocalName: "lib", StartLine: 1}},
				Calls:       []SyntaxFact{{Kind: "call", Name: "lib.helper", Scope: "useLib", StartLine: 2}},
			},
		},
	}}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	for _, edge := range graph.edges {
		if edge.kind == "CALLS" && strings.Contains(edge.target, "test") && strings.Contains(edge.source, "caller") {
			t.Fatalf("weak JS dotted call must not CALLS: %#v", edge)
		}
	}
	unresolved := false
	for _, edge := range graph.unresolved {
		if edge.kind == "CALL_REFERENCE" && edge.target == "re.test" {
			unresolved = true
		}
	}
	if !unresolved {
		t.Fatalf("expected unresolved CALL_REFERENCE for re.test: %#v", graph.unresolved)
	}
	mapped := false
	for _, edge := range graph.edges {
		if edge.kind == "CALLS" && strings.Contains(edge.source, "useLib") && strings.Contains(edge.target, "helper") {
			if edge.evidence != nil && (edge.evidence.Strategy == "import_map" || edge.evidence.Strategy == "import_map_suffix") {
				mapped = true
			}
		}
	}
	if !mapped {
		t.Fatalf("import_map CALLS missing: %#v", graph.edges)
	}
}

func TestSuppressWeakJSMethodCall(t *testing.T) {
	if !suppressWeakJSMethodCall("typescript", "re.test", "unique_name") {
		t.Fatal("unique_name dotted JS call should be suppressed")
	}
	if suppressWeakJSMethodCall("typescript", "re.test", "import_map") {
		t.Fatal("import_map must still CALLS")
	}
	if suppressWeakJSMethodCall("go", "re.test", "unique_name") {
		t.Fatal("non-JS must not use this suppress")
	}
	if suppressWeakJSMethodCall("typescript", "helper", "unique_name") {
		t.Fatal("undotted callee is not a weak method bind")
	}
}

func TestAssembleSyntaxGraphFileEndLine(t *testing.T) {
	body := strings.Repeat("line\n", 80)
	repository := SyntaxRepository{
		Files: []ParsedSyntaxFile{{
			File: FileRecord{Project: "fixture", Path: "big.ts", Language: "typescript"},
			Body: []byte(body),
			Extraction: FileResult{
				RootModule:  true,
				Definitions: []SyntaxFact{{Kind: "function", Name: "leaf", StartLine: 20, EndLine: 22}},
			},
		}},
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	var fileEnd, moduleEnd int
	for _, node := range graph.nodes {
		if node.Label == "File" && node.Location.File == "big.ts" {
			fileEnd = node.Location.EndLine
		}
		if node.Label == "Module" && node.Location.File == "big.ts" {
			moduleEnd = node.Location.EndLine
		}
	}
	if fileEnd != 80 {
		t.Fatalf("File end_line=%d want 80", fileEnd)
	}
	if moduleEnd != 80 {
		t.Fatalf("Module end_line=%d want 80", moduleEnd)
	}
}

func TestAssembleSyntaxGraphFileEndLineWithoutBody(t *testing.T) {
	repository := SyntaxRepository{
		Files: []ParsedSyntaxFile{{
			File: FileRecord{Project: "fixture", Path: "src/app.ts", Language: "typescript", LineCount: 75},
			Extraction: FileResult{
				RootModule:  true,
				Definitions: []SyntaxFact{{Kind: "function", Name: "main", StartLine: 20, EndLine: 22}},
			},
		}},
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	var fileEnd int
	for _, node := range graph.nodes {
		if node.Label == "File" && node.Location.File == "src/app.ts" {
			fileEnd = node.Location.EndLine
		}
	}
	if fileEnd != 75 {
		t.Fatalf("File end_line=%d want 75 from LineCount when Body is empty", fileEnd)
	}
}

// A member name is not a free reference, and a Python class's own method is
// what self.name() calls.
func TestPythonMemberNamesResolveThroughTheirReceiver(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	runtime := testSyntaxGrammarRuntime(t)
	root := t.TempDir()
	files := map[string]string{
		"pkg/query.py":     "from pkg.constants import SINGLE\n\nclass Query:\n    def clone(self):\n        return self\n\n    def get_count(self, compiler):\n        obj = self.clone()\n        label = compiler.name\n        for connection in compiler.connections:\n            print(connection)\n        return compiler.get_converters(SINGLE)\n\nclass RawQuery:\n    def clone(self):\n        return self\n",
		"pkg/constants.py": "SINGLE = 'single'\nconnection = None\n",
		"pkg/urls.py":      "def get_converters():\n    return {}\n",
		"pkg/compiler.py":  "class SQLCompiler:\n    def get_converters(self, kind):\n        return []\n",
		"pkg/content.py":   "class Content:\n    @property\n    def name(self):\n        return 'content'\n",
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
	src := "pkg.query.Query.get_count"
	if !hasPendingEdge(graph.edges, src, "CALLS", "pkg.query.Query.clone") {
		t.Fatalf("self.clone() must call Query.clone: %#v", graph.edges)
	}
	if hasPendingEdge(graph.edges, src, "CALLS", "pkg.query.RawQuery.clone") {
		t.Fatal("self.clone() resolved to another class")
	}
	if !hasPendingEdge(graph.edges, src, "USAGE", "pkg.constants.SINGLE") {
		t.Fatalf("imported constant must resolve through the import: %#v", graph.edges)
	}
	for _, edge := range graph.edges {
		if edge.source == src && edge.kind == "USAGE" && edge.target == "pkg.content.Content.name" &&
			(edge.evidence == nil || edge.evidence.Confidence >= traceMinConfidence) {
			t.Fatalf("untyped receiver must not confidently resolve to an unrelated accessor: %#v", edge)
		}
	}
	// A loop variable binds its name; it is not the module-level connection.
	for _, edge := range graph.edges {
		if edge.source == src && edge.target == "pkg.constants.connection" {
			t.Fatalf("loop variable read as a free name: %#v", edge)
		}
	}
	// compiler.get_converters() on a parameter may still be kept as a guess
	// for impact, but never at the confidence trace and query walk.
	for _, edge := range graph.edges {
		if edge.source == src && edge.target == "pkg.urls.get_converters" &&
			(edge.kind == "USAGE" || edge.evidence == nil || edge.evidence.Confidence >= traceMinConfidence) {
			t.Fatalf("member name resolved as a confident or free reference: %#v", edge)
		}
	}
}
