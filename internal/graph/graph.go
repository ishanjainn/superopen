package graph

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/retrieve"
	"github.com/ishanjainn/superopen/internal/runtimestate"
)

var graphAbout = map[string]string{
	"purpose":    "Repository code graph used by Graphify and so graph query.",
	"authority":  "derived from repository source files",
	"updated_by": "background graph refresh after sessions that changed files",
}

const graphHTMLComment = "<!-- Superopen Graphify visualization. Derived from graph.json for the Sessions UI and safe to regenerate. -->\n<!-- Updated by successful atomic graph refreshes. -->\n"

const graphifyOutName = "graphify-out"

// Result of a graph build.
type Result struct {
	NodeCount int    `json:"node_count"`
	EdgeCount int    `json:"edge_count"`
	Source    string `json:"source"` // graphify | stub
	Path      string `json:"path"`
	HasHTML   bool   `json:"has_html"`
}

// EnsureTool installs graphifyy via uv when `graphify` is not resolvable.
// Safe to call from `so init` / `so install`; no-op when already available.
func EnsureTool() error {
	if _, err := resolveGraphifyBin(); err == nil {
		fmt.Println("graphify: available")
		return nil
	}
	uv, err := exec.LookPath("uv")
	if err != nil {
		return fmt.Errorf("graphify not found; install uv (https://docs.astral.sh/uv/) then: uv tool install graphifyy")
	}
	fmt.Println("Installing graphifyy via uv tool install…")
	cmd := exec.Command(uv, "tool", "install", "graphifyy")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("uv tool install graphifyy: %w", err)
	}
	if _, err := resolveGraphifyBin(); err != nil {
		// uvx fallback still works even when ~/.local/bin is not on PATH.
		if _, err2 := exec.LookPath("uvx"); err2 == nil {
			fmt.Println("graphifyy installed (will run via uvx)")
			return nil
		}
		return fmt.Errorf("graphify installed but not found on PATH (add ~/.local/bin)")
	}
	fmt.Println("graphifyy installed")
	return nil
}

func resolveGraphifyBin() (string, error) {
	if bin, err := exec.LookPath("graphify"); err == nil {
		return bin, nil
	}
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".local", "bin", "graphify"),
		"/opt/homebrew/bin/graphify",
		"/usr/local/bin/graphify",
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("graphify not found")
}

// resolveGraphify returns (bin, prefixArgs) for running graphify commands.
// Prefers a real binary; falls back to `uvx --from graphifyy graphify`.
func resolveGraphify() (bin string, prefix []string, err error) {
	if b, err := resolveGraphifyBin(); err == nil {
		return b, nil, nil
	}
	if _, err := exec.LookPath("uvx"); err == nil {
		return "uvx", []string{"--from", "graphifyy", "graphify"}, nil
	}
	return "", nil, fmt.Errorf("graphify not available (install: uv tool install graphifyy)")
}

// Build runs Graphify when available, otherwise a lightweight stub graph.
// Graphify output is always written to a temp dir (never repo-root graphify-out/),
// then ingested into .so/graph/. Any leftover repo-root graphify-out/ is removed.
func Build(repoRoot string, paths harness.Paths, codeOnly bool, semanticBackend string) (Result, error) {
	if err := os.MkdirAll(paths.GraphDir, 0o755); err != nil {
		return Result{}, err
	}
	// Always scrub stray Graphify folders at the repo root (from older runs / agent skills).
	defer removeGraphifyOut(repoRoot)

	bin, prefix, err := resolveGraphify()
	if err != nil {
		fmt.Printf("  graphify unavailable (%v) - using stub graph\n", err)
		return buildStub(repoRoot, paths)
	}
	return buildWithGraphify(bin, prefix, repoRoot, paths, codeOnly, semanticBackend)
}

