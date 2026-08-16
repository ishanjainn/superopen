package graph

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/redact"
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

var managedGraphExcludes = []string{
	".so/", "AGENTS.md", "CLAUDE.md", ".mcp.json", ".cursor/mcp.json",
	".claude/skills/so/", ".codex/skills/so/", ".cursor/skills/so/", ".cursor/rules/superopen.mdc",
	".gemini/skills/so/", ".github/skills/so/", ".opencode/skills/so/", ".pi/skills/so/",
	".openclaw/skills/so/", ".factory/skills/so/", ".trae/skills/so/", ".trae-cn/skills/so/",
	".hermes/skills/so/", ".kiro/skills/so/", ".devin/skills/so/", ".codebuddy/skills/so/",
	".kimi/skills/so/", ".kilo/skills/so/", ".aider/so/", ".agents/skills/so/",
}

// Result of a real Graphify graph build.
type Result struct {
	NodeCount     int    `json:"node_count"`
	EdgeCount     int    `json:"edge_count"`
	Engine        string `json:"engine"`
	EngineVersion string `json:"engine_version"`
	Status        string `json:"status"`
	Path          string `json:"path"`
	HasHTML       bool   `json:"has_html"`
	RunID         string `json:"run_id,omitempty"`
}

type BuildOptions struct {
	CodeOnly        bool
	SemanticBackend string
	Target          string
	Directed        bool
	NoCluster       bool
	ExtraArgs       []string
}

// Build runs the exact pinned Graphify runtime. It never publishes a degraded
// directory/file graph when extraction fails.
// Graphify output is always written to a temp dir (never repo-root graphify-out/),
// then ingested into .so/graph/. Any leftover repo-root graphify-out/ is removed.
func Build(repoRoot string, paths harness.Paths, codeOnly bool, semanticBackend string) (Result, error) {
	return BuildWithOptions(repoRoot, paths, BuildOptions{CodeOnly: codeOnly, SemanticBackend: semanticBackend, Target: repoRoot})
}

func BuildWithOptions(repoRoot string, paths harness.Paths, opts BuildOptions) (Result, error) {
	if err := os.MkdirAll(paths.GraphDir, 0o755); err != nil {
		return Result{}, err
	}
	// Always scrub stray Graphify folders at the repo root (from older runs / agent skills).
	defer removeGraphifyOut(repoRoot)

	bin, prefix, err := resolveGraphify()
	if err != nil {
		return Result{}, err
	}
	if opts.Target == "" {
		opts.Target = repoRoot
	}
	return buildWithGraphify(bin, prefix, repoRoot, paths, opts)
}

// RefreshAtomic builds graph artifacts in `.so/graph/.staging-*` and copies
// them into `.so/graph/` only after validation. Staging stays under graph/;
// leftover `.so/.graph-v2-*` siblings from older builds are swept on entry.
func RefreshAtomic(repoRoot string, paths harness.Paths, codeOnly bool, semanticBackend string) (Result, error) {
	return RefreshAtomicWithOptions(repoRoot, paths, BuildOptions{CodeOnly: codeOnly, SemanticBackend: semanticBackend, Target: repoRoot})
}

func RefreshAtomicWithOptions(repoRoot string, paths harness.Paths, opts BuildOptions) (Result, error) {
	if err := os.MkdirAll(paths.GraphDir, 0o755); err != nil {
		return Result{}, err
	}
	SweepStaleGraphWork(paths)
	tmp, err := os.MkdirTemp(paths.GraphDir, ".staging-")
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
	res, err := BuildWithOptions(repoRoot, tp, opts)
	if err != nil {
		recordRefreshFailure(paths, err)
		return res, err
	}
	if _, err := os.Stat(tp.GraphJSON); err != nil {
		return res, fmt.Errorf("graph refresh validation: %w", err)
	}
	if err := validateGraphArtifactsWithOptions(tp, opts.NoCluster); err != nil {
		return res, fmt.Errorf("graph refresh validation: %w", err)
	}
	if err := validateGraphQuery(repoRoot, tp); err != nil {
		return res, fmt.Errorf("graph query validation: %w", err)
	}
	if err := installGraphStaging(tmp, paths); err != nil {
		return res, err
	}
	res.Path = paths.GraphJSON
	return res, nil
}

func validateGraphQuery(repoRoot string, paths harness.Paths) error {
	bin, prefix, err := resolveGraphify()
	if err != nil {
		return err
	}
	args := append(append([]string{}, prefix...), "query", "superopen validation", "--graph", paths.GraphJSON, "--budget", "200")
	cmd := exec.Command(bin, args...)
	cmd.Dir = repoRoot
	cmd.Env = graphifyEnv(paths.GraphDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w (%s)", err, truncateOut(out, 600))
	}
	if len(strings.TrimSpace(string(out))) == 0 {
		return fmt.Errorf("empty query response")
	}
	return nil
}

