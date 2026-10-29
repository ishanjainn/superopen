package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEmptyPythonPackageHasFileWithoutModule(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "__init__.py"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	outcome := parseSyntaxFile(context.Background(), testSyntaxGrammarRuntime(t), root, "test", "__init__.py", nil)
	if outcome.file == nil || outcome.err != nil {
		t.Fatalf("parse: %+v", outcome)
	}
	graph, _ := AssembleSyntaxGraph(SyntaxRepository{Files: []ParsedSyntaxFile{*outcome.file}}, "test")
	file, module := false, false
	for _, node := range graph.nodes {
		file = file || node.Label == "File" && node.Location.File == "__init__.py"
		module = module || node.Label == "Module" && node.Location.File == "__init__.py"
	}
	if !file || module {
		t.Fatalf("empty package nodes: %+v", graph.nodes)
	}
}

func TestPythonSuperConstructorCallsBaseConstructor(t *testing.T) {
	root := t.TempDir()
	body := []byte("class Base:\n    def __init__(self):\n        pass\n\nclass Child(Base):\n    def __init__(self):\n        super().__init__()\n")
	if err := os.WriteFile(filepath.Join(root, "model.py"), body, 0600); err != nil {
		t.Fatal(err)
	}
	outcome := parseSyntaxFile(context.Background(), testSyntaxGrammarRuntime(t), root, "test", "model.py", nil)
	if outcome.file == nil || outcome.err != nil {
		t.Fatalf("parse: %+v", outcome)
	}
	graph, _ := AssembleSyntaxGraph(SyntaxRepository{Files: []ParsedSyntaxFile{*outcome.file}}, "test")
	for _, edge := range graph.edges {
		if edge.kind == "CALLS" && edge.source == "model.Child.__init__" && edge.target == "model.Base.__init__" {
			return
		}
	}
	t.Fatalf("base constructor call missing: %+v", graph.edges)
}

func TestPythonInlineConstructorMethodCall(t *testing.T) {
	root := t.TempDir()
	body := []byte("class Worker:\n    def run(self):\n        pass\n\ndef start():\n    Worker().run()\n")
	if err := os.WriteFile(filepath.Join(root, "worker.py"), body, 0600); err != nil {
		t.Fatal(err)
	}
	outcome := parseSyntaxFile(context.Background(), testSyntaxGrammarRuntime(t), root, "test", "worker.py", nil)
	if outcome.file == nil || outcome.err != nil {
		t.Fatalf("parse: %+v", outcome)
	}
	graph, _ := AssembleSyntaxGraph(SyntaxRepository{Files: []ParsedSyntaxFile{*outcome.file}}, "test")
	for _, edge := range graph.edges {
		if edge.kind == "CALLS" && edge.source == "worker.start" && edge.target == "worker.Worker.run" {
			return
		}
	}
	t.Fatalf("inline constructor method call missing: %+v", graph.edges)
}

func TestPythonAnnotatedParameterAndFieldAliasResolveMethods(t *testing.T) {
	root := t.TempDir()
	body := []byte("class Manager:\n    def register(self):\n        pass\n\nclass Provider:\n    def __init__(self, manager: Manager):\n        self.manager = manager\n        self.manager.register()\n\ndef run(manager: Manager):\n    alias: Manager = manager\n    alias.register()\n")
	if err := os.WriteFile(filepath.Join(root, "provider.py"), body, 0600); err != nil {
		t.Fatal(err)
	}
	outcome := parseSyntaxFile(context.Background(), testSyntaxGrammarRuntime(t), root, "test", "provider.py", nil)
	if outcome.file == nil || outcome.err != nil {
		t.Fatalf("parse: %+v", outcome)
	}
	graph, _ := AssembleSyntaxGraph(SyntaxRepository{Files: []ParsedSyntaxFile{*outcome.file}}, "test")
	for _, source := range []string{"provider.Provider.__init__", "provider.run"} {
		if !hasPendingEdge(graph.edges, source, "CALLS", "provider.Manager.register") {
			t.Errorf("annotated receiver %s: %+v", source, graph.edges)
		}
	}
}