// RefreshAtomic builds a complete graph directory off to the side and swaps it
// into place only after graph.json and required UI artifacts validate.
func RefreshAtomic(repoRoot string, paths harness.Paths, codeOnly bool, semanticBackend string) (Result, error) {
	tmp, err := os.MkdirTemp(filepath.Dir(paths.GraphDir), ".graph-v2-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(tmp)
	tp := paths
	tp.GraphDir = tmp
	tp.GraphJSON = filepath.Join(tmp, "graph.json")
	tp.GraphCorpus = filepath.Join(tmp, "corpus.json")
	tp.GraphHTML = filepath.Join(tmp, "graph.html")
	tp.GraphState = filepath.Join(tmp, "state.json")
	res, err := Build(repoRoot, tp, codeOnly, semanticBackend)
	if err != nil {
		return res, err
	}
	if _, err := os.Stat(tp.GraphJSON); err != nil {
		return res, fmt.Errorf("graph refresh validation: %w", err)
	}
	if err := validateGraphArtifacts(tp); err != nil {
		return res, fmt.Errorf("graph refresh validation: %w", err)
	}
	backup := paths.GraphDir + ".previous"
	_ = os.RemoveAll(backup)
	if _, err := os.Stat(paths.GraphDir); err == nil {
		if err := os.Rename(paths.GraphDir, backup); err != nil {
			return res, err
		}
	}
	if err := os.Rename(tmp, paths.GraphDir); err != nil {
		_ = os.Rename(backup, paths.GraphDir)
		return res, err
	}
	if _, err := Query(repoRoot, "superopen validation"); err != nil {
		failed := paths.GraphDir + ".failed"
		_ = os.RemoveAll(failed)
		_ = os.Rename(paths.GraphDir, failed)
		_ = os.Rename(backup, paths.GraphDir)
		_ = os.RemoveAll(failed)
		return res, fmt.Errorf("graph query validation: %w", err)
	}
	_ = os.RemoveAll(backup)
	res.Path = paths.GraphJSON
	return res, nil
}

func validateGraphArtifacts(paths harness.Paths) error {
	data, err := os.ReadFile(paths.GraphJSON)
	if err != nil {
		return err
	}
	var graphObj map[string]json.RawMessage
	if json.Unmarshal(data, &graphObj) != nil || len(graphObj["_about"]) == 0 || len(graphObj["nodes"]) == 0 {
		return fmt.Errorf("graph.json is not a described graph object")
	}
	for _, path := range []string{paths.GraphCorpus, paths.GraphState} {
		var obj map[string]json.RawMessage
		body, readErr := os.ReadFile(path)
		if readErr != nil || json.Unmarshal(body, &obj) != nil || len(obj["_about"]) == 0 {
			return fmt.Errorf("%s is missing valid _about metadata", filepath.Base(path))
		}
	}
	html, err := os.ReadFile(paths.GraphHTML)
	if err != nil || !strings.HasPrefix(string(html), "<!-- Superopen Graphify visualization.") {
		return fmt.Errorf("graph.html is missing its leading description")
	}
	return nil
}

func buildWithGraphify(bin string, prefix []string, repoRoot string, paths harness.Paths, codeOnly bool, semanticBackend string) (Result, error) {
	const attempts = 3 // 1 try + 2 retries
	var lastErr error
	var lastRes Result
	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 {
			fmt.Printf("  graphify retry %d/%d…\n", attempt-1, attempts-1)
		}
		res, err := invokeGraphify(bin, prefix, repoRoot, paths, codeOnly, semanticBackend)
		lastRes = res
		if err == nil && res.Source == "graphify" && res.HasHTML {
			return res, nil
		}
		lastErr = err
		if err == nil && !res.HasHTML {
			lastErr = fmt.Errorf("graphify produced graph.json but no community graph.html")
		}
		// Semantic extract needs an API key; fall back to local AST-only rather than stub.
		if !codeOnly && attempt == attempts {
			fmt.Println("  semantic graph failed - retrying AST-only (--code-only)…")
			if res2, err2 := invokeGraphify(bin, prefix, repoRoot, paths, true, ""); err2 == nil && res2.Source == "graphify" && res2.HasHTML {
				return res2, nil
			} else if err2 != nil {
				lastErr = err2
				lastRes = res2
			} else if res2.Source == "graphify" {
				lastRes = res2
				lastErr = fmt.Errorf("graphify AST-only produced no community graph.html")
			}
		}
	}
	removeGraphifyOut(repoRoot)
	if lastRes.Source == "graphify" {
		fmt.Printf("  graphify communities/html failed after retries (%v)\n", lastErr)
		fmt.Println("  Tip: so graph rebuild")
		fmt.Println("       or: graphify cluster-only . && graphify export html --graph .so/graph/graph.json")
		return lastRes, nil
	}
	fmt.Printf("  graphify extract failed after retries (%v) - using stub graph\n", lastErr)
	fmt.Println("  Tip: run `so graph rebuild` or `graphify cluster-only .` then `graphify export html --graph .so/graph/graph.json`")
	return buildStub(repoRoot, paths)
}