func recordRefreshFailure(paths harness.Paths, buildErr error) {
	if _, err := os.Stat(paths.GraphJSON); err != nil {
		return
	}
	data, err := os.ReadFile(paths.GraphState)
	if err != nil {
		return
	}
	var state map[string]any
	if json.Unmarshal(data, &state) != nil {
		return
	}
	state["last_build_result"] = "failed"
	state["last_failure_at"] = time.Now().UTC()
	state["last_failure"] = truncateOut([]byte(buildErr.Error()), 400)
	out, err := json.MarshalIndent(state, "", "  ")
	if err == nil {
		_ = replaceBytes(paths.GraphState, append(out, '\n'))
	}
}

func recordSemanticContinuation(paths harness.Paths, run SemanticRun) error {
	data, err := os.ReadFile(paths.GraphState)
	if err != nil {
		return err
	}
	var state map[string]any
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	count := 0
	for kind, files := range run.ChangedFiles {
		if kind != "code" {
			count += len(files)
		}
	}
	state["status"] = "ready"
	state["last_build_result"] = "continuation_required"
	state["pending_semantic_run_id"] = run.RunID
	state["pending_semantic_source_count"] = count
	state["pending_source_file_fingerprint"] = run.SourceFingerprint
	state["last_failure"] = nil
	out, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return replaceBytes(paths.GraphState, append(out, '\n'))
}

func priorSemanticState(paths harness.Paths) semanticState {
	data, err := os.ReadFile(paths.GraphState)
	if err != nil {
		return semanticState{}
	}
	var state struct {
		Semantic semanticState `json:"semantic"`
	}
	_ = json.Unmarshal(data, &state)
	return state.Semantic
}

func replaceBytes(dst string, data []byte) error {
	tmp := dst + ".new"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// SweepStaleGraphWork removes abandoned atomic-swap directories that used to
// live as siblings of `.so/graph/` and leftover staging dirs inside it.
func SweepStaleGraphWork(paths harness.Paths) {
	parent := filepath.Dir(paths.GraphDir)
	if ents, err := os.ReadDir(parent); err == nil {
		for _, e := range ents {
			n := e.Name()
			if strings.HasPrefix(n, ".graph-v2-") || strings.HasPrefix(n, ".graph-publish-") || n == "graph.previous" || n == "graph.failed" {
				_ = os.RemoveAll(filepath.Join(parent, n))
			}
		}
	}
	if ents, err := os.ReadDir(paths.GraphDir); err == nil {
		for _, e := range ents {
			n := e.Name()
			if strings.HasSuffix(n, ".new") {
				_ = os.RemoveAll(filepath.Join(paths.GraphDir, n))
				continue
			}
			if strings.HasPrefix(n, ".staging-") {
				path := filepath.Join(paths.GraphDir, n)
				body, readErr := os.ReadFile(semanticManifest(path))
				var run SemanticRun
				_ = json.Unmarshal(body, &run)
				info, _ := e.Info()
				expired := info != nil && time.Since(info.ModTime()) > 7*24*time.Hour
				if readErr != nil || run.Status == "published" || run.Status == "superseded" || expired {
					_ = os.RemoveAll(path)
				}
			}
		}
	}
}

func installGraphStaging(tmp string, paths harness.Paths) error {
	if data, readErr := os.ReadFile(filepath.Join(tmp, "graph.json")); readErr == nil {
		if err := writeGraphVocabulary(tmp, data); err != nil {
			return err
		}
	}
	_ = os.Remove(filepath.Join(tmp, "cache", "last_query_stamp"))
	parent := filepath.Dir(paths.GraphDir)
	publishDir, err := os.MkdirTemp(parent, ".graph-publish-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(publishDir)

	// Preserve durable user-generated views, reflection output, and resumable
	// semantic runs. Canonical build artifacts are supplied by staging below.
	preserve := map[string]bool{
		"exports": true, "obsidian": true, "reflections": true, "wiki": true,
		".graphify_learning.json": true,
	}
	if current, readErr := os.ReadDir(paths.GraphDir); readErr == nil {
		for _, entry := range current {
			name := entry.Name()
			if !preserve[name] && !strings.HasPrefix(name, ".staging-") {
				continue
			}
			src := filepath.Join(paths.GraphDir, name)
			// A normal refresh stages inside graph/. Do not carry that temporary
			// directory into the published graph. Semantic run directories contain
			// a manifest and remain queryable after publication.
			if samePath(src, tmp) {
				if _, statErr := os.Stat(semanticManifest(tmp)); statErr != nil {
					continue
				}
			}
			dst := filepath.Join(publishDir, name)
			if entry.IsDir() {
				if err := copyDir(src, dst); err != nil {
					return err
				}
			} else if err := replaceFile(src, dst); err != nil {
				return err
			}
		}
	}

	entries, err := os.ReadDir(tmp)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".staging-") {
			continue
		}
		if entry.Name() == "semantic-run.json" || entry.Name() == ".graphify_ast.json" || entry.Name() == ".graphify_semantic.json" || entry.Name() == ".graphify_extract.json" || entry.Name() == ".graphify_detect.json" || strings.HasPrefix(entry.Name(), ".graphify_chunk_") {
			continue
		}
		src, dst := filepath.Join(tmp, entry.Name()), filepath.Join(publishDir, entry.Name())
		if entry.IsDir() {
			if !preserve[entry.Name()] {
				_ = os.RemoveAll(dst)
			}
			if err := copyDir(src, dst); err != nil {
				return err
			}
			continue
		}
		if err := replaceFile(src, dst); err != nil {
			return err
		}
	}
	for _, name := range []string{"cache", "converted", "wiki", "obsidian", "exports", "reflections"} {
		if err := os.MkdirAll(filepath.Join(publishDir, name), 0o755); err != nil {
			return err
		}
	}

	previous := filepath.Join(parent, "graph.previous")
	failed := filepath.Join(parent, "graph.failed")
	_ = os.RemoveAll(previous)
	_ = os.RemoveAll(failed)
	if _, statErr := os.Stat(paths.GraphDir); statErr == nil {
		if err := os.Rename(paths.GraphDir, previous); err != nil {
			return fmt.Errorf("preserve previous graph publication: %w", err)
		}
	}
	if err := os.Rename(publishDir, paths.GraphDir); err != nil {
		_ = os.Rename(paths.GraphDir, failed)
		_ = os.Rename(previous, paths.GraphDir)
		return fmt.Errorf("publish graph directory: %w", err)
	}
	_ = os.RemoveAll(previous)
	_ = os.RemoveAll(failed)
	return nil
}

