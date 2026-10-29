package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestGoReceiverMethodsAndFieldsRemainDistinct(t *testing.T) {
	root := t.TempDir()
	code := `package sample
type Alpha struct { Name string; count int }
type Beta struct { Name string }
func (a Alpha) Write() {}
func (b *Beta) Write() {}
func (a Alpha) Forward() { a.Write() }
func (b *Beta) Forward() { b.Write() }
func use(a Alpha, b *Beta) { a.Write(); b.Write() }
`
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", []string{"sample.go"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	for _, qn := range []string{"Alpha.Write", "Beta.Write", "Alpha.Forward", "Beta.Forward", "Alpha.Name", "Alpha.count", "Beta.Name"} {
		if !hasNode(graph.nodes, qn) {
			t.Fatalf("missing %s: definitions=%#v", qn, repository.Files[0].Extraction.Definitions)
		}
	}
	for _, qn := range []string{"Alpha.Write", "Beta.Write"} {
		if !hasPendingEdge(graph.edges, "sample.go.__file__", "DEFINES", qn) {
			t.Fatalf("missing method definition edge to %s", qn)
		}
	}
	if !hasPendingEdge(graph.edges, "Alpha.Forward", "CALLS", "Alpha.Write") ||
		!hasPendingEdge(graph.edges, "Beta.Forward", "CALLS", "Beta.Write") {
		t.Fatalf("receiver method call lost its owner: %#v", graph.edges)
	}
}

func TestGoRootImportUsesDeclaredPackageName(t *testing.T) {
	files := []ParsedSyntaxFile{
		{File: FileRecord{Path: "a_test.go", Language: "go"}, Body: []byte("package sample_test\n")},
		{File: FileRecord{Path: "root.go", Language: "go"}, Body: []byte("package sample\n")},
	}
	if got := syntaxGoRootPackage(files); got != "sample" {
		t.Fatalf("root package = %q", got)
	}
	registry := newSymbolRegistry([]api.Node{{Label: "Project", QualifiedName: "fixture"}, {Label: "Function", Name: "NewRouter", QualifiedName: "NewRouter"}})
	resolved := registry.resolveWith(newResolveSession("example", map[string]string{"sample": "fixture"}), "sample.NewRouter")
	if resolved.qn != "NewRouter" || resolved.strategy != "import_map" {
		t.Fatalf("root import resolution = %#v", resolved)
	}
}

func TestGoConstructorResultTypesLocalSelector(t *testing.T) {
	root := t.TempDir()
	code := "package sample\ntype Mux struct{}\nfunc (m *Mux) Get() {}\nfunc NewRouter() *Mux { return &Mux{} }\nfunc run() { r := NewRouter(); r.Get() }\n"
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", []string{"sample.go"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	if !hasPendingEdge(graph.edges, "run", "CALLS", "Mux.Get") {
		t.Fatalf("constructor result selector missing: types=%#v", repository.Files[0].Extraction.TypeAssignments)
	}
}

func TestGoConstructorResultInsideCallback(t *testing.T) {
	root := t.TempDir()
	code := "package sample\ntype Context struct{}\nfunc (c *Context) RoutePattern() string { return \"/\" }\nfunc RouteContext() *Context { return &Context{} }\nfunc run() { register(func() { ctx := RouteContext(); ctx.RoutePattern() }) }\n"
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", []string{"sample.go"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	if !hasPendingEdge(graph.edges, "run", "CALLS", "Context.RoutePattern") {
		t.Fatalf("callback constructor result selector missing: types=%#v", repository.Files[0].Extraction.TypeAssignments)
	}
}

func TestGoTypeAssertionAndFieldChainDispatch(t *testing.T) {
	root := t.TempDir()
	code := "package sample\ntype Child struct{}\nfunc (c *Child) Run() {}\ntype Parent struct { child *Child }\nfunc (p *Parent) Do(value any) { local, _ := value.(*Child); local.Run(); p.child.Run() }\n"
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", []string{"sample.go"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	types := newGoLocalTypes(&repository.Files[0], newSymbolRegistry(graph.nodes), nil)
	for _, callee := range []string{"local.Run", "p.child.Run"} {
		if got := types.resolve(newSymbolRegistry(graph.nodes), "Parent.Do", callee, nil, ^uint32(0)); got.qn != "Child.Run" {
			t.Fatalf("%s resolved to %#v", callee, got)
		}
	}
	if !hasPendingEdge(graph.edges, "Parent.Do", "CALLS", "Child.Run") {
		t.Fatalf("typed selector missing: assignments=%#v", repository.Files[0].Extraction.TypeAssignments)
	}
}

func TestGoCompositeLiteralAndFunctionReturnDispatch(t *testing.T) {
	root := t.TempDir()
	code := "package sample\ntype Resource struct{}\nfunc (Resource) Routes() {}\ntype Chain struct{}\nfunc (Chain) Handler() {}\nfunc NewChain() Chain { return Chain{} }\nfunc run() { Resource{}.Routes(); NewChain().Handler() }\n"
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", []string{"sample.go"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	if !hasPendingEdge(graph.edges, "run", "CALLS", "Resource.Routes") || !hasPendingEdge(graph.edges, "run", "CALLS", "Chain.Handler") {
		t.Fatalf("typed dispatch missing: calls=%#v", repository.Files[0].Extraction.Calls)
	}
}

func TestGoMethodLinksTypeInAnotherFile(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"a_method.go": "package sample\nfunc (s Sample) Run() {}\n",
		"z_type.go":   "package sample\ntype Sample struct{}\n",
	}
	var paths []string
	for path, code := range files {
		if err := os.WriteFile(filepath.Join(root, path), []byte(code), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", paths, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	if !hasPendingEdge(graph.edges, "Sample", "DEFINES_METHOD", "Sample.Run") {
		t.Fatalf("cross-file method ownership missing: %#v", graph.edges)
	}
}

func TestGoWeakSelectorGuardUsesReceiverEvidence(t *testing.T) {
	for _, tc := range []struct {
		callee, strategy string
		confidence       float64
		drop             bool
	}{
		{"file.Close", "suffix_match", .9, true},
		{"file.Close", "field_type_hint", .85, true},
		{"file.Close", "unique_name", .38, true},
		{"router.Get", "unique_name", .75, false},
		{"router.Get", "import_map", .95, false},
		{"Get", "suffix_match", .2, false},
	} {
		if got := goSuppressWeakSelector("go", tc.callee, symbolResolution{strategy: tc.strategy, confidence: tc.confidence}); got != tc.drop {
			t.Errorf("%s %s: drop=%t, want %t", tc.callee, tc.strategy, got, tc.drop)
		}
	}
}
