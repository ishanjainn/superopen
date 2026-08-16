package graph

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/harness"
)

// This is the release gate for the pinned engine. It stays opt-in for normal
// unit runs because it invokes the real Python toolchain and can take minutes.
func TestDirectGraphifyParity(t *testing.T) {
	if os.Getenv("SUPEROPEN_GRAPHIFY_PARITY_TEST") != "1" {
		t.Skip("set SUPEROPEN_GRAPHIFY_PARITY_TEST=1 for direct Graphify parity")
	}
	if err := EnsureTool(); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package demo\nfunc Alpha(){ Beta() }\nfunc Beta(){}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths := harness.Resolve(root)
	if err := os.MkdirAll(paths.GraphDir, 0o755); err != nil {
		t.Fatal(err)
	}
	direct := filepath.Join(t.TempDir(), "direct")
	bin, prefix, err := resolveGraphify()
	if err != nil {
		t.Fatal(err)
	}
	args := append(append([]string{}, prefix...), "extract", root, "--out", direct, "--code-only")
	cmd := exec.CommandContext(context.Background(), bin, args...)
	cmd.Dir = root
	cmd.Env = graphifyEnv(direct)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("direct Graphify: %v: %s", err, out)
	}
	directGraph := filepath.Join(direct, "graph.json")
	if _, err := os.Stat(directGraph); err != nil {
		directGraph = filepath.Join(direct, graphifyOutName, "graph.json")
	}
	if _, err := RefreshAtomic(root, paths, true, "none"); err != nil {
		t.Fatal(err)
	}
	if got, want := normalizedStructure(t, paths.GraphJSON), normalizedStructure(t, directGraph); !equalStrings(got, want) {
		t.Fatalf("Superopen/direct Graphify structure differs\nso=%v\ndirect=%v", got, want)
	}
}

func TestPinnedGraphifyHelpSurface(t *testing.T) {
	if os.Getenv("SUPEROPEN_GRAPHIFY_PARITY_TEST") != "1" {
		t.Skip("set SUPEROPEN_GRAPHIFY_PARITY_TEST=1 for pinned help-surface parity")
	}
	if err := EnsureTool(); err != nil {
		t.Fatal(err)
	}
	python, err := graphifyPython()
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(python, "-m", "graphify", "--help").Output()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "    ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 0 {
			got[fields[0]] = true
		}
	}
	want := []string{"install", "uninstall", "path", "explain", "diagnose", "clone", "merge-driver", "merge-graphs", "add", "watch", "update", "cluster-only", "label", "query", "affected", "god-nodes", "save-result", "reflect", "check-update", "tree", "extract", "global", "benchmark", "export", "hook", "gemini", "cursor", "claude", "codebuddy", "codex", "opencode", "kilo", "aider", "copilot", "vscode", "claw", "droid", "trae", "trae-cn", "antigravity", "hermes", "kiro", "pi", "devin"}
	wantSet := map[string]bool{}
	for _, command := range want {
		wantSet[command] = true
		if !got[command] {
			t.Errorf("pinned Graphify help removed command %q", command)
		}
	}
	for command := range got {
		if !wantSet[command] {
			t.Errorf("pinned Graphify help added unreviewed command %q", command)
		}
	}
	if CommandSchema["provider"].Native != "provider" {
		t.Error("hidden pinned provider command is not represented in the facade")
	}
	for _, key := range []string{"vendor-installers", "git-hooks", "merge-driver"} {
		if CommandSchema[key].ExcludedReason == "" {
			t.Errorf("intentional exclusion %q lacks ownership reason", key)
		}
	}
}