func samePath(a, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	return errA == nil && errB == nil && filepath.Clean(aa) == filepath.Clean(bb)
}

func writeGraphVocabulary(graphDir string, data []byte) error {
	var raw struct {
		Nodes []map[string]any `json:"nodes"`
		Links []map[string]any `json:"links"`
		Edges []map[string]any `json:"edges"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("build graph vocabulary: %w", err)
	}
	terms := map[string]bool{}
	add := func(v any) {
		s, ok := v.(string)
		s = strings.TrimSpace(s)
		if ok && isManagedGraphSourcePath(s) {
			return
		}
		if ok && s != "" {
			terms[s] = true
		}
	}
	for _, n := range raw.Nodes {
		for _, key := range []string{"id", "label", "norm_label", "community_name", "source_file"} {
			add(n[key])
		}
	}
	links := append(raw.Links, raw.Edges...)
	for _, e := range links {
		for _, key := range []string{"relation", "context", "source_file"} {
			add(e[key])
		}
	}
	list := make([]string, 0, len(terms))
	for term := range terms {
		list = append(list, term)
	}
	sort.Strings(list)
	cache := filepath.Join(graphDir, "cache")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(cache, "vocab.txt"), []byte(strings.Join(list, "\n")+"\n"), 0o644)
}

// RecordQueryStamp records successful graph orientation. Failed commands must
// never call it. The embedded graph hash makes the stamp self-invalidating.
func RecordQueryStamp(repoRoot, command string) error {
	paths := harness.Resolve(repoRoot)
	hash := GraphHash(repoRoot)
	if hash == "" {
		return fmt.Errorf("graph state has no hash")
	}
	body, err := json.MarshalIndent(map[string]any{
		"graph_sha256": hash,
		"command":      command,
		"created_at":   time.Now().UTC(),
	}, "", "  ")
	if err != nil {
		return err
	}
	cache := filepath.Join(paths.GraphDir, "cache")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return err
	}
	return replaceBytes(filepath.Join(cache, "last_query_stamp"), append(body, '\n'))
}

func HasCurrentQueryStamp(repoRoot string) bool {
	return HasCurrentQueryStampSince(repoRoot, time.Time{})
}

// HasCurrentQueryStampSince prevents a query from an earlier coding session
// from satisfying graph orientation for a newly-started session.
func HasCurrentQueryStampSince(repoRoot string, since time.Time) bool {
	paths := harness.Resolve(repoRoot)
	body, err := os.ReadFile(filepath.Join(paths.GraphDir, "cache", "last_query_stamp"))
	if err != nil {
		return false
	}
	var stamp struct {
		GraphSHA256 string    `json:"graph_sha256"`
		CreatedAt   time.Time `json:"created_at"`
	}
	if json.Unmarshal(body, &stamp) != nil || stamp.GraphSHA256 == "" || stamp.GraphSHA256 != GraphHash(repoRoot) {
		return false
	}
	return since.IsZero() || (!stamp.CreatedAt.IsZero() && !stamp.CreatedAt.Before(since))
}

func replaceFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".new"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		previous := dst + ".previous"
		_ = os.Remove(previous)
		if _, statErr := os.Stat(dst); statErr == nil {
			if moveErr := os.Rename(dst, previous); moveErr != nil {
				_ = os.Remove(tmp)
				return err
			}
		}
		if err2 := os.Rename(tmp, dst); err2 != nil {
			_ = os.Rename(previous, dst)
			_ = os.Remove(tmp)
			return err2
		}
		_ = os.Remove(previous)
	}
	return nil
}

func validateGraphArtifacts(paths harness.Paths) error {
	return validateGraphArtifactsWithOptions(paths, false)
}

func validateGraphArtifactsWithOptions(paths harness.Paths, allowMissingHTML bool) error {
	data, err := os.ReadFile(paths.GraphJSON)
	if err != nil {
		return err
	}
	var graphObj map[string]json.RawMessage
	if json.Unmarshal(data, &graphObj) != nil || len(graphObj["_about"]) == 0 || len(graphObj["nodes"]) == 0 {
		return fmt.Errorf("graph.json is not a described graph object")
	}
	if err := validateNoHarnessSources(data); err != nil {
		return err
	}
	if err := validateManifestSources(filepath.Join(paths.GraphDir, "manifest.json")); err != nil {
		return err
	}
	for _, path := range []string{paths.GraphCorpus, paths.GraphState} {
		var obj map[string]json.RawMessage
		body, readErr := os.ReadFile(path)
		if readErr != nil || json.Unmarshal(body, &obj) != nil || len(obj["_about"]) == 0 {
			return fmt.Errorf("%s is missing valid _about metadata", filepath.Base(path))
		}
	}
	if !allowMissingHTML {
		html, err := os.ReadFile(paths.GraphHTML)
		if err != nil || !strings.HasPrefix(string(html), "<!-- Superopen Graphify visualization.") {
			return fmt.Errorf("graph.html is missing its leading description")
		}
	}
	return nil
}

func buildWithGraphify(bin string, prefix []string, repoRoot string, paths harness.Paths, opts BuildOptions) (Result, error) {
	const attempts = 3 // 1 try + 2 retries
	var lastErr error
	var lastRes Result
	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 {
			fmt.Fprintf(os.Stderr, "  graphify retry %d/%d…\n", attempt-1, attempts-1)
		}
		res, err := invokeGraphify(bin, prefix, repoRoot, paths, opts)
		lastRes = res
		if err == nil && res.Engine == "graphify" && (res.HasHTML || opts.NoCluster) {
			return res, nil
		}
		lastErr = err
		if err == nil && !res.HasHTML && !opts.NoCluster {
			lastErr = fmt.Errorf("graphify produced graph.json but no community graph.html")
		}
	}
	removeGraphifyOut(repoRoot)
	if lastErr == nil {
		lastErr = fmt.Errorf("Graphify produced no valid graph")
	}
	return lastRes, fmt.Errorf("Graphify %s extraction failed after %d attempts: %w", PinnedVersion, attempts, lastErr)
}

// runGraphifyExtract invokes `graphify extract` with --out pointing at a temp directory
// so Graphify never creates <repo>/graphify-out/.
func invokeGraphify(bin string, prefix []string, repoRoot string, paths harness.Paths, opts BuildOptions) (Result, error) {
	tmp, err := os.MkdirTemp("", "so-graphify-*")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(tmp)

	args := append([]string{}, prefix...)
	args = append(args, "extract", opts.Target, "--out", tmp)
	for _, exclude := range managedGraphExcludes {
		args = append(args, "--exclude", exclude)
	}
	if opts.CodeOnly {
		args = append(args, "--code-only")
	} else if opts.SemanticBackend != "" && opts.SemanticBackend != "agent" && opts.SemanticBackend != "none" {
		args = append(args, "--backend", opts.SemanticBackend)
	}
	args = append(args, opts.ExtraArgs...)

	cmd := exec.Command(bin, args...)
	cmd.Dir = tmp
	cmd.Env = graphifyEnvWithBackend(srcDirForEnv(tmp), opts.SemanticBackend)
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
	if _, err := sanitizeManagedGraphArtifacts(srcDir); err != nil {
		return Result{}, err
	}
	if opts.Directed {
		if err := rebuildDirectedGraph(opts.Target, srcDir); err != nil {
			return Result{}, err
		}
	}
	if opts.NoCluster {
		return ingestFromDir(srcDir, paths, semanticForBuild(opts.CodeOnly, opts.SemanticBackend))
	}

	// Newer graphify extract writes graph.json only; cluster + HTML are separate steps.
	labelBackend := opts.SemanticBackend
	if opts.CodeOnly || labelBackend == "agent" || labelBackend == "none" {
		labelBackend = ""
	}
	if err := finalizeGraphifyArtifacts(bin, prefix, tmp, srcDir, labelBackend); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: graphify communities/html: %v\n", err)
		fmt.Fprintln(os.Stderr, "  Tip: so graph rebuild")
		res, ingestErr := ingestFromDir(srcDir, paths, semanticForBuild(opts.CodeOnly, opts.SemanticBackend))
		if ingestErr != nil {
			return Result{}, ingestErr
		}
		res.HasHTML = false
		return res, fmt.Errorf("%w", err)
	}

	return ingestFromDir(srcDir, paths, semanticForBuild(opts.CodeOnly, opts.SemanticBackend))
}

func rebuildDirectedGraph(root, outDir string) error {
	python, err := graphifyPython()
	if err != nil {
		return err
	}
	rootJSON, _ := json.Marshal(root)
	outJSON, _ := json.Marshal(outDir)
	script := fmt.Sprintf(`import json
from pathlib import Path
from graphify.build import build_from_json
from graphify.export import to_json
root=Path(%s);out=Path(%s);extract=out/'.graphify_extract.json'
raw=json.loads(extract.read_text()) if extract.exists() else json.loads((out/'graph.json').read_text())
if 'edges' not in raw: raw['edges']=raw.get('links',[])
G=build_from_json(raw,root=root,directed=True)
if not to_json(G,{},out/'graph.json'): raise SystemExit('directed graph shrink guard refused output')`, string(rootJSON), string(outJSON))
	if _, err := runPython(context.Background(), python, root, outDir, script); err != nil {
		return fmt.Errorf("Graphify directed build: %w", err)
	}
	return nil
}

func srcDirForEnv(parent string) string { return filepath.Join(parent, graphifyOutName) }

func graphifyEnv(outDir string) []string {
	return graphifyEnvWithBackend(outDir, "")
}

// graphifyEnvWithBackend prevents Graphify's provider auto-detection from
// turning ambient credentials into an unrequested API call. Credentials remain
// available only after Superopen has selected an explicit backend.
func graphifyEnvWithBackend(outDir, backend string) []string {
	blocked := map[string]bool{}
	if backend == "" || backend == "agent" || backend == "none" {
		for _, key := range []string{
			"GEMINI_API_KEY", "GOOGLE_API_KEY", "MOONSHOT_API_KEY", "ANTHROPIC_API_KEY",
			"OPENAI_API_KEY", "DEEPSEEK_API_KEY", "AZURE_OPENAI_API_KEY", "AZURE_OPENAI_ENDPOINT",
			"AWS_PROFILE", "AWS_REGION", "AWS_DEFAULT_REGION", "OLLAMA_BASE_URL", "OLLAMA_HOST",
		} {
			blocked[key] = true
		}
	}
	env := make([]string, 0, len(os.Environ())+2)
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if !blocked[key] {
			env = append(env, item)
		}
	}
	if outDir != "" {
		abs, _ := filepath.Abs(outDir)
		env = append(env, "GRAPHIFY_OUT="+abs)
	}
	// Prefer community-aggregated HTML for large repos (export html uses this).
	if os.Getenv("GRAPHIFY_VIZ_NODE_LIMIT") == "" {
		env = append(env, "GRAPHIFY_VIZ_NODE_LIMIT=5000")
	}
	return env
}

const finalizeAttempts = 3 // 1 try + 2 retries

// finalizeGraphifyArtifacts runs Graphify cluster + HTML export so communities
// (LEGEND) come from Graphify itself - never synthesized by the Superopen UI.
func finalizeGraphifyArtifacts(bin string, prefix []string, workDir, srcDir, backend string) error {
	var lastErr error
	for attempt := 1; attempt <= finalizeAttempts; attempt++ {
		if attempt > 1 {
			fmt.Fprintf(os.Stderr, "  graphify community/html retry %d/%d…\n", attempt-1, finalizeAttempts-1)
		}
		if err := runGraphifyClusterHTML(bin, prefix, workDir, srcDir, backend); err != nil {
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

func runGraphifyClusterHTML(bin string, prefix []string, workDir, srcDir, backend string) error {
	// Let Graphify name communities (hub labels; LLM when configured).
	// Do NOT pass --no-label then fall back to export-without-labels - that
	// ships const LEGEND = [] and empty community chrome.
	clusterArgs := append([]string{}, prefix...)
	clusterArgs = append(clusterArgs, "cluster-only", "--graph", filepath.Join(srcDir, "graph.json"))
	if backend == "" {
		clusterArgs = append(clusterArgs, "--no-label")
	} else {
		clusterArgs = append(clusterArgs, "--backend="+backend)
	}
	cmd := exec.Command(bin, clusterArgs...)
	cmd.Dir = workDir
	cmd.Env = graphifyEnvWithBackend(srcDir, backend)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  cluster-only: %s\n", truncateOut(out, 200))
		// Still try export if graph.json + sidecars exist.
	} else {
		fmt.Fprintf(os.Stderr, "  cluster-only: %s\n", truncateOut(out, 120))
	}

	htmlPath := filepath.Join(srcDir, "graph.html")
	if ok, _ := htmlHasGraphifyCommunities(htmlPath); ok {
		return nil
	}

	exportArgs := append([]string{}, prefix...)
	exportArgs = append(exportArgs, "export", "html", "--graph", filepath.Join(srcDir, "graph.json"))
	cmd = exec.Command(bin, exportArgs...)
	cmd.Dir = workDir
	cmd.Env = graphifyEnv(srcDir)
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

func truncateOut(b []byte, n int) string {
	s := strings.TrimSpace(redact.StringFull(string(b)))
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// ingestFromDir preserves the full Graphify artifact set in .so/graph/.
func ingestFromDir(srcDir string, paths harness.Paths, semantic semanticState) (Result, error) {
	src := filepath.Join(srcDir, "graph.json")
	data, err := os.ReadFile(src)
	if err != nil {
		return Result{}, fmt.Errorf("read graph.json: %w", err)
	}
	if err := validateNoHarnessSources(data); err != nil {
		return Result{}, err
	}
	if err := copyDir(srcDir, paths.GraphDir); err != nil {
		return Result{}, err
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
	_ = writeGraphState(paths, data, nodes, edges, "ready", semantic)
	return Result{NodeCount: nodes, EdgeCount: edges, Engine: "graphify", EngineVersion: PinnedVersion, Status: "ready", Path: paths.GraphJSON, HasHTML: hasHTML}, nil
}

func validateNoHarnessSources(data []byte) error {
	var raw struct {
		Nodes []map[string]any `json:"nodes"`
		Links []map[string]any `json:"links"`
		Edges []map[string]any `json:"edges"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	items := append(raw.Nodes, raw.Links...)
	items = append(items, raw.Edges...)
	for _, item := range items {
		source, _ := item["source_file"].(string)
		if isManagedGraphSourcePath(source) {
			return fmt.Errorf("structural graph contains forbidden managed source %q", source)
		}
	}
	return nil
}

func validateManifestSources(path string) error {
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(body, &entries); err != nil {
		return fmt.Errorf("manifest.json is invalid: %w", err)
	}
	for source := range entries {
		if isManagedGraphSourcePath(source) {
			return fmt.Errorf("manifest.json contains forbidden managed source %q", source)
		}
	}
	return nil
}

// sanitizeManagedGraphArtifacts enforces the trust boundary before clustering
// and HTML generation. Graphify exclusions remain defense-in-depth, but exact
// dotfiles and legacy manifests cannot leak into a newly-published graph.
func sanitizeManagedGraphArtifacts(dir string) (bool, error) {
	changed := false
	graphPath := filepath.Join(dir, "graph.json")
	body, err := os.ReadFile(graphPath)
	if err == nil {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(body, &raw); err != nil {
			return false, err
		}
		var nodes []map[string]any
		if err := json.Unmarshal(raw["nodes"], &nodes); err != nil {
			return false, err
		}
		removed := map[string]bool{}
		keptNodes := nodes[:0]
		for _, node := range nodes {
			source, _ := node["source_file"].(string)
			if isManagedGraphSourcePath(source) {
				if id, _ := node["id"].(string); id != "" {
					removed[id] = true
				}
				changed = true
				continue
			}
			keptNodes = append(keptNodes, node)
		}
		raw["nodes"], _ = json.Marshal(keptNodes)
		for _, key := range []string{"edges", "links"} {
			if len(raw[key]) == 0 {
				continue
			}
			var edges []map[string]any
			if err := json.Unmarshal(raw[key], &edges); err != nil {
				return false, err
			}
			kept := edges[:0]
			for _, edge := range edges {
				sourceFile, _ := edge["source_file"].(string)
				sourceID, _ := edge["source"].(string)
				targetID, _ := edge["target"].(string)
				if isManagedGraphSourcePath(sourceFile) || removed[sourceID] || removed[targetID] {
					changed = true
					continue
				}
				kept = append(kept, edge)
			}
			raw[key], _ = json.Marshal(kept)
		}
		if len(raw["hyperedges"]) > 0 {
			var hyperedges []map[string]any
			if err := json.Unmarshal(raw["hyperedges"], &hyperedges); err != nil {
				return false, err
			}
			kept := hyperedges[:0]
			for _, hyperedge := range hyperedges {
				sourceFile, _ := hyperedge["source_file"].(string)
				drop := isManagedGraphSourcePath(sourceFile)
				if members, ok := hyperedge["nodes"].([]any); ok {
					for _, member := range members {
						id, _ := member.(string)
						drop = drop || removed[id]
					}
				}
				if drop {
					changed = true
					continue
				}
				kept = append(kept, hyperedge)
			}
			raw["hyperedges"], _ = json.Marshal(kept)
		}
		if changed {
			out, err := json.MarshalIndent(raw, "", "  ")
			if err != nil {
				return false, err
			}
			if err := os.WriteFile(graphPath, append(out, '\n'), 0o644); err != nil {
				return false, err
			}
		}
	} else if !os.IsNotExist(err) {
		return false, err
	}

	manifestPath := filepath.Join(dir, "manifest.json")
	manifestBody, err := os.ReadFile(manifestPath)
	if err == nil {
		var entries map[string]json.RawMessage
		if err := json.Unmarshal(manifestBody, &entries); err != nil {
			return false, err
		}
		manifestChanged := false
		for source := range entries {
			if isManagedGraphSourcePath(source) {
				delete(entries, source)
				manifestChanged = true
			}
		}
		if manifestChanged {
			out, err := json.MarshalIndent(entries, "", "  ")
			if err != nil {
				return false, err
			}
			if err := os.WriteFile(manifestPath, append(out, '\n'), 0o644); err != nil {
				return false, err
			}
			changed = true
		}
	} else if !os.IsNotExist(err) {
		return false, err
	}
	return changed, nil
}

func isManagedGraphSourcePath(source string) bool {
	slash := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(strings.TrimSpace(source))), "./")
	if slash == "" || slash == "." {
		return false
	}
	for _, excluded := range managedGraphExcludes {
		excluded = strings.TrimSuffix(filepath.ToSlash(excluded), "/")
		if slash == excluded || strings.HasPrefix(slash, excluded+"/") ||
			strings.HasSuffix(slash, "/"+excluded) || strings.Contains(slash, "/"+excluded+"/") {
			return true
		}
	}
	return false
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
		out, err := os.Create(target)
		if err != nil {
			_ = in.Close()
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeOutErr := out.Close()
		closeInErr := in.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeOutErr != nil {
			return closeOutErr
		}
		return closeInErr
	})
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

