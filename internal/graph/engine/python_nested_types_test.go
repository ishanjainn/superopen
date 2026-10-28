package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPythonNestedFunctionsInheritTypedReceivers(t *testing.T) {
	root := t.TempDir()
	code := `from typing import Optional

class Environment:
    def path2doc(self, path): pass

class Builder:
    def __init__(self, env: Environment):
        self.env: Optional[Environment] = env

    def run(self, other: Environment):
        def inner():
            self.env.path2doc('a')
            other.path2doc('b')
        inner()
`
	if err := os.WriteFile(filepath.Join(root, "builder.py"), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", []string{"builder.py"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	if !hasPendingEdge(graph.edges, "builder.Builder.run.inner", "CALLS", "builder.Environment.path2doc") {
		t.Fatalf("nested typed call missing: %#v", graph.edges)
	}
}

func TestPythonCrossFileFieldTypeRoutesChainedCall(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"env.py":    "class Environment:\n    def get_domain(self, name): pass\n",
		"app.py":    "from typing import Optional\nfrom env import Environment\nclass App:\n    def __init__(self):\n        self.env: Optional[Environment] = None\n",
		"caller.py": "from app import App\ndef run(app: App):\n    app.env.get_domain('test')\n",
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
	if !hasPendingEdge(graph.edges, "caller.run", "CALLS", "env.Environment.get_domain") {
		t.Fatalf("cross-file field call missing: %#v", graph.edges)
	}
}

func TestPythonTypedReceiverFindsInheritedMethod(t *testing.T) {
	root := t.TempDir()
	code := "class Base:\n    def render_string(self): pass\nclass Mid(Base): pass\nclass Renderer(Mid): pass\ndef run():\n    renderer = Renderer()\n    renderer.render_string()\n"
	if err := os.WriteFile(filepath.Join(root, "renderer.py"), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", []string{"renderer.py"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	if !hasPendingEdge(graph.edges, "renderer.run", "CALLS", "renderer.Base.render_string") {
		t.Fatalf("inherited typed method missing: %#v", graph.edges)
	}
}

func TestPythonCastCarriesMethodReceiverType(t *testing.T) {
	root := t.TempDir()
	code := "from typing import cast\nclass Domain:\n    def clear(self): pass\ndef run(value):\n    domain = cast(Domain, value)\n    domain.clear()\n"
	if err := os.WriteFile(filepath.Join(root, "domain.py"), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", []string{"domain.py"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	if !hasPendingEdge(graph.edges, "domain.run", "CALLS", "domain.Domain.clear") {
		t.Fatalf("cast receiver call missing: %#v", graph.edges)
	}
}

func TestPythonCrossFileFieldChainRoutesCall(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"env.py":     "class Environment:\n    def doc2path(self, name): pass\n",
		"builder.py": "from env import Environment\nclass Builder:\n    def __init__(self, env: Environment):\n        self.env = env\n",
		"app.py":     "from builder import Builder\nclass App:\n    def __init__(self, builder: Builder):\n        self.builder = builder\n",
		"caller.py":  "from app import App\ndef run(app: App):\n    app.builder.env.doc2path('test')\n",
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
	if !hasPendingEdge(graph.edges, "caller.run", "CALLS", "env.Environment.doc2path") {
		t.Fatalf("cross-file field chain missing: %#v", graph.edges)
	}
}

func TestPythonInheritedFieldRoutesMethodCall(t *testing.T) {
	root := t.TempDir()
	code := "class Environment:\n    def doc2path(self, name): pass\nclass Base:\n    def __init__(self, env: Environment):\n        self.env = env\nclass Child(Base):\n    def run(self):\n        self.env.doc2path('test')\n"
	if err := os.WriteFile(filepath.Join(root, "builder.py"), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", []string{"builder.py"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	if !hasPendingEdge(graph.edges, "builder.Child.run", "CALLS", "builder.Environment.doc2path") {
		t.Fatalf("inherited field call missing: %#v", graph.edges)
	}
}