// runGraphifyExtract invokes `graphify extract` with --out pointing at a temp directory
// so Graphify never creates <repo>/graphify-out/.
func invokeGraphify(bin string, prefix []string, repoRoot string, paths harness.Paths, codeOnly bool, semanticBackend string) (Result, error) {
	tmp, err := os.MkdirTemp("", "so-graphify-*")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(tmp)

	args := append([]string{}, prefix...)
	args = append(args, "extract", repoRoot, "--out", tmp)
	if codeOnly {
		args = append(args, "--code-only")
	} else if semanticBackend != "" && semanticBackend != "auto" {
		args = append(args, "--backend", semanticBackend)
	}

	cmd := exec.Command(bin, args...)
	cmd.Dir = tmp
	cmd.Env = graphifyEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Result{}, fmt.Errorf("graphify extract: %w (%s)", err, truncateOut(out, 240))
	}

	srcDir := filepath.Join(tmp, graphifyOutName)
	if _, err := os.Stat(filepath.Join(srcDir, "graph.json")); err != nil {
		if _, err2 := os.Stat(filepath.Join(tmp, "graph.json")); err2 == nil {
			srcDir = tmp
		} else {
			return Result{}, fmt.Errorf("graphify produced no graph.json: %s", truncateOut(out, 240))
		}
	}

	// Newer graphify extract writes graph.json only; cluster + HTML are separate steps.
	if err := finalizeGraphifyArtifacts(bin, prefix, tmp, srcDir); err != nil {
		fmt.Printf("  warning: graphify communities/html: %v\n", err)
		fmt.Println("  Tip: so graph rebuild")
		fmt.Println("       or: graphify cluster-only . && graphify export html --graph .so/graph/graph.json")
		res, ingestErr := ingestFromDir(srcDir, paths)
		if ingestErr != nil {
			return Result{}, ingestErr
		}
		res.HasHTML = false
		return res, fmt.Errorf("%w", err)
	}

	return ingestFromDir(srcDir, paths)
}

func graphifyEnv() []string {
	env := os.Environ()
	// Prefer community-aggregated HTML for large repos (export html uses this).
	if os.Getenv("GRAPHIFY_VIZ_NODE_LIMIT") == "" {
		env = append(env, "GRAPHIFY_VIZ_NODE_LIMIT=5000")
	}
	return env
}

const finalizeAttempts = 3 // 1 try + 2 retries

// finalizeGraphifyArtifacts runs Graphify cluster + HTML export so communities
// (LEGEND) come from Graphify itself - never synthesized by the Superopen UI.
func finalizeGraphifyArtifacts(bin string, prefix []string, workDir, srcDir string) error {
	var lastErr error
	for attempt := 1; attempt <= finalizeAttempts; attempt++ {
		if attempt > 1 {
			fmt.Printf("  graphify community/html retry %d/%d…\n", attempt-1, finalizeAttempts-1)
		}
		if err := runGraphifyClusterHTML(bin, prefix, workDir, srcDir); err != nil {
			lastErr = err
			continue
		}
		htmlPath := filepath.Join(srcDir, "graph.html")
		if ok, reason := htmlHasGraphifyCommunities(htmlPath); ok {
			return nil
		} else {
			lastErr = fmt.Errorf("graph.html communities incomplete: %s", reason)
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("graphify did not produce community legend")
	}
	return lastErr
}

func runGraphifyClusterHTML(bin string, prefix []string, workDir, srcDir string) error {
	// Let Graphify name communities (hub labels; LLM when configured).
	// Do NOT pass --no-label then fall back to export-without-labels - that
	// ships const LEGEND = [] and empty community chrome.
	clusterArgs := append([]string{}, prefix...)
	clusterArgs = append(clusterArgs, "cluster-only", workDir)
	cmd := exec.Command(bin, clusterArgs...)
	cmd.Dir = workDir
	cmd.Env = graphifyEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("  cluster-only: %s\n", truncateOut(out, 200))
		// Still try export if graph.json + sidecars exist.
	} else {
		fmt.Printf("  cluster-only: %s\n", truncateOut(out, 120))
	}

	htmlPath := filepath.Join(srcDir, "graph.html")
	if ok, _ := htmlHasGraphifyCommunities(htmlPath); ok {
		return nil
	}

	// Ensure label/analysis sidecars so `export html` can fill LEGEND.
	if err := ensureGraphifyCommunitySidecars(srcDir); err != nil {
		return fmt.Errorf("community sidecars: %w", err)
	}

	exportArgs := append([]string{}, prefix...)
	exportArgs = append(exportArgs, "export", "html", "--graph", filepath.Join(srcDir, "graph.json"))
	cmd = exec.Command(bin, exportArgs...)
	cmd.Dir = workDir
	cmd.Env = graphifyEnv()
	out, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("export html: %w (%s)", err, truncateOut(out, 240))
	}
	if st, err := os.Stat(htmlPath); err != nil || st.Size() == 0 {
		return fmt.Errorf("export html produced no graph.html (%s)", truncateOut(out, 200))
	}
	return nil
}

