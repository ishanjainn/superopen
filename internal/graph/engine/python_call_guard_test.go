package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPythonWeakMemberCallGuard(t *testing.T) {
	cases := []struct {
		callee, strategy string
		imports          map[string]string
		external         map[string]bool
		drop             bool
	}{
		{"accelerator.backward", "unique_name", nil, nil, true},
		{"parts.append", "suffix_match", nil, nil, true},
		{"self.parts.append", "unique_name", nil, nil, true},
		{"self.compiler.apply_converters", "unique_name", nil, nil, false},
		{"self.compiler.apply_converters", "suffix_match", nil, nil, true},
		{"self.run", "unique_name", nil, nil, false},
		{"super().__init__", "suffix_match", nil, nil, false},
		{"pkg.worker.run", "unique_name", map[string]string{"pkg": "pkg"}, nil, false},
		{"external.run", "unique_name", nil, map[string]bool{"external": true}, false},
		{"accelerator.backward", "local_constructor", nil, nil, false},
		{"self.client().run", "unique_name", nil, nil, true},
	}
	for _, c := range cases {
		if got := pythonSuppressWeakMemberCall(c.callee, c.strategy, c.imports, c.external); got != c.drop {
			t.Errorf("%s %s: drop=%v, want %v", c.callee, c.strategy, got, c.drop)
		}
	}
}

func TestPythonParameterShadowsWeakCall(t *testing.T) {
	parameters := map[string]map[string]bool{"outer": {"run": true}}
	if !pythonParameterShadowsCall("outer.inner", "run", "unique_name", parameters) {
		t.Fatal("parameter shadow was not retained in nested scope")
	}
	if pythonParameterShadowsCall("outer.inner", "run", "lsp_callable_alias", parameters) ||
		pythonParameterShadowsCall("other", "run", "unique_name", parameters) {
		t.Fatal("proved callable or unrelated scope was suppressed")
	}
}

func TestPythonCallEdgesRequireReceiverEvidence(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.py")
	code := `class Accelerator:
    def backward(self):
        pass

class Compiler:
    def apply_converters(self):
        pass

class Host:
    def run(self):
        self.compiler.apply_converters()

class Printer:
    def emit(self):
        pass

class Other:
    def emit(self):
        pass

class Writer:
    def __init__(self):
        self.printer: Printer

    def write(self):
        self.printer.emit()

def process(accelerator):
    accelerator.backward()
    parts = []
    parts.append(1)
    len(parts)

def execute():
    pass

def invoke(execute):
    execute()
`
	if err := os.WriteFile(path, []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	repository, err := ParseSyntaxRepository(context.Background(), testSyntaxGrammarRuntime(t), root, "fixture", []string{"main.py"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	if hasPendingEdge(graph.edges, "main.process", "CALLS", "main.Accelerator.backward") {
		t.Fatal("untyped parameter resolved by method name")
	}
	if hasPendingEdge(graph.edges, "main.invoke", "CALLS", "main.execute") {
		t.Fatal("parameter resolved as same-named project function")
	}
	if !hasPendingEdge(graph.edges, "main.Host.run", "CALLS", "main.Compiler.apply_converters") {
		t.Fatal("unique method owned by self attribute was dropped")
	}
	if !hasPendingEdge(graph.edges, "main.Writer.write", "CALLS", "main.Printer.emit") ||
		hasPendingEdge(graph.edges, "main.Writer.write", "CALLS", "main.Other.emit") {
		t.Fatal("annotated instance field did not select its method")
	}
	if !hasPendingEdge(graph.edges, "main.process", "CALLS", "builtins.list.append") {
		t.Fatal("typed list call did not resolve to builtin method")
	}
	if !hasPendingEdge(graph.edges, "main.process", "CALLS", "builtins.len") {
		t.Fatal("bare builtin call did not resolve")
	}
}
