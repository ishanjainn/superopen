package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/graph"
)

func TestGraphFacadePinnedSurface(t *testing.T) {
	root := newGraphCommand()
	want := []string{"extract", "rebuild", "update", "watch", "check-update", "query", "path", "explain", "affected", "god-nodes", "stats", "diagnose", "cluster", "label", "export", "serve", "mcp", "add", "clone", "merge", "global", "prs", "result", "reflect", "benchmark", "semantic", "labels", "publish"}
	found := map[string]bool{}
	for _, c := range root.Commands() {
		found[c.Name()] = true
	}
	if root.Flags().Lookup("code-only") == nil {
		t.Error("bare so graph must preserve --code-only")
	}
	for _, name := range want {
		if !found[name] {
			t.Errorf("pinned Graphify facade missing %s", name)
		}
	}
	for _, c := range root.Commands() {
		if c.DisableFlagParsing {
			t.Errorf("%s still bypasses Cobra flag parsing", c.CommandPath())
		}
	}
	affected, _, _ := root.Find([]string{"affected"})
	for _, flag := range []string{"relation", "depth"} {
		if affected.Flags().Lookup(flag) == nil {
			t.Errorf("affected help missing explicit --%s", flag)
		}
	}
	for command, schema := range graph.CommandSchema {
		if schema.Native == "" || schema.ExcludedReason != "" {
			continue
		}
		facade := command
		if strings.HasPrefix(command, "export:") {
			continue
		}
		if command == "export" || command == "diagnose" || command == "global" || command == "tree" {
			continue
		}
		cmd, _, err := root.Find([]string{facade})
		if err != nil {
			continue // native names such as cluster-only have a reviewed alias
		}
		for _, flag := range schema.Flags {
			if cmd.Flags().Lookup(flag.Name) == nil {
				t.Errorf("%s help missing schema flag --%s", command, flag.Name)
			}
		}
	}
	for _, path := range [][]string{{"diagnose", "multigraph"}, {"global", "add"}} {
		cmd, _, err := root.Find(path)
		if err != nil {
			t.Fatal(err)
		}
		schema := graph.CommandSchema[path[0]]
		for _, flag := range schema.Flags {
			if cmd.Flags().Lookup(flag.Name) == nil {
				t.Errorf("%s help missing schema flag --%s", strings.Join(path, " "), flag.Name)
			}
		}
	}
	for key, schema := range graph.CommandSchema {
		if !strings.HasPrefix(key, "export:") || key == "export:canvas" || key == "export:cypher" {
			continue
		}
		name := strings.TrimPrefix(key, "export:")
		cmd, _, err := root.Find([]string{"export", name})
		if err != nil {
			t.Fatal(err)
		}
		for _, flag := range schema.Flags {
			if cmd.Flags().Lookup(flag.Name) == nil {
				t.Errorf("export %s help missing schema flag --%s", name, flag.Name)
			}
		}
	}
	tree, _, err := root.Find([]string{"export", "tree"})
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range graph.CommandSchema["tree"].Flags {
		if tree.Flags().Lookup(flag.Name) == nil {
			t.Errorf("export tree help missing schema flag --%s", flag.Name)
		}
	}
	exports, _, _ := root.Find([]string{"export"})
	for _, name := range []string{"html", "wiki", "obsidian", "svg", "graphml", "canvas", "cypher", "callflow", "tree", "neo4j", "falkordb"} {
		if child, _, err := exports.Find([]string{name}); err != nil || child.Name() != name {
			t.Errorf("export facade missing %s", name)
		}
	}
}

func TestSemanticOptionsUsesConfiguredDeepMode(t *testing.T) {
	root := newGraphCommand()
	extract, _, err := root.Find([]string{"extract"})
	if err != nil {
		t.Fatal(err)
	}
	opts, err := semanticOptions(extract, t.TempDir(), "deep")
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Deep {
		t.Fatal("configured deep mode was not propagated to agent semantic options")
	}
}

func TestValidateDSNRejectsCredentials(t *testing.T) {
	for _, dsn := range []string{
		"postgres://user:secret@localhost/db",
		"postgres://localhost/db?password=secret",
	} {
		if err := validateDSN(dsn, "postgres"); err == nil {
			t.Errorf("credential-bearing DSN was accepted: %s", dsn)
		}
	}
	if err := validateDSN("postgres://localhost/db?sslmode=require", "postgres"); err != nil {
		t.Fatalf("safe DSN rejected: %v", err)
	}
}

func TestResolveGraphUpdateTargetRejectsEscapingSymlink(t *testing.T) {
	repo := t.TempDir()
	inside := filepath.Join(repo, "pkg")
	if err := os.Mkdir(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := resolveGraphUpdateTarget(repo, []string{inside})
	want, _ := filepath.EvalSymlinks(inside)
	if err != nil || got != want {
		t.Fatalf("inside target = %q, %v", got, err)
	}
	link := filepath.Join(repo, "outside")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := resolveGraphUpdateTarget(repo, []string{link}); err == nil {
		t.Fatal("update target accepted a symlink escaping the repository")
	}
}