func TestPythonChainedCallUsesReturnAnnotation(t *testing.T) {
	root := t.TempDir()
	code := "class Nested:\n    def get_id(self): pass\nclass Symbol:\n    def get_nested(self) -> Nested: return Nested()\ndef run(symbol: Symbol):\n    symbol.get_nested().get_id()\n"
	if err := os.WriteFile(filepath.Join(root, "symbols.py"), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", []string{"symbols.py"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	if !hasPendingEdge(graph.edges, "symbols.run", "CALLS", "symbols.Nested.get_id") {
		t.Fatalf("chained typed return missing: %#v", graph.edges)
	}
}

func TestPythonIndexedTypedFieldRoutesMethod(t *testing.T) {
	root := t.TempDir()
	code := "class Item:\n    def render(self): pass\nclass Parent:\n    def __init__(self, items: list[Item]):\n        self.items = items\n    def run(self):\n        self.items[0].render()\n"
	if err := os.WriteFile(filepath.Join(root, "items.py"), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", []string{"items.py"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	if !hasPendingEdge(graph.edges, "items.Parent.run", "CALLS", "items.Item.render") {
		t.Fatalf("indexed field call missing: %#v", graph.edges)
	}
}

func TestPythonBareImportIsNotEnclosingClassMethod(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"loader.py":   "def get_template(name): pass\n",
		"renderer.py": "from loader import get_template\nclass Renderer:\n    def get_template(self, name):\n        return get_template(name)\n",
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
	if !hasPendingEdge(graph.edges, "renderer.Renderer.get_template", "CALLS", "loader.get_template") {
		t.Fatalf("imported callable shadowed by class method: %#v", graph.edges)
	}
}

func TestPythonPackageReexportTypesModuleLevelReceiver(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"pkg/__init__.py": "from .library import Library\n",
		"pkg/library.py":  "class Library:\n    def tag(self): pass\n",
		"consumer.py":     "from pkg import Library\nregister = Library()\nregister.tag()\n",
	}
	var paths []string
	for path, code := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o700); err != nil {
			t.Fatal(err)
		}
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
	if !hasPendingEdge(graph.edges, "consumer", "CALLS", "pkg.library.Library.tag") {
		t.Fatalf("package reexport receiver missing: %#v", graph.edges)
	}
}

func TestPythonChainedModuleReexport(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"pkg/__init__.py": "from .bridge import Library\n",
		"pkg/bridge.py":   "from .library import Library\n",
		"pkg/library.py":  "class Library:\n    def tag(self): pass\n",
		"consumer.py":     "from pkg import Library\nregister = Library()\nregister.tag()\n",
	}
	var paths []string
	for path, code := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o700); err != nil {
			t.Fatal(err)
		}
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
	if !hasPendingEdge(graph.edges, "consumer", "CALLS", "pkg.library.Library.tag") {
		t.Fatalf("chained package reexport missing: %#v", graph.edges)
	}
}

func TestPythonNestedClassMethodConstructor(t *testing.T) {
	root := t.TempDir()
	code := "class View:\n    def setup(self): pass\n    @classmethod\n    def as_view(cls):\n        def view():\n            self = cls()\n            self.setup()\n        return view\n"
	if err := os.WriteFile(filepath.Join(root, "views.py"), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", []string{"views.py"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	if !hasPendingEdge(graph.edges, "views.View.as_view.view", "CALLS", "views.View.setup") {
		t.Fatalf("nested cls constructor lost method call: %#v", graph.edges)
	}
}

func TestPythonInheritedSelfAndClassCalls(t *testing.T) {
	root := t.TempDir()
	code := "class Base:\n    def clean(self): pass\n    @classmethod\n    def as_view(cls): pass\nclass Child(Base):\n    def run(self): self.clean()\ndef call(): Child.as_view()\n"
	if err := os.WriteFile(filepath.Join(root, "views.py"), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", []string{"views.py"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	if !hasPendingEdge(graph.edges, "views.Child.run", "CALLS", "views.Base.clean") ||
		!hasPendingEdge(graph.edges, "views.call", "CALLS", "views.Base.as_view") {
		t.Fatalf("inherited method dispatch missing: %#v", graph.edges)
	}
}
