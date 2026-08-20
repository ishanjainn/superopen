package engine

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestIndexGoDevelopmentBuildsGroundedGraph(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	repo := t.TempDir()
	writeFixtureFile(t, repo, "a.go", `package sample

import "fmt"

type Runner struct{}

func Helper() {}

func (r *Runner) Run() {
	Helper()
	fmt.Println("running")
}
`)
	writeFixtureFile(t, repo, "web/view.ts", `export function view() { return "ok" }
`)
	writeFixtureFile(t, repo, ".so/hidden.go", "package hidden\nfunc Hidden() {}\n")

	result, err := IndexGoDevelopment(context.Background(), api.BuildRequest{RepoRoot: repo, Project: "sample"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "ok" || result.NodeCount < 5 || result.EdgeCount < 7 || result.FileCount != 1 {
		t.Fatalf("unexpected build result: %+v", result)
	}
	if result.Coverage.Status != "partial" || len(result.Coverage.Rows) != 1 || result.Coverage.Rows[0].Path != "web/view.ts" {
		t.Fatalf("unexpected coverage: %+v", result.Coverage)
	}

	store, err := OpenReadOnly(result.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	search, err := store.Search(context.Background(), api.SearchRequest{Project: "sample", Query: "Runner Run"})
	if err != nil {
		t.Fatal(err)
	}
	foundMethod := false
	for _, match := range search.Matches {
		foundMethod = foundMethod || match.QualifiedName == "sample.Run"
	}
	if !foundMethod {
		t.Fatalf("unexpected search: %+v", search)
	}
	semantic, err := store.Search(context.Background(), api.SearchRequest{Project: "sample", SemanticQuery: []string{"runner", "run"}, Limit: 5})
	if err != nil || len(semantic.Semantic) == 0 || semantic.Semantic[0].QualifiedName != "sample.Run" {
		t.Fatalf("published semantic search: %+v, %v", semantic, err)
	}
	var properties string
	if err := store.db.QueryRow(`SELECT properties FROM nodes WHERE project=? AND qualified_name=?`, "sample", "sample.Run").Scan(&properties); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(properties, `"signature":"()"`) || !strings.Contains(properties, `"complexity":1`) {
		t.Fatalf("method semantic properties missing: %s", properties)
	}
	rows, err := store.db.Query(`SELECT e.type,s.qualified_name,t.qualified_name,e.evidence
		FROM edges e JOIN nodes s ON s.id=e.source_id JOIN nodes t ON t.id=e.target_id
		WHERE e.type='CALLS' ORDER BY t.qualified_name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var calls [][3]string
	typeResolved := 0
	for rows.Next() {
		var kind, source, target, evidence string
		if err := rows.Scan(&kind, &source, &target, &evidence); err != nil {
			t.Fatal(err)
		}
		if evidence == "{}" || evidence == "null" {
			t.Fatalf("call lacks evidence: %s -> %s", source, target)
		}
		if strings.Contains(evidence, `"strategy":"go_types"`) && strings.Contains(evidence, `"confidence":1`) {
			typeResolved++
		}
		calls = append(calls, [3]string{kind, source, target})
	}
	if len(calls) != 1 || calls[0][2] != "sample.Helper" {
		t.Fatalf("unexpected calls: %+v", calls)
	}
	if typeResolved != 1 {
		t.Fatalf("expected local call to be type-resolved, got %d: %+v", typeResolved, calls)
	}
	var unresolvedTarget, unresolvedEvidence string
	if err := store.db.QueryRow(`SELECT target_text,evidence FROM unresolved_relationships
		WHERE project='sample' AND source_qn='sample.Run' AND type='CALLS'`).Scan(&unresolvedTarget, &unresolvedEvidence); err != nil {
		t.Fatal(err)
	}
	if unresolvedTarget != "external:fmt.Println" || !strings.Contains(unresolvedEvidence, `"strategy":"go_types"`) {
		t.Fatalf("unexpected unresolved relationship: %s %s", unresolvedTarget, unresolvedEvidence)
	}
	var dangling int
	if err := store.db.QueryRow(`SELECT count(*) FROM edges e LEFT JOIN nodes s ON s.id=e.source_id LEFT JOIN nodes d ON d.id=e.target_id WHERE s.id IS NULL OR d.id IS NULL`).Scan(&dangling); err != nil || dangling != 0 {
		t.Fatalf("dangling endpoints=%d, err=%v", dangling, err)
	}

	trace, err := store.Trace(context.Background(), api.TraceRequest{
		Project: "sample", Start: "sample.Run", Direction: "outgoing", EdgeTypes: []string{"CALLS"}, Depth: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(trace.Paths) != 1 || len(trace.Unresolved) != 1 || trace.Unresolved[0].TargetText != "external:fmt.Println" {
		t.Fatalf("unexpected trace: %+v", trace)
	}
	query, err := store.Query(context.Background(), api.QueryRequest{Project: "sample", Question: "Runner Run", Depth: 1, Budget: 200})
	if err != nil {
		t.Fatal(err)
	}
	if query.Text == "" || len(query.Nodes) == 0 || len(query.Edges) == 0 {
		t.Fatalf("unexpected query: %+v", query)
	}
	snippet, err := store.Snippet(context.Background(), api.SnippetRequest{
		Project: "sample", QualifiedName: "sample.Run",
	})
	if err != nil {
		t.Fatal(err)
	}
	if snippet.Location.StartLine != 9 || snippet.Code == "" {
		t.Fatalf("unexpected snippet: %+v", snippet)
	}
	architecture, err := store.Architecture(context.Background(), api.ArchitectureRequest{Project: "sample", Aspects: []string{"hotspots"}})
	if err != nil {
		t.Fatal(err)
	}
	if architecture.Summary == "" || len(architecture.Hotspots) == 0 {
		t.Fatalf("unexpected architecture: %+v", architecture)
	}
	impact, err := store.Impact(context.Background(), api.ImpactRequest{
		Project: "sample", Symbols: []string{"sample.Helper"}, EdgeTypes: []string{"CALLS"}, Depth: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if impact.Total != 1 || impact.Impacted[0].QualifiedName != "sample.Run" {
		t.Fatalf("unexpected impact: %+v", impact)
	}
}

func TestIndexGoGenerationIsContentDeterministic(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	repo := t.TempDir()
	writeFixtureFile(t, repo, "main.go", "package main\nfunc main() {}\n")
	first, err := IndexGoDevelopment(context.Background(), api.BuildRequest{RepoRoot: repo, Project: "sample"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(repo, "main.go"), first.Coverage.RecordedAt.Add(-1), first.Coverage.RecordedAt.Add(1)); err != nil {
		t.Fatal(err)
	}
	second, err := IndexGoDevelopment(context.Background(), api.BuildRequest{RepoRoot: repo, Project: "sample"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if first.Generation != second.Generation {
		t.Fatalf("generation depends on metadata: %s != %s", first.Generation, second.Generation)
	}
}

func TestGoFunctionEntryAndExportProperties(t *testing.T) {
	parsed := &parsedFile{rel: "main.go", fset: token.NewFileSet()}
	file, err := parser.ParseFile(parsed.fset, "main.go", "package main\nfunc main() {}\nfunc Exported() {}\nfunc hidden() {}\n", 0)
	if err != nil {
		t.Fatal(err)
	}
	properties := map[string]api.Properties{}
	for _, declaration := range file.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok {
			properties[function.Name.Name] = functionProperties(parsed, function)
		}
	}
	if properties["main"]["is_entry_point"] != true || properties["Exported"]["is_exported"] != true || properties["hidden"]["is_exported"] != false {
		t.Fatalf("function properties=%+v", properties)
	}
}

func TestIndexGoTypeAndUsageRelationships(t *testing.T) {
	repo := t.TempDir()
	writeFixtureFile(t, repo, "types.go", `package sample

type Reader interface { Read() }
type Base struct{}
type Concrete struct { Base }
func (Concrete) Read() {}
var Shared int
`)
	writeFixtureFile(t, repo, "use.go", `package sample
func Use() {
	Shared = 2
	_ = Shared
}
`)
	files := []string{"types.go", "use.go"}
	graph, _, _, err := parseGoFiles(repo, "sample", files)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"sample.Concrete\x00INHERITS\x00sample.Base":     false,
		"sample.Concrete\x00IMPLEMENTS\x00sample.Reader": false,
		"sample.Read\x00OVERRIDE\x00sample.Reader.Read":  false,
		"sample.Use\x00USAGE\x00sample.Shared":           false,
	}
	for _, edge := range graph.edges {
		key := edge.source + "\x00" + edge.kind + "\x00" + edge.target
		if _, ok := want[key]; ok {
			want[key] = true
			if edge.evidence == nil || edge.evidence.Strategy == "" {
				t.Errorf("edge lacks evidence: %s", key)
			}
		}
	}
	for key, found := range want {
		if !found {
			t.Errorf("missing relationship %q", key)
		}
	}
}

func TestArchitectureFindsDeterministicCallCycles(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	repo := t.TempDir()
	writeFixtureFile(t, repo, "cycles.go", `package sample
func Alpha() { Beta() }
func Beta() { Alpha() }
func Self() { Self() }
func Leaf() {}
`)
	result, err := IndexGoDevelopment(context.Background(), api.BuildRequest{RepoRoot: repo, Project: "sample"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenReadOnly(result.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	architecture, err := store.Architecture(context.Background(), api.ArchitectureRequest{Project: "sample", Aspects: []string{"cycles"}})
	if err != nil {
		t.Fatal(err)
	}
	// The pinned resolver suppresses direct self-call edges before graph
	// analysis, so only the two-node cycle is observable.
	if len(architecture.Cycles) != 1 {
		t.Fatalf("cycles=%v", architecture.Cycles)
	}
	got := [][]string{}
	for _, cycle := range architecture.Cycles {
		var names []string
		for _, node := range cycle {
			names = append(names, node.QualifiedName)
		}
		got = append(got, names)
	}
	if fmt.Sprint(got) != "[[sample.Alpha sample.Beta]]" {
		t.Fatalf("unexpected cycles: %v", got)
	}
}

func TestGoPackagesWithSameNameDoNotCollide(t *testing.T) {
	repo := t.TempDir()
	writeFixtureFile(t, repo, "one/shared/a.go", "package shared\nfunc Run() {}\n")
	writeFixtureFile(t, repo, "two/shared/b.go", "package shared\nfunc Run() {}\n")
	graph, _, _, err := parseGoFiles(repo, "sample", []string{"one/shared/a.go", "two/shared/b.go"})
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, node := range graph.nodes {
		found[node.QualifiedName] = true
	}
	for _, qn := range []string{"one.shared.Run", "two.shared.Run", "one.shared", "two.shared"} {
		if !found[qn] {
			t.Errorf("missing %s", qn)
		}
	}
}

func TestGoMethodsUsePinnedPackageQualifiedIdentity(t *testing.T) {
	repo := t.TempDir()
	writeFixtureFile(t, repo, "methods.go", `package sample
type Alpha struct{}
type Beta struct{}
func (Alpha) Run() {}
func (Beta) Run() {}
func Call(alpha Alpha, beta Beta) { alpha.Run(); beta.Run() }
`)
	graph, _, _, err := parseGoFiles(repo, "sample", []string{"methods.go"})
	if err != nil {
		t.Fatal(err)
	}
	targets := map[string]bool{}
	for _, edge := range graph.edges {
		if edge.source == "sample.Call" && edge.kind == "CALLS" {
			targets[edge.target] = edge.evidence != nil && edge.evidence.Strategy == "go_types" && edge.evidence.Confidence == 1
		}
	}
	if !targets["sample.Run"] {
		t.Errorf("missing package-qualified method call: %+v", targets)
	}
}

func writeFixtureFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGoBodyIdentifierTokensAreUniqueAndSourceOrdered(t *testing.T) {
	t.Parallel()
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "fixture.go", `package fixture
func planIncrementalChanges(oldPath, newPath string) { rename := oldPath; _ = rename; _ = newPath }
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	decl := file.Decls[0].(*ast.FuncDecl)
	if got, want := goBodyIdentifierTokens(decl.Body), "rename oldPath _ newPath"; got != want {
		t.Fatalf("body tokens = %q, want %q", got, want)
	}
}