func TestDirectedAndNoClusterBuildBehavior(t *testing.T) {
	if os.Getenv("SUPEROPEN_GRAPHIFY_PARITY_TEST") != "1" {
		t.Skip("set SUPEROPEN_GRAPHIFY_PARITY_TEST=1 for directed Graphify parity")
	}
	if err := EnsureTool(); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package demo\nfunc Alpha(){ Beta() }\nfunc Beta(){}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths := harness.Resolve(root)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	result, err := RefreshAtomicWithOptions(root, paths, BuildOptions{CodeOnly: true, SemanticBackend: "none", Target: root, Directed: true, NoCluster: true, ExtraArgs: []string{"--no-cluster"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.HasHTML {
		t.Fatal("--no-cluster unexpectedly published graph.html")
	}
	var raw struct {
		Directed bool `json:"directed"`
	}
	if err := json.Unmarshal(mustRead(t, paths.GraphJSON), &raw); err != nil {
		t.Fatal(err)
	}
	if !raw.Directed {
		t.Fatal("--directed did not produce a directed Graphify graph")
	}
}

func TestExistingSemanticResultRejectsCodeOnlyGraph(t *testing.T) {
	root := t.TempDir()
	paths := harness.Resolve(root)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	graphBody := []byte(`{"nodes":[{"id":"x"}],"links":[],"_about":{}}`)
	if err := os.WriteFile(paths.GraphJSON, graphBody, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeGraphState(paths, graphBody, 1, 0, "ready", semanticState{Required: false, Backend: "none"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := ExistingSemanticResult(paths); ok {
		t.Fatal("code-only graph satisfied semantic readiness")
	}
}

func TestPinnedExportSurfaceSmoke(t *testing.T) {
	if os.Getenv("SUPEROPEN_GRAPHIFY_PARITY_TEST") != "1" {
		t.Skip("set SUPEROPEN_GRAPHIFY_PARITY_TEST=1 for export smoke tests")
	}
	if err := EnsureTool(); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package demo\nfunc Alpha(){ Beta() }\nfunc Beta(){}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths := harness.Resolve(root)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	if _, err := RefreshAtomic(root, paths, true, "none"); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"export", "wiki", "--graph", paths.GraphJSON},
		{"export", "obsidian", "--graph", paths.GraphJSON},
		{"export", "svg", "--graph", paths.GraphJSON},
		{"export", "graphml", "--graph", paths.GraphJSON},
		{"export", "callflow-html", "--graph", paths.GraphJSON, "--output", filepath.Join(paths.GraphDir, "exports", "callflow.html")},
		{"export", "neo4j", "--graph", paths.GraphJSON},
		{"export", "falkordb", "--graph", paths.GraphJSON},
		{"tree", "--graph", paths.GraphJSON, "--output", filepath.Join(paths.GraphDir, "exports", "tree.html")},
	} {
		if result, err := Run(context.Background(), root, args...); err != nil {
			t.Fatalf("%v: %v stderr=%s", args, err, result.Stderr)
		}
	}
	if err := ExportCanvas(context.Background(), root, ""); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(paths.GraphDir, "wiki", "index.md"), filepath.Join(paths.GraphDir, "obsidian", "graph.canvas"),
		filepath.Join(paths.GraphDir, "graph.svg"), filepath.Join(paths.GraphDir, "graph.graphml"),
		filepath.Join(paths.GraphDir, "exports", "callflow.html"), filepath.Join(paths.GraphDir, "cypher.txt"),
		filepath.Join(paths.GraphDir, "exports", "tree.html"), filepath.Join(paths.GraphDir, "exports", "graph.canvas"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected export %s: %v", path, err)
		}
	}
}

func TestAgentSemanticBriefSmoke(t *testing.T) {
	if os.Getenv("SUPEROPEN_GRAPHIFY_PARITY_TEST") != "1" {
		t.Skip("set SUPEROPEN_GRAPHIFY_PARITY_TEST=1 for the agent semantic protocol smoke test")
	}
	if err := EnsureTool(); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package demo\nfunc Alpha(){}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "architecture.md"), []byte("Alpha owns request routing and calls the durable store."), 0o644); err != nil {
		t.Fatal(err)
	}
	run, err := StartSemanticRun(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "needs_agent_semantic" || len(run.Chunks) != 1 {
		t.Fatalf("unexpected semantic run: %+v", run)
	}
	briefs, err := SemanticBriefs(harness.Resolve(root), run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if len(briefs) != 1 || len(briefs[0]) < 100 {
		t.Fatalf("unexpected briefs: %d", len(briefs))
	}
}

func TestIncrementalAgentQueuesOnlySemanticChanges(t *testing.T) {
	if os.Getenv("SUPEROPEN_GRAPHIFY_PARITY_TEST") != "1" {
		t.Skip("set SUPEROPEN_GRAPHIFY_PARITY_TEST=1 for real incremental lifecycle")
	}
	if err := EnsureTool(); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package demo\nfunc Alpha(){}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths := harness.Resolve(root)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	if _, err := RefreshAtomic(root, paths, true, "none"); err != nil {
		t.Fatal(err)
	}
	baseHash := GraphHash(root)
	if err := os.WriteFile(filepath.Join(root, "architecture.md"), []byte("Alpha owns request routing."), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := UpdateAtomic(context.Background(), root, paths, false, "agent")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "needs_agent_semantic" || result.RunID == "" {
		t.Fatalf("expected resumable semantic continuation, got %+v", result)
	}
	if _, ok := ExistingResult(paths); !ok || GraphHash(root) == "" {
		t.Fatal("published graph must remain usable")
	}
	run, _, err := SemanticStatus(paths, result.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Kind != "incremental" || run.BaseGraphHash == "" || run.BaseGraphHash == baseHash || len(run.ChangedFiles["document"]) != 1 {
		t.Fatalf("bad incremental metadata: %+v", run)
	}
	state, err := os.ReadFile(paths.GraphState)
	if err != nil || !strings.Contains(string(state), `"last_build_result": "continuation_required"`) || !strings.Contains(string(state), `"status": "ready"`) {
		t.Fatalf("canonical ready/pending state missing: %s err=%v", state, err)
	}
	doc := run.Chunks[0].Files[0]
	payload, _ := json.Marshal(map[string]any{
		"nodes":      []map[string]any{{"id": "architecture_alpha", "label": "Alpha Architecture", "source_file": doc}, {"id": "architecture_router", "label": "Request Router", "source_file": doc}},
		"edges":      []map[string]any{{"source": "architecture_alpha", "target": "architecture_router", "relation": "owns", "confidence": "EXTRACTED", "confidence_score": 1.0, "source_file": doc}},
		"hyperedges": []any{}, "input_tokens": 0, "output_tokens": 0,
	})
	if err := ApplySemanticChunk(paths, run.RunID, run.Chunks[0].Number, payload); err != nil {
		t.Fatal(err)
	}
	if err := FinalizeSemantic(context.Background(), paths, run.RunID); err != nil {
		t.Fatal(err)
	}
	_, runDir, err := loadSemanticRun(paths, run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	analysisBody, err := os.ReadFile(filepath.Join(runDir, ".graphify_analysis.json"))
	if err != nil {
		t.Fatal(err)
	}
	var analysis struct {
		Communities map[string][]string `json:"communities"`
	}
	if err := json.Unmarshal(analysisBody, &analysis); err != nil {
		t.Fatal(err)
	}
	labels := map[string]string{}
	for id := range analysis.Communities {
		labels[id] = "Architecture"
	}
	labelBody, _ := json.Marshal(labels)
	if err := ApplyLabels(paths, run.RunID, labelBody); err != nil {
		t.Fatal(err)
	}
	published, err := PublishSemantic(context.Background(), paths, run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if published.Status != "ready" || !strings.Contains(string(mustRead(t, paths.GraphJSON)), "architecture_alpha") {
		t.Fatalf("incremental semantic graph did not publish: %+v", published)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func normalizedStructure(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Nodes []map[string]any `json:"nodes"`
		Edges []map[string]any `json:"edges"`
		Links []map[string]any `json:"links"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw.Edges) == 0 {
		raw.Edges = raw.Links
	}
	items := make([]string, 0, len(raw.Nodes)+len(raw.Edges))
	for _, n := range raw.Nodes {
		items = append(items, "n:"+stringValue(n["id"]))
	}
	for _, e := range raw.Edges {
		items = append(items, "e:"+stringValue(e["source"])+":"+stringValue(e["target"])+":"+stringValue(e["relation"]))
	}
	sort.Strings(items)
	return items
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