type semanticState struct {
	Required        bool   `json:"required"`
	Backend         string `json:"backend"`
	TotalChunks     int    `json:"total_chunks"`
	CompletedChunks int    `json:"completed_chunks"`
}

func codeSemanticState(paths harness.Paths) semanticState {
	return semanticState{Required: false, Backend: "none"}
}

func semanticForBuild(codeOnly bool, backend string) semanticState {
	if codeOnly {
		return semanticState{Required: false, Backend: "none"}
	}
	if backend == "" {
		backend = "none"
	}
	return semanticState{Required: true, Backend: backend}
}

func writeGraphState(paths harness.Paths, graphData []byte, nodes, edges int, status string, semantic semanticState) error {
	sum := sha256.Sum256(graphData)
	repoSum := sha256.Sum256([]byte(filepath.Clean(paths.RepoRoot)))
	sourceSum := sourceFingerprint(paths.RepoRoot)
	now := time.Now().UTC()
	state := map[string]any{
		"_about": map[string]string{
			"purpose":   "Graph freshness and build metadata used to decide whether a refresh is necessary.",
			"authority": "runtime state", "updated_by": "successful atomic graph refresh",
		},
		"schema_version": 3, "status": status, "engine": "graphify", "engine_version": PinnedVersion,
		"run_id":                 fmt.Sprintf("graph-%d", now.UnixNano()),
		"repository_fingerprint": fmt.Sprintf("%x", repoSum[:]), "source_file_fingerprint": sourceSum,
		"started_at": now, "completed_at": now, "semantic": semantic,
		"capabilities": CapabilityState(),
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
		if isManagedGraphRelativePath(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, statErr := d.Info()
		if statErr == nil {
			_, _ = fmt.Fprintf(h, "%s\x00%d\x00%d\n", filepath.ToSlash(rel), info.Size(), info.ModTime().UnixNano())
		}
		return nil
	})
	return fmt.Sprintf("%x", h.Sum(nil))
}