// htmlHasGraphifyCommunities requires Graphify's own LEGEND (non-empty).
func htmlHasGraphifyCommunities(htmlPath string) (bool, string) {
	data, err := os.ReadFile(htmlPath)
	if err != nil {
		return false, "missing graph.html"
	}
	if len(data) < 64 {
		return false, "empty graph.html"
	}
	s := string(data)
	if !strings.Contains(s, "const LEGEND") && !strings.Contains(s, "var LEGEND") {
		return false, "no LEGEND in graph.html"
	}
	// Empty array from Graphify when labels were missing.
	if strings.Contains(s, "const LEGEND = []") || strings.Contains(s, "const LEGEND=[];") ||
		strings.Contains(s, "var LEGEND = []") || strings.Contains(s, "var LEGEND=[];") {
		return false, "LEGEND is empty (Graphify needs cluster labels)"
	}
	return true, ""
}

// ensureGraphifyCommunitySidecars reconstructs .graphify_labels.json (and analysis
// communities when missing) from per-node community fields so export html can
// populate LEGEND the Graphify way.
func ensureGraphifyCommunitySidecars(srcDir string) error {
	graphPath := filepath.Join(srcDir, "graph.json")
	data, err := os.ReadFile(graphPath)
	if err != nil {
		return err
	}
	var raw struct {
		Nodes []map[string]any `json:"nodes"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	labels := map[string]string{}
	communities := map[string][]string{}
	for _, n := range raw.Nodes {
		cidRaw, ok := n["community"]
		if !ok || cidRaw == nil {
			continue
		}
		cid := fmt.Sprintf("%v", cidRaw)
		id, _ := n["id"].(string)
		if id == "" {
			continue
		}
		communities[cid] = append(communities[cid], id)
		if name, _ := n["community_name"].(string); strings.TrimSpace(name) != "" {
			labels[cid] = strings.TrimSpace(name)
		} else if _, ok := labels[cid]; !ok {
			labels[cid] = "Community " + cid
		}
	}
	if len(labels) == 0 {
		return fmt.Errorf("graph.json has no community fields - run graphify cluster-only first")
	}

	labelsPath := filepath.Join(srcDir, ".graphify_labels.json")
	if _, err := os.Stat(labelsPath); err != nil {
		b, err := json.MarshalIndent(labels, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(labelsPath, append(b, '\n'), 0o644); err != nil {
			return err
		}
	}

	analysisPath := filepath.Join(srcDir, ".graphify_analysis.json")
	if _, err := os.Stat(analysisPath); err != nil {
		analysis := map[string]any{
			"communities": communities,
			"cohesion":    map[string]any{},
			"gods":        []any{},
			"surprises":   []any{},
			"questions":   []any{},
		}
		b, err := json.MarshalIndent(analysis, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(analysisPath, append(b, '\n'), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func truncateOut(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// ingestFromDir copies Graphify artifacts into .so/graph/.
func ingestFromDir(srcDir string, paths harness.Paths) (Result, error) {
	src := filepath.Join(srcDir, "graph.json")
	data, err := os.ReadFile(src)
	if err != nil {
		return Result{}, fmt.Errorf("read graph.json: %w", err)
	}
	data = describeGraphJSON(data)
	if err := os.WriteFile(paths.GraphJSON, data, 0o644); err != nil {
		return Result{}, err
	}
	hasHTML := false
	if html, err := os.ReadFile(filepath.Join(srcDir, "graph.html")); err == nil && len(html) > 0 {
		if ok, _ := htmlHasGraphifyCommunities(filepath.Join(srcDir, "graph.html")); ok {
			if err := os.WriteFile(paths.GraphHTML, append([]byte(graphHTMLComment), html...), 0o644); err == nil {
				hasHTML = true
			}
		} else {
			// Don't serve Graphify HTML with an empty LEGEND - UI will prompt for CLI rebuild.
			_ = os.Remove(filepath.Join(paths.GraphDir, "graph.html"))
		}
	}
	nodes, edges := countGraph(data)
	repoRoot := filepath.Dir(paths.Root)
	_, _ = retrieve.Rebuild(repoRoot, paths)
	_ = writeGraphState(paths, data, nodes, edges, "graphify")
	return Result{NodeCount: nodes, EdgeCount: edges, Source: "graphify", Path: paths.GraphJSON, HasHTML: hasHTML}, nil
}

func removeGraphifyOut(repoRoot string) {
	_ = os.RemoveAll(filepath.Join(repoRoot, graphifyOutName))
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
}

type stubGraph struct {
	Nodes []map[string]any `json:"nodes"`
	Edges []map[string]any `json:"edges"`
}

func buildStub(repoRoot string, paths harness.Paths) (Result, error) {
	removeGraphifyOut(repoRoot)
	g := stubGraph{}
	_ = filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(repoRoot, path)
		base := filepath.Base(path)
		if d.IsDir() {
			if base == ".git" || base == "node_modules" || base == "vendor" || base == ".so" || base == graphifyOutName || base == "superopen" {
				return filepath.SkipDir
			}
			g.Nodes = append(g.Nodes, map[string]any{
				"id": rel, "label": base, "source_file": rel, "kind": "directory",
			})
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".rs", ".java", ".md", ".yaml", ".yml", ".json":
			g.Nodes = append(g.Nodes, map[string]any{
				"id": rel, "label": base, "source_file": rel, "kind": "file",
			})
			parent := filepath.Dir(rel)
			if parent != "." {
				g.Edges = append(g.Edges, map[string]any{
					"source": parent, "target": rel, "relation": "contains", "confidence": "EXTRACTED",
				})
			}
		}
		return nil
	})
	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return Result{}, err
	}
	data = describeGraphJSON(data)
	if err := os.WriteFile(paths.GraphJSON, data, 0o644); err != nil {
		return Result{}, err
	}
	stubHTML := graphHTMLComment + "<!doctype html><html><head><meta charset=\"utf-8\"><title>Superopen graph</title></head><body><p>Graphify visualization is unavailable; use <code>so graph query</code>.</p></body></html>\n"
	if err := os.WriteFile(paths.GraphHTML, []byte(stubHTML), 0o644); err != nil {
		return Result{}, err
	}
	_, _ = retrieve.Rebuild(repoRoot, paths)
	_ = writeGraphState(paths, data, len(g.Nodes), len(g.Edges), "stub")
	return Result{NodeCount: len(g.Nodes), EdgeCount: len(g.Edges), Source: "stub", Path: paths.GraphJSON, HasHTML: true}, nil
}

func describeGraphJSON(data []byte) []byte {
	var obj map[string]any
	if json.Unmarshal(data, &obj) != nil {
		return data
	}
	obj["_about"] = graphAbout
	out, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return data
	}
	return append(out, '\n')
}

func writeGraphState(paths harness.Paths, graphData []byte, nodes, edges int, source string) error {
	sum := sha256.Sum256(graphData)
	repoSum := sha256.Sum256([]byte(filepath.Clean(paths.RepoRoot)))
	sourceSum := sourceFingerprint(paths.RepoRoot)
	now := time.Now().UTC()
	state := map[string]any{
		"_about": map[string]string{
			"purpose":   "Graph freshness and build metadata used to decide whether a refresh is necessary.",
			"authority": "runtime state", "updated_by": "successful atomic graph refresh",
		},
		"schema_version": 2, "graphify_version": source,
		"repository_fingerprint": fmt.Sprintf("%x", repoSum[:]), "source_file_fingerprint": sourceSum,
		"started_at": now, "completed_at": now, "source": source,
		"graph_sha256": fmt.Sprintf("%x", sum[:]), "nodes": nodes, "edges": edges,
		"last_build_result": "success",
	}
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(paths.GraphState, append(b, '\n'), 0o644)
}

func sourceFingerprint(repoRoot string) string {
	h := sha256.New()
	_ = filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == ".so" || d.Name() == "node_modules" || d.Name() == "vendor") {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(repoRoot, path)
		info, statErr := d.Info()
		if statErr == nil {
			_, _ = fmt.Fprintf(h, "%s\x00%d\x00%d\n", filepath.ToSlash(rel), info.Size(), info.ModTime().UnixNano())
		}
		return nil
	})
	return fmt.Sprintf("%x", h.Sum(nil))
}

func countGraph(data []byte) (int, int) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return 0, 0
	}
	var nodes []any
	_ = json.Unmarshal(raw["nodes"], &nodes)
	var edges []any
	if err := json.Unmarshal(raw["edges"], &edges); err != nil || len(edges) == 0 {
		_ = json.Unmarshal(raw["links"], &edges) // NetworkX / Graphify format
	}
	return len(nodes), len(edges)
}

// Query shells to graphify when available, else hybrid harness retrieve + stub graph labels.
func Query(repoRoot, question string) (string, error) {
	graphPath := filepath.Join(repoRoot, ".so", "graph", "graph.json")
	// Graphify writes cache/last_query_stamp beside graph.json. Superopen keeps
	// that coordination marker in the consolidated OS runtime state instead.
	defer os.RemoveAll(filepath.Join(filepath.Dir(graphPath), "cache"))
	_, _ = runtimestate.TouchIfStale(repoRoot, "graph_query", 0)
	if bin, prefix, err := resolveGraphify(); err == nil {
		args := append(append([]string{}, prefix...), "query", question, "--graph", graphPath)
		cmd := exec.Command(bin, args...)
		out, err := cmd.CombinedOutput()
		if err == nil && len(strings.TrimSpace(string(out))) > 0 {
			return string(out), nil
		}
	}
	paths := ResolvePaths(repoRoot)
	var b strings.Builder
	if hits, err := QueryRetrieve(paths, question); err == nil && len(hits) > 0 {
		b.WriteString("Harness corpus:\n")
		for _, h := range hits {
			b.WriteString(fmt.Sprintf("- [%s] %s - %s\n", h.Kind, h.Path, truncateSnippet(h.Snippet, 120)))
		}
	}
	data, err := os.ReadFile(graphPath)
	if err != nil {
		if b.Len() > 0 {
			return b.String(), nil
		}
		return "", err
	}
	q := strings.ToLower(question)
	var g stubGraph
	_ = json.Unmarshal(data, &g)
	var nodeHits []string
	for _, n := range g.Nodes {
		label, _ := n["label"].(string)
		id, _ := n["id"].(string)
		if strings.Contains(strings.ToLower(label), q) || strings.Contains(strings.ToLower(id), q) {
			nodeHits = append(nodeHits, fmt.Sprintf("- %s (%s)", label, id))
		}
		if len(nodeHits) >= 20 {
			break
		}
	}
	if len(nodeHits) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("Graph nodes:\n")
		b.WriteString(strings.Join(nodeHits, "\n"))
	}
	if b.Len() == 0 {
		return "No matching nodes in local graph.", nil
	}
	return b.String(), nil
}

// ResolvePaths is a thin wrapper so graph can call harness without circular imports in tests.
func ResolvePaths(repoRoot string) harness.Paths {
	return harness.Resolve(repoRoot)
}

func QueryRetrieve(paths harness.Paths, question string) ([]retrieve.Hit, error) {
	return QueryRetrieveVendor(paths, question, "")
}

// QueryRetrieveVendor ranks harness corpus hits with optional session-vendor weighting.
func QueryRetrieveVendor(paths harness.Paths, question, vendor string) ([]retrieve.Hit, error) {
	opts := retrieve.SearchOptions{Limit: 15, Vendor: vendor}
	hits, err := retrieve.SearchWith(paths, question, opts)
	if err != nil || len(hits) > 0 {
		return hits, err
	}
	if _, err := retrieve.Rebuild(filepath.Dir(paths.Root), paths); err != nil {
		return nil, err
	}
	return retrieve.SearchWith(paths, question, opts)
}

func truncateSnippet(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