func TestPythonTypedListFlowsThroughFieldAndForLoop(t *testing.T) {
	root := t.TempDir()
	body := []byte("class Reporter:\n    def handle(self):\n        pass\n\nclass Multi:\n    def __init__(self, reporters: list[Reporter]):\n        self.reporters = reporters\n    def run(self):\n        for reporter in self.reporters:\n            reporter.handle()\n")
	if err := os.WriteFile(filepath.Join(root, "reporters.py"), body, 0600); err != nil {
		t.Fatal(err)
	}
	outcome := parseSyntaxFile(context.Background(), testSyntaxGrammarRuntime(t), root, "test", "reporters.py", nil)
	if outcome.file == nil || outcome.err != nil {
		t.Fatalf("parse: %+v", outcome)
	}
	graph, _ := AssembleSyntaxGraph(SyntaxRepository{Files: []ParsedSyntaxFile{*outcome.file}}, "test")
	if !hasPendingEdge(graph.edges, "reporters.Multi.run", "CALLS", "reporters.Reporter.handle") {
		t.Fatalf("typed list element call: %+v", graph.edges)
	}
}

func TestPythonOwnMethodWinsOverSelfAnnotation(t *testing.T) {
	root := t.TempDir()
	body := []byte("class Other:\n    def order(self):\n        pass\n\nclass MixIn:\n    def order(self):\n        pass\n    def run(self: Other):\n        self.order()\n")
	if err := os.WriteFile(filepath.Join(root, "mixin.py"), body, 0600); err != nil {
		t.Fatal(err)
	}
	outcome := parseSyntaxFile(context.Background(), testSyntaxGrammarRuntime(t), root, "test", "mixin.py", nil)
	if outcome.file == nil || outcome.err != nil {
		t.Fatalf("parse: %+v", outcome)
	}
	graph, _ := AssembleSyntaxGraph(SyntaxRepository{Files: []ParsedSyntaxFile{*outcome.file}}, "test")
	if !hasPendingEdge(graph.edges, "mixin.MixIn.run", "CALLS", "mixin.MixIn.order") {
		t.Fatalf("own method lost to self annotation: %+v", graph.edges)
	}
}

func TestPythonListLiteralAndIndexedFieldTypes(t *testing.T) {
	root := t.TempDir()
	body := []byte("class Consumer:\n    def use(self):\n        pass\n\nclass Holder:\n    def __init__(self):\n        self.items: list[Consumer] = []\n    def run(self):\n        values = []\n        values.append(1)\n        current = self.items[0]\n        current.use()\n")
	if err := os.WriteFile(filepath.Join(root, "lists.py"), body, 0600); err != nil {
		t.Fatal(err)
	}
	outcome := parseSyntaxFile(context.Background(), testSyntaxGrammarRuntime(t), root, "test", "lists.py", nil)
	if outcome.file == nil || outcome.err != nil {
		t.Fatalf("parse: %+v", outcome)
	}
	graph, _ := AssembleSyntaxGraph(SyntaxRepository{Files: []ParsedSyntaxFile{*outcome.file}}, "test")
	for _, target := range []string{"builtins.list.append", "lists.Consumer.use"} {
		if !hasPendingEdge(graph.edges, "lists.Holder.run", "CALLS", target) {
			t.Errorf("missing %s: %+v", target, graph.edges)
		}
	}
}

func TestPythonBuiltinContainersDoNotGuessProjectMethods(t *testing.T) {
	root := t.TempDir()
	body := []byte("class Other:\n    def add(self, value):\n        pass\n    def setdefault(self, key, value):\n        pass\n\ndef run():\n    visited = set()\n    visited.add(1)\n    options = {}\n    options.setdefault('x', 1)\n")
	if err := os.WriteFile(filepath.Join(root, "containers.py"), body, 0600); err != nil {
		t.Fatal(err)
	}
	outcome := parseSyntaxFile(context.Background(), testSyntaxGrammarRuntime(t), root, "test", "containers.py", nil)
	if outcome.file == nil || outcome.err != nil {
		t.Fatalf("parse: %+v", outcome)
	}
	graph, _ := AssembleSyntaxGraph(SyntaxRepository{Files: []ParsedSyntaxFile{*outcome.file}}, "test")
	for _, edge := range graph.edges {
		if edge.source == "containers.run" && edge.kind == "CALLS" &&
			(edge.target == "containers.Other.add" || edge.target == "containers.Other.setdefault") {
			t.Fatalf("builtin container routed to project method: %+v", edge)
		}
	}
}

func TestPythonLocalFallbackDefinitionWinsOverExternalImport(t *testing.T) {
	root := t.TempDir()
	body := []byte("try:\n    from external import Filter\nexcept ImportError:\n    class Filter:\n        pass\n\nFilter()\n")
	if err := os.WriteFile(filepath.Join(root, "fallback.py"), body, 0600); err != nil {
		t.Fatal(err)
	}
	outcome := parseSyntaxFile(context.Background(), testSyntaxGrammarRuntime(t), root, "test", "fallback.py", nil)
	if outcome.file == nil || outcome.err != nil {
		t.Fatalf("parse: %+v", outcome)
	}
	graph, _ := AssembleSyntaxGraph(SyntaxRepository{Files: []ParsedSyntaxFile{*outcome.file}}, "test")
	if !hasPendingEdge(graph.edges, syntaxModuleQN("fallback.py"), "CALLS", "fallback.Filter") {
		t.Fatalf("local fallback: %+v", graph.edges)
	}
}