func SourceFingerprint(repoRoot string) string { return sourceFingerprint(repoRoot) }

// isHarnessSource is the shared structural-graph trust boundary. Graphify may
// discover files created while a build is running, so every Go-side evidence
// check independently rejects .so regardless of detector behavior.
func isHarnessSource(repoRoot, source string) bool {
	if strings.TrimSpace(source) == "" {
		return false
	}
	p := filepath.Clean(source)
	if !filepath.IsAbs(p) {
		p = filepath.Join(repoRoot, p)
	}
	rel, err := filepath.Rel(filepath.Clean(repoRoot), p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return isManagedGraphRelativePath(rel)
}

func isManagedGraphRelativePath(rel string) bool {
	rel = strings.TrimPrefix(filepath.ToSlash(filepath.Clean(rel)), "./")
	for _, excluded := range managedGraphExcludes {
		excluded = strings.TrimSuffix(filepath.ToSlash(excluded), "/")
		if rel == excluded || strings.HasPrefix(rel, excluded+"/") {
			return true
		}
	}
	return false
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

// Query shells to the pinned Graphify runtime. Legacy stub graphs are rejected.
func Query(repoRoot, question string) (string, error) {
	return QueryWithArgs(repoRoot, question, nil)
}

func QueryWithArgs(repoRoot, question string, extra []string) (string, error) {
	graphPath := filepath.Join(repoRoot, ".so", "graph", "graph.json")
	_, _ = runtimestate.TouchIfStale(repoRoot, "graph_query", 0)
	if err := ValidateQueryableGraph(repoRoot); err != nil {
		return "", err
	}
	bin, prefix, err := resolveGraphify()
	if err != nil {
		return "", err
	}
	args := append(append([]string{}, prefix...), "query", question, "--graph", graphPath)
	args = append(args, extra...)
	cmd := exec.Command(bin, args...)
	cmd.Dir = repoRoot
	cmd.Env = graphifyEnv(filepath.Dir(graphPath))
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("graphify query: %w (%s)", err, truncateOut(ee.Stderr, 800))
		}
		return "", fmt.Errorf("graphify query: %w", err)
	}
	if len(strings.TrimSpace(string(out))) == 0 {
		return "", fmt.Errorf("Graphify returned an empty query result")
	}
	if err := RecordQueryStamp(repoRoot, "query"); err != nil {
		return "", err
	}
	return annotateQuery(repoRoot, question, redact.StringFull(string(out))), nil
}

