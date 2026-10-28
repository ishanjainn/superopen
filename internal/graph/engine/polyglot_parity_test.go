package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRustImplAndCppOutOfLineMethods(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"rust/src/lib.rs": "pub struct Worker;\nfn helper() -> i32 { 1 }\nimpl Worker { pub fn run(&self) -> i32 { helper() } }\n",
		"cpp/store.cpp":   "class Store { public: void run(); void save(); };\nvoid Store::run() { save(); }\nvoid Store::save() {}\n",
	}
	var paths []string
	for path, body := range files {
		abs := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	runtime, _, err := loadSelectedGrammarAssets(context.Background(), EngineAssets, "assets/grammars/manifest.json", false, []string{"rust", "cpp"})
	if err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), runtime, root, "fixture", paths, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	labels := map[string]string{}
	for _, node := range graph.nodes {
		labels[node.QualifiedName] = node.Label
	}
	for qn, label := range map[string]string{
		"rust.src.lib.Worker": "Struct", "rust.src.lib.Worker.run": "Method",
		"cpp.store.Store": "Class", "cpp.store.Store.run": "Method", "cpp.store.Store.save": "Method",
	} {
		if labels[qn] != label {
			t.Errorf("%s: got %q, want %q", qn, labels[qn], label)
		}
	}
	if !hasPendingEdge(graph.edges, "rust.src.lib.Worker", "DEFINES_METHOD", "rust.src.lib.Worker.run") ||
		!hasPendingEdge(graph.edges, "rust.src.lib", "USAGE", "rust.src.lib.Worker") ||
		!hasPendingEdge(graph.edges, "rust.src.lib.Worker.run", "CALLS", "rust.src.lib.helper") ||
		!hasPendingEdge(graph.edges, "cpp.store.Store", "DEFINES_METHOD", "cpp.store.Store.run") ||
		!hasPendingEdge(graph.edges, "cpp.store.Store.run", "CALLS", "cpp.store.Store.save") {
		t.Fatalf("method relationships: %+v", graph.edges)
	}
	if labels["cpp.store.Store::run"] != "" {
		t.Fatalf("out-of-line method also emitted as function: %+v", graph.nodes)
	}
}

func TestTypeScriptConstructorBindingResolvesMethodCall(t *testing.T) {
	root := t.TempDir()
	body := []byte("export class Router { handle(input: string): string { return input; } }\nexport function start(): string { const router = new Router(); return router.handle('hi'); }\n")
	if err := os.WriteFile(filepath.Join(root, "router.ts"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	runtime, _, err := loadSelectedGrammarAssets(context.Background(), EngineAssets, "assets/grammars/manifest.json", false, []string{"typescript"})
	if err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), runtime, root, "fixture", []string{"router.ts"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	if !hasPendingEdge(graph.edges, "router.start", "CALLS", "router.Router.handle") {
		t.Fatalf("typed receiver call: %+v", graph.edges)
	}
}

func TestRustTraitImplConnectsInterfaceAndOverride(t *testing.T) {
	root := t.TempDir()
	body := []byte("pub trait Render { fn render(&self) -> i32; }\npub struct Page;\nimpl Render for Page { fn render(&self) -> i32 { helper() } }\nfn helper() -> i32 { 1 }\n")
	if err := os.WriteFile(filepath.Join(root, "traits.rs"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	runtime, _, err := loadSelectedGrammarAssets(context.Background(), EngineAssets, "assets/grammars/manifest.json", false, []string{"rust"})
	if err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), runtime, root, "fixture", []string{"traits.rs"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	registry := newSymbolRegistry(graph.nodes)
	indexSyntaxInheritance("fixture", repository.Files, &graph, registry)
	indexSyntaxExplicitOverrides(&graph)
	if !hasPendingEdge(graph.edges, "traits.Page", "IMPLEMENTS", "traits.Render") ||
		!hasPendingEdge(graph.edges, "traits.Page.render", "OVERRIDE", "traits.Render.render") {
		t.Fatalf("trait relations: %+v", graph.edges)
	}
	for _, node := range graph.nodes {
		if node.QualifiedName == "traits.Render" && node.Label != "Interface" {
			t.Fatalf("trait label: %s", node.Label)
		}
	}
}
