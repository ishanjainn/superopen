package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/ishanjainn/superopen/internal/graph/engine/cpreproc"
)

func TestMergePreprocessedCallsKeepMainFileOnly(t *testing.T) {
	t.Parallel()
	raw := FileResult{
		Calls: []SyntaxFact{{Kind: "call", Name: "LOG", StartLine: 3}},
	}
	expanded := FileResult{
		Calls: []SyntaxFact{
			{Kind: "call", Name: "printk", StartLine: 4, EndLine: 4, StartByte: 99},
			{Kind: "call", Name: "header_fn", StartLine: 8, EndLine: 8},
		},
		Definitions: []SyntaxFact{{Kind: "function", Name: "header_only", StartLine: 8}},
	}
	pp := &cpreproc.Result{
		OriginalLine:  []uint32{0, 1, 2, 3, 3, 0, 0, 0, 1},
		BelongsToMain: []bool{false, true, true, true, true, false, false, false, false},
	}
	got := mergePreprocessedFileResult(raw, expanded, pp, []byte("#define LOG(m) printk(m)\nvoid f(void) { LOG(\"x\"); }\n"))
	if len(got.Calls) != 2 {
		t.Fatalf("calls = %#v", got.Calls)
	}
	if got.Calls[1].Name != "printk" || got.Calls[1].StartLine != 3 || got.Calls[1].SourceOrigin != sourceOriginPreprocessed {
		t.Fatalf("merged call = %#v", got.Calls[1])
	}
	for _, def := range got.Definitions {
		if def.Name == "header_only" {
			t.Fatal("header definition leaked into the main FileResult")
		}
	}
}

func TestSplitCCallee(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, recv, meth, form string
	}{
		{"printk", "", "printk", "direct"},
		{"rq->enqueue", "rq", "enqueue", "arrow"},
		{"obj.method", "obj", "method", "dot"},
		{"Foo::bar", "Foo", "bar", "scoped"},
		{"a->b->c", "a->b", "c", "arrow"},
	}
	for _, tc := range cases {
		recv, meth, form := splitCCallee(tc.name)
		if recv != tc.recv || meth != tc.meth || form != tc.form {
			t.Fatalf("%s: got %q %q %q", tc.name, recv, meth, form)
		}
	}
}

func TestCTypeDispatchResolvesArrowCall(t *testing.T) {
	t.Parallel()
	files := []ParsedSyntaxFile{
		{File: FileRecord{Path: "sched.c", Language: "c"}, Extraction: FileResult{
			Definitions: []SyntaxFact{
				{Kind: "class", Name: "rq"},
				{Kind: "function", Name: "enqueue", Scope: "rq", ParamNames: []string{"self"}, ParamTypes: []string{"rq"}},
				{Kind: "function", Name: "schedule", StartLine: 1, EndLine: 10, ParamNames: []string{"rq"}, ParamTypes: []string{"rq"}},
			},
			Calls: []SyntaxFact{{Kind: "call", Name: "rq->enqueue", Scope: "schedule", StartLine: 4}},
		}},
	}
	if err := enrichCResolvedCalls(context.Background(), files); err != nil {
		t.Fatal(err)
	}
	if len(files[0].Extraction.ResolvedCalls) != 1 {
		t.Fatalf("resolved = %#v", files[0].Extraction.ResolvedCalls)
	}
	got := files[0].Extraction.ResolvedCalls[0]
	if got.Target != "sched.enqueue" && !strings.HasSuffix(got.Target, ".enqueue") {
		t.Fatalf("target = %q", got.Target)
	}
	if got.Strategy != "lsp_type_dispatch" {
		t.Fatalf("strategy = %q", got.Strategy)
	}
}