func annotateQuery(repoRoot, question, answer string) string {
	paths := harness.Resolve(repoRoot)
	items, err := memory.NewStore(paths).ListGraphOutcomes()
	if err != nil {
		return answer
	}
	hash := GraphHash(repoRoot)
	q := strings.ToLower(question)
	notes := []string{}
	for _, item := range items {
		iq := strings.ToLower(item.Question)
		if iq != q && !strings.Contains(q, iq) && !strings.Contains(iq, q) {
			continue
		}
		state := item.Outcome
		if item.GraphSHA256 != hash {
			state = "stale"
		}
		if item.Outcome == "corrected" {
			state = "contested"
		}
		notes = append(notes, fmt.Sprintf("- %s: %s", state, item.AnswerSummary))
	}
	if len(notes) == 0 {
		return answer
	}
	return answer + "\nGraph outcome overlay:\n" + strings.Join(notes, "\n") + "\n"
}

func ValidateQueryableGraph(repoRoot string) error {
	paths := harness.Resolve(repoRoot)
	data, err := os.ReadFile(paths.GraphJSON)
	if err != nil {
		return fmt.Errorf("no graph available: run `so graph rebuild`: %w", err)
	}
	var g struct {
		Nodes []map[string]any `json:"nodes"`
		Edges []map[string]any `json:"edges"`
		Links []map[string]any `json:"links"`
	}
	if err := json.Unmarshal(data, &g); err != nil || len(g.Nodes) == 0 {
		return fmt.Errorf("invalid or empty graph: run `so graph rebuild`")
	}
	if err := validateNoHarnessSources(data); err != nil {
		return fmt.Errorf("graph contains Superopen harness artifacts: run `so graph rebuild`: %w", err)
	}
	state, readErr := os.ReadFile(paths.GraphState)
	if readErr != nil {
		return fmt.Errorf("graph state is missing: run `so graph rebuild`")
	}
	var s map[string]any
	if json.Unmarshal(state, &s) != nil {
		return fmt.Errorf("graph state is invalid: run `so graph rebuild`")
	}
	if s["source"] == "stub" {
		return fmt.Errorf("legacy stub graph is not queryable: run `so graph rebuild`")
	}
	if s["schema_version"] != float64(3) || s["engine"] != "graphify" || s["engine_version"] != PinnedVersion {
		return fmt.Errorf("graph was not built by pinned Graphify %s: run `so graph rebuild`", PinnedVersion)
	}
	return nil
}

