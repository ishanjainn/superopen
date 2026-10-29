package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPythonSpawnProgramUsesOnlyFirstCollectionElement(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`['/usr/bin/git', 'status']`, "git"},
		{`[sys.executable, 'status']`, "<dynamic>"},
		{`'env LC_ALL=C git status'`, "git"},
		{`('dot', '-Tsvg')`, "dot"},
		{`f"git clone --depth=1 {git_repo}"`, "git"},
	}
	for _, tc := range cases {
		call := SyntaxFact{Arguments: []SyntaxArgument{{Expr: tc.expr, Index: 0}}}
		if got := spawnProgram(call, 0); got != tc.want {
			t.Errorf("%s: %s, want %s", tc.expr, got, tc.want)
		}
	}
}

func TestPythonSubprocessCallBuildsProcessRelationship(t *testing.T) {
	root := t.TempDir()
	body := []byte("import subprocess\n\ndef build():\n    subprocess.run(['git', 'status'])\n")
	if err := os.WriteFile(filepath.Join(root, "task.py"), body, 0600); err != nil {
		t.Fatal(err)
	}
	outcome := parseSyntaxFile(context.Background(), testSyntaxGrammarRuntime(t), root, "test", "task.py", nil)
	if outcome.file == nil || outcome.coverage != nil || outcome.err != nil {
		t.Fatalf("parse: %+v", outcome)
	}
	graph, _ := AssembleSyntaxGraph(SyntaxRepository{Files: []ParsedSyntaxFile{*outcome.file}}, "test")
	process, spawn := false, false
	for _, node := range graph.nodes {
		if node.Label == "Process" && node.Name == "git" {
			process = true
		}
	}
	for _, edge := range graph.edges {
		if edge.kind == "SPAWNS" && edge.target == "__process__git" {
			spawn = true
		}
	}
	if !process || !spawn {
		t.Fatalf("subprocess graph: process=%v spawn=%v", process, spawn)
	}
}

func TestGoAndRustQualifiedSpawnCalls(t *testing.T) {
	for _, tc := range []struct{ file, code, source, process string }{
		{"task.go", "package task\nimport \"os/exec\"\nfunc run() { exec.Command(\"git\", \"status\") }\n", "run", "git"},
		{"task.rs", "use std::process;\nfn run() { process::Command::new(\"echo\"); }\n", "task.run", "echo"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, tc.file), []byte(tc.code), 0o600); err != nil {
				t.Fatal(err)
			}
			runtime := testSyntaxGrammarRuntime(t)
			if filepath.Ext(tc.file) == ".rs" {
				var err error
				runtime, _, err = loadSelectedGrammarAssets(context.Background(), EngineAssets, "assets/grammars/manifest.json", false, []string{"rust"})
				if err != nil {
					t.Fatal(err)
				}
			}
			repository, err := ParseSyntaxRepository(context.Background(), runtime, root, "fixture", []string{tc.file}, nil, 1)
			if err != nil {
				t.Fatal(err)
			}
			graph, _ := AssembleSyntaxGraph(repository, "fixture")
			if !hasPendingEdge(graph.edges, tc.source, "SPAWNS", "__process__"+tc.process) {
				t.Fatalf("missing process spawn: imports=%#v calls=%#v", repository.Files[0].Extraction.Imports, repository.Files[0].Extraction.Calls)
			}
		})
	}
}