func TestAssembleSkipsCallReferenceWhenCResolved(t *testing.T) {
	t.Parallel()
	repository := SyntaxRepository{
		Files: []ParsedSyntaxFile{{
			File: FileRecord{Project: "fixture", Path: "sched.c", Language: "c"},
			Extraction: FileResult{
				Definitions: []SyntaxFact{{Kind: "function", Name: "schedule", StartLine: 1, EndLine: 10}, {Kind: "function", Name: "enqueue", StartLine: 20}},
				Calls:       []SyntaxFact{{Kind: "call", Name: "rq->enqueue", Scope: "schedule", StartLine: 4}},
				ResolvedCalls: []ResolvedRelationship{{
					Source: "sched.schedule", Target: "sched.enqueue", Type: "CALLS", Strategy: "lsp_type_dispatch", Confidence: .9,
					Location:   api.Location{File: "sched.c", StartLine: 4},
					Properties: api.Properties{"callee": "rq->enqueue"},
				}},
			},
		}},
	}
	graph, _ := AssembleSyntaxGraph(repository, "fixture")
	for _, edge := range graph.unresolved {
		if edge.kind == "CALL_REFERENCE" && strings.Contains(edge.target, "enqueue") {
			t.Fatalf("unresolved leftover: %#v", edge)
		}
	}
}

var (
	testCRuntime     *GrammarRuntime
	testCRuntimeErr  error
	testCRuntimeOnce sync.Once
)

func testCFamilyParser(t *testing.T) SyntaxParser {
	t.Helper()
	testCRuntimeOnce.Do(func() {
		testCRuntime, _, testCRuntimeErr = loadSelectedGrammarAssets(
			context.Background(), EngineAssets, "assets/grammars/manifest.json", false,
			[]string{"c", "cpp"},
		)
	})
	if testCRuntimeErr != nil {
		t.Fatal(testCRuntimeErr)
	}
	if native := nativeSyntaxParser(); native != nil {
		return &fallbackSyntaxParser{native: native, wasm: testCRuntime}
	}
	return testCRuntime
}

func TestCPreprocessorSurfacesMacroHiddenCalls(t *testing.T) {
	src := []byte("#define LOG(msg) printk(msg)\nvoid report(void) { LOG(\"x\"); }\n")
	if cpreproc.WithMap(src, "log.c", false) == nil {
		t.Skip("C preprocessor requires cgo")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "log.c"), src, 0o600); err != nil {
		t.Fatal(err)
	}
	repo, err := ParseSyntaxRepository(context.Background(), testCFamilyParser(t), root, "fixture", []string{"log.c"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.Files) != 1 {
		t.Fatalf("files = %#v", repo.Files)
	}
	var names []string
	var sawPreprocessed bool
	for _, call := range repo.Files[0].Extraction.Calls {
		names = append(names, call.Name)
		if call.Name == "printk" && call.SourceOrigin == sourceOriginPreprocessed {
			sawPreprocessed = true
		}
	}
	if !sawPreprocessed {
		t.Fatalf("missing preprocessed printk call: %v", names)
	}
}

func TestCPreprocessorDoesNotAdoptHeaderDefs(t *testing.T) {
	if cpreproc.WithMap([]byte("#define X 1\nint x = X;\n"), "t.c", false) == nil {
		t.Skip("C preprocessor requires cgo")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "inc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "inc", "hidden.h"), []byte("void header_only(void) {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := []byte("#include \"hidden.h\"\n#define LOG(msg) printk(msg)\nvoid report(void) { LOG(\"x\"); }\n")
	if err := os.WriteFile(filepath.Join(root, "inc", "main.c"), src, 0o600); err != nil {
		t.Fatal(err)
	}
	repo, err := ParseSyntaxRepository(context.Background(), testCFamilyParser(t), root, "fixture", []string{"inc/main.c"}, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.Files) != 1 {
		t.Fatalf("files = %#v", repo.Files)
	}
	for _, def := range repo.Files[0].Extraction.Definitions {
		if def.Name == "header_only" {
			t.Fatalf("header def leaked: %#v", repo.Files[0].Extraction.Definitions)
		}
	}
}