func ExistingResult(paths harness.Paths) (Result, bool) {
	if err := ValidateQueryableGraph(paths.RepoRoot); err != nil {
		return Result{}, false
	}
	data, err := os.ReadFile(paths.GraphJSON)
	if err != nil {
		return Result{}, false
	}
	nodes, edges := countGraph(data)
	if nodes == 0 {
		return Result{}, false
	}
	_, htmlErr := os.Stat(paths.GraphHTML)
	return Result{NodeCount: nodes, EdgeCount: edges, Engine: "graphify", EngineVersion: PinnedVersion, Status: "ready", Path: paths.GraphJSON, HasHTML: htmlErr == nil}, true
}

// ExistingSemanticResult returns an existing graph only when its published
// state proves that semantic extraction completed. A valid AST-only graph must
// never satisfy a later full-semantic initialization request.
func ExistingSemanticResult(paths harness.Paths) (Result, bool) {
	result, ok := ExistingResult(paths)
	if !ok {
		return Result{}, false
	}
	b, err := os.ReadFile(paths.GraphState)
	if err != nil {
		return Result{}, false
	}
	var state struct {
		Status   string        `json:"status"`
		Semantic semanticState `json:"semantic"`
	}
	if json.Unmarshal(b, &state) != nil || state.Status != "ready" || !state.Semantic.Required || state.Semantic.Backend == "none" {
		return Result{}, false
	}
	return result, true
}

func PendingSemanticRunID(paths harness.Paths) string {
	body, err := os.ReadFile(paths.GraphState)
	if err != nil {
		return ""
	}
	var state struct {
		LastBuildResult string `json:"last_build_result"`
		RunID           string `json:"pending_semantic_run_id"`
	}
	if json.Unmarshal(body, &state) != nil || state.LastBuildResult != "continuation_required" {
		return ""
	}
	return state.RunID
}

func ValidateNodes(graphPath string, ids []string) error {
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
	known := map[string]bool{}
	for _, n := range raw.Nodes {
		id, _ := n["id"].(string)
		known[id] = true
	}
	for _, id := range ids {
		if !known[id] {
			return fmt.Errorf("cited graph node does not exist: %s", id)
		}
	}
	return nil
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