func TestPythonSuperUsesC3OrderAcrossDiamond(t *testing.T) {
	root := t.TempDir()
	body := []byte("class A:\n    def __init__(self):\n        pass\n\nclass B(A):\n    pass\n\nclass C(A):\n    def __init__(self):\n        pass\n\nclass D(B, C):\n    def __init__(self):\n        super().__init__()\n")
	if err := os.WriteFile(filepath.Join(root, "diamond.py"), body, 0600); err != nil {
		t.Fatal(err)
	}
	outcome := parseSyntaxFile(context.Background(), testSyntaxGrammarRuntime(t), root, "test", "diamond.py", nil)
	if outcome.file == nil || outcome.err != nil {
		t.Fatalf("parse: %+v", outcome)
	}
	graph, _ := AssembleSyntaxGraph(SyntaxRepository{Files: []ParsedSyntaxFile{*outcome.file}}, "test")
	for _, edge := range graph.edges {
		if edge.kind == "CALLS" && edge.source == "diamond.D.__init__" {
			if edge.target != "diamond.C.__init__" {
				t.Fatalf("wrong C3 target: %+v", edge)
			}
			return
		}
	}
	t.Fatalf("diamond super constructor missing: %+v", graph.edges)
}

func TestPythonCallableAliasTracksAssignmentAndShadow(t *testing.T) {
	root := t.TempDir()
	body := []byte("def outer():\n    def original():\n        pass\n    alias = original\n    another = alias\n    another()\n    alias = 42\n    alias()\n")
	if err := os.WriteFile(filepath.Join(root, "aliases.py"), body, 0600); err != nil {
		t.Fatal(err)
	}
	outcome := parseSyntaxFile(context.Background(), testSyntaxGrammarRuntime(t), root, "test", "aliases.py", nil)
	if outcome.file == nil || outcome.err != nil {
		t.Fatalf("parse: %+v", outcome)
	}
	graph, _ := AssembleSyntaxGraph(SyntaxRepository{Files: []ParsedSyntaxFile{*outcome.file}}, "test")
	if !hasPendingEdge(graph.edges, "aliases.outer", "CALLS", "aliases.outer.original") {
		t.Fatalf("callable alias missing: %+v", graph.edges)
	}
	for _, edge := range graph.edges {
		if edge.kind == "CALLS" && edge.source == "aliases.outer" && edge.properties["callee"] == "alias" {
			t.Fatalf("reassigned alias remained callable: %+v", edge)
		}
	}
}

func TestPythonNestedFunctionCallResolvesLexicalTarget(t *testing.T) {
	root := t.TempDir()
	body := []byte("def outer():\n    def nested():\n        pass\n    nested()\n")
	if err := os.WriteFile(filepath.Join(root, "nested.py"), body, 0600); err != nil {
		t.Fatal(err)
	}
	outcome := parseSyntaxFile(context.Background(), testSyntaxGrammarRuntime(t), root, "test", "nested.py", nil)
	if outcome.file == nil || outcome.err != nil {
		t.Fatalf("parse: %+v", outcome)
	}
	graph, _ := AssembleSyntaxGraph(SyntaxRepository{Files: []ParsedSyntaxFile{*outcome.file}}, "test")
	if !hasPendingEdge(graph.edges, "nested.outer", "CALLS", "nested.outer.nested") {
		t.Fatalf("nested call: %+v", graph.edges)
	}
}

func TestPythonImportedClassCallFindsUniqueInheritedMethod(t *testing.T) {
	root := t.TempDir()
	for path, body := range map[string]string{
		"pkg/base.py":        "class Base:\n    def visit(self, value):\n        return value\n",
		"pkg/impl.py":        "from pkg.base import Base\nclass Derived(Base):\n    pass\n",
		"tests/test_impl.py": "from pkg.impl import Derived\ndef run():\n    Derived().visit(None)\n",
	} {
		abs := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(abs), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "test", []string{"pkg/base.py", "pkg/impl.py", "tests/test_impl.py"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "test")
	if !hasPendingEdge(graph.edges, "tests.test_impl.run", "CALLS", "pkg.base.Base.visit") {
		t.Fatalf("inherited method: %+v", graph.edges)
	}
}
