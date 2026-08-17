package graph

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/retrieve"
	"github.com/ishanjainn/superopen/internal/tracestore"
)

type SemanticChunk struct {
	Number   int      `json:"number"`
	Files    []string `json:"files"`
	Done     bool     `json:"done"`
	Attempts int      `json:"attempts"`
}

type SemanticBrief struct {
	Number int    `json:"number"`
	Prompt string `json:"prompt"`
}

var structuralDocumentExtensions = map[string]bool{
	".md": true, ".mdx": true, ".qmd": true, ".markdown": true, ".rst": true, ".txt": true,
	".adoc": true, ".asciidoc": true,
}

type SemanticRun struct {
	SchemaVersion       int                  `json:"schema_version"`
	RunID               string               `json:"run_id"`
	Status              string               `json:"status"`
	RepoRoot            string               `json:"repo_root"`
	EngineVersion       string               `json:"engine_version"`
	Kind                string               `json:"kind"`
	BaseGraphHash       string               `json:"base_graph_hash,omitempty"`
	SourceFingerprint   string               `json:"source_fingerprint"`
	ChangedFiles        map[string][]string  `json:"changed_files,omitempty"`
	DeletedFiles        []string             `json:"deleted_files,omitempty"`
	PromptPath          string               `json:"prompt_path"`
	Options             SemanticStartOptions `json:"options"`
	GeneratedSources    map[string]string    `json:"generated_sources,omitempty"`
	Chunks              []SemanticChunk      `json:"chunks"`
	Excluded            []string             `json:"excluded,omitempty"`
	CreatedAt           time.Time            `json:"created_at"`
	UpdatedAt           time.Time            `json:"updated_at"`
	SemanticCompletedAt *time.Time           `json:"semantic_completed_at,omitempty"`
	LabelsCompletedAt   *time.Time           `json:"labels_completed_at,omitempty"`
}

type SemanticStartOptions struct {
	Target          string   `json:"target,omitempty"`
	WhisperModel    string   `json:"whisper_model,omitempty"`
	Directed        bool     `json:"directed,omitempty"`
	Deep            bool     `json:"deep,omitempty"`
	Force           bool     `json:"force,omitempty"`
	NoCluster       bool     `json:"no_cluster,omitempty"`
	Excludes        []string `json:"excludes,omitempty"`
	Resolution      float64  `json:"resolution,omitempty"`
	ExcludeHubs     float64  `json:"exclude_hubs,omitempty"`
	GoogleWorkspace bool     `json:"google_workspace,omitempty"`
	NoGitignore     bool     `json:"no_gitignore,omitempty"`
	PostgresDSN     string   `json:"postgres_dsn,omitempty"`
	Cargo           bool     `json:"cargo,omitempty"`
	AllowPartial    bool     `json:"allow_partial,omitempty"`
	Timing          bool     `json:"timing,omitempty"`
	DedupLLM        bool     `json:"dedup_llm,omitempty"`
	MaxWorkers      int      `json:"max_workers,omitempty"`
	TokenBudget     int      `json:"token_budget,omitempty"`
	MaxConcurrency  int      `json:"max_concurrency,omitempty"`
	APITimeout      float64  `json:"api_timeout,omitempty"`
	Incremental     bool     `json:"incremental,omitempty"`
}

type SemanticRebasedError struct{ RunID string }

func (e *SemanticRebasedError) Error() string {
	return "semantic sources changed; run rebased to " + e.RunID
}

func runDir(paths harness.Paths, id string) (string, error) {
	if id == "" || strings.ContainsAny(id, `/\\`) || !strings.HasPrefix(id, "run-") {
		return "", fmt.Errorf("invalid semantic run id")
	}
	return filepath.Join(paths.GraphDir, ".staging-"+id), nil
}

func semanticManifest(dir string) string { return filepath.Join(dir, "semantic-run.json") }

func loadSemanticRun(paths harness.Paths, id string) (SemanticRun, string, error) {
	dir, err := runDir(paths, id)
	if err != nil {
		return SemanticRun{}, "", err
	}
	b, err := os.ReadFile(semanticManifest(dir))
	if err != nil {
		return SemanticRun{}, "", err
	}
	var run SemanticRun
	if err := json.Unmarshal(b, &run); err != nil {
		return SemanticRun{}, "", err
	}
	if run.RunID != id || filepath.Clean(run.RepoRoot) != filepath.Clean(paths.RepoRoot) {
		return SemanticRun{}, "", fmt.Errorf("semantic run does not belong to this repository")
	}
	return run, dir, nil
}

func saveSemanticRun(dir string, run SemanticRun) error {
	run.UpdatedAt = time.Now().UTC()
	b, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(semanticManifest(dir), append(b, '\n'), 0o600); err != nil {
		return err
	}
	status := "building"
	if run.Status == "needs_agent_semantic" {
		status = "needs_agent_semantic"
	}
	completed := 0
	for _, c := range run.Chunks {
		if c.Done {
			completed++
		}
	}
	state := map[string]any{"schema_version": 3, "status": status, "engine": "graphify", "engine_version": PinnedVersion, "run_id": run.RunID, "repository_fingerprint": fingerprintPath(run.RepoRoot), "source_file_fingerprint": sourceFingerprint(run.RepoRoot), "nodes": 0, "edges": 0, "semantic": semanticState{Required: true, Backend: "agent", TotalChunks: len(run.Chunks), CompletedChunks: completed}, "capabilities": CapabilityState(), "started_at": run.CreatedAt, "last_build_result": ""}
	if graphData, readErr := os.ReadFile(filepath.Join(dir, "graph.json")); readErr == nil {
		nodes, edges := countGraph(graphData)
		sum := sha256.Sum256(graphData)
		state["graph_sha256"] = fmt.Sprintf("%x", sum[:])
		state["nodes"] = nodes
		state["edges"] = edges
	}
	out, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "state.json"), append(out, '\n'), 0o600)
}

func fingerprintPath(path string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(path)))
	return fmt.Sprintf("%x", sum[:])
}

func newRunID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "run-" + hex.EncodeToString(b[:])
}

func graphifyPython() (string, error) {
	st := Status()
	if st.Interpreter != "" {
		return st.Interpreter, nil
	}
	if st.Binary != "" {
		name := "python"
		if filepath.Ext(st.Binary) == ".exe" {
			name = "python.exe"
		}
		p := filepath.Join(filepath.Dir(st.Binary), name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("Graphify Python interpreter unavailable; use the managed runtime")
}

func extractionPromptPath() (string, error) {
	root, _, _, err := managedPaths()
	if err != nil {
		return "", err
	}
	roots := []string{root}
	if st := Status(); st.Binary != "" && !st.Managed {
		roots = append(roots, filepath.Dir(filepath.Dir(st.Binary)))
	}
	var found string
	for _, searchRoot := range roots {
		_ = filepath.WalkDir(searchRoot, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil || found != "" {
				return nil
			}
			if !d.IsDir() && d.Name() == "extraction-spec.md" && strings.Contains(filepath.ToSlash(path), "/skills/codex/references/") {
				found = path
			}
			return nil
		})
		if found != "" {
			break
		}
	}
	if found == "" {
		return "", fmt.Errorf("Graphify %s extraction prompt is missing", PinnedVersion)
	}
	return found, nil
}

func runPython(ctx context.Context, python, cwd, outDir, script string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, python, "-c", script)
	cmd.Dir = cwd
	cmd.Env = graphifyEnv(outDir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return stdout.Bytes(), fmt.Errorf("Graphify Python protocol: %w (%s)", err, truncateOut(stderr.Bytes(), 1200))
	}
	return stdout.Bytes(), nil
}

// StartSemanticRun performs Graphify detection and deterministic AST extraction,
// then creates directory-coherent semantic work chunks for the host agent.
func StartSemanticRun(ctx context.Context, repoRoot string) (SemanticRun, error) {
	return StartSemanticRunWithOptions(ctx, repoRoot, SemanticStartOptions{})
}

func StartIncrementalSemanticRun(ctx context.Context, repoRoot string) (SemanticRun, error) {
	return StartSemanticRunWithOptions(ctx, repoRoot, SemanticStartOptions{Incremental: true})
}

func StartSemanticRunWithOptions(ctx context.Context, repoRoot string, opts SemanticStartOptions) (SemanticRun, error) {
	if err := EnsureTool(); err != nil {
		return SemanticRun{}, err
	}
	paths := harness.Resolve(repoRoot)
	id := newRunID()
	dir, _ := runDir(paths, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return SemanticRun{}, err
	}
	if opts.Target == "" {
		opts.Target = repoRoot
	}
	target, err := filepath.Abs(opts.Target)
	if err != nil {
		return SemanticRun{}, err
	}
	relTarget, err := filepath.Rel(repoRoot, target)
	if err != nil || relTarget == ".." || strings.HasPrefix(relTarget, ".."+string(filepath.Separator)) {
		return SemanticRun{}, fmt.Errorf("semantic target must be inside the repository")
	}
	if info, statErr := os.Stat(target); statErr != nil || !info.IsDir() {
		return SemanticRun{}, fmt.Errorf("semantic target must be an existing directory: %s", target)
	}
	opts.Target = target
	if opts.Incremental {
		for _, name := range []string{"manifest.json", "graph.json", ".graphify_labels.json", ".graphify_analysis.json"} {
			src := filepath.Join(paths.GraphDir, name)
			if _, statErr := os.Stat(src); statErr == nil {
				if copyErr := replaceFile(src, filepath.Join(dir, name)); copyErr != nil {
					return SemanticRun{}, copyErr
				}
			}
		}
		if _, statErr := os.Stat(filepath.Join(paths.GraphDir, "cache")); statErr == nil {
			if copyErr := copyDir(filepath.Join(paths.GraphDir, "cache"), filepath.Join(dir, "cache")); copyErr != nil {
				return SemanticRun{}, copyErr
			}
		}
		if _, sanitizeErr := sanitizeManagedGraphArtifacts(dir); sanitizeErr != nil {
			return SemanticRun{}, sanitizeErr
		}
	}
	python, err := graphifyPython()
	if err != nil {
		return SemanticRun{}, err
	}
	prompt, err := extractionPromptPath()
	if err != nil {
		return SemanticRun{}, err
	}
	repoJSON, _ := json.Marshal(target)
	dirJSON, _ := json.Marshal(dir)
	promptJSON, _ := json.Marshal(prompt)
	structuralExtensions := make([]string, 0, len(structuralDocumentExtensions))
	for ext := range structuralDocumentExtensions {
		structuralExtensions = append(structuralExtensions, ext)
	}
	sort.Strings(structuralExtensions)
	structuralJSON, _ := json.Marshal(structuralExtensions)
	whisperModel := strings.TrimSpace(opts.WhisperModel)
	if whisperModel == "" {
		whisperModel = "base"
	}
	whisperJSON, _ := json.Marshal(whisperModel)
	incrementalPython := "False"
	if opts.Incremental {
		incrementalPython = "True"
	}
	excludesJSON, _ := json.Marshal(append(append([]string{}, managedGraphExcludes...), opts.Excludes...))
	managedJSON, _ := json.Marshal(managedGraphExcludes)
	googlePython := "False"
	if opts.GoogleWorkspace {
		googlePython = "True"
	}
	gitignorePython := "True"
	if opts.NoGitignore {
		gitignorePython = "False"
	}
	maxWorkersPython := "None"
	if opts.MaxWorkers > 0 {
		maxWorkersPython = strconv.Itoa(opts.MaxWorkers)
	}
	script := fmt.Sprintf(`import contextlib,json,os,sys
from pathlib import Path
from graphify.detect import detect,detect_incremental
from graphify.extract import collect_files, extract
from graphify.cache import check_semantic_cache
from graphify.transcribe import build_whisper_prompt,transcribe
root=Path(%s); out=Path(%s); out.mkdir(parents=True,exist_ok=True); incremental=%s; excludes=json.loads(%q); managed=json.loads(%q); structural_exts=set(json.loads(%q)); deep=%s
raw=detect_incremental(root,manifest_path=str(out/'manifest.json'),extra_excludes=excludes,google_workspace=%s,gitignore=%s) if incremental else None
d={'files':raw.get('new_files',{}),'all_files':raw.get('files',{}),'total_files':raw.get('new_total',0),'total_words':raw.get('total_words',0),'skipped_sensitive':raw.get('skipped_sensitive',[]),'deleted_files':raw.get('deleted_files',[])} if incremental else detect(root,extra_excludes=excludes,google_workspace=%s,gitignore=%s,cache_root=out)
def allowed_source(value):
 try:
  p=Path(value)
  p=(p if p.is_absolute() else root/p).resolve()
  rel=p.relative_to(root.resolve())
  key=rel.as_posix()
  return not any(key == x.rstrip('/') or key.startswith(x.rstrip('/') + '/') for x in managed)
 except (ValueError,OSError):
  return False
for kind,values in list(d.get('files',{}).items()):
 d['files'][kind]=[f for f in values if allowed_source(f)]
d['total_files']=sum(len(v) for v in d.get('files',{}).values())
(out/'.graphify_detect.json').write_text(json.dumps(d,ensure_ascii=False),encoding='utf-8')
files=[]
for f in d.get('files',{}).get('code',[]):
 p=Path(f); files.extend(collect_files(p) if p.is_dir() else [p])
for f in d.get('files',{}).get('document',[]):
 p=Path(f)
 if p.suffix.lower() in structural_exts: files.append(p)
if files:
 with contextlib.redirect_stdout(sys.stderr):
  r=extract(files,cache_root=root,max_workers=%s)
else:
 r={'nodes':[],'edges':[],'input_tokens':0,'output_tokens':0}
(out/'.graphify_ast.json').write_text(json.dumps(r,ensure_ascii=False),encoding='utf-8')
os.environ['GRAPHIFY_WHISPER_MODEL']=%s
prompt=build_whisper_prompt(r.get('nodes',[])) if r.get('nodes') else 'Use proper punctuation and paragraph breaks.'
transcripts=[];transcription_failures=[];generated_sources={}
for media in d.get('files',{}).get('video',[]):
 try:
  with contextlib.redirect_stdout(sys.stderr):
   transcript=str(transcribe(media,output_dir=out/'converted',initial_prompt=prompt));transcripts.append(transcript);generated_sources[transcript]=str(media)
 except Exception as exc:
  transcription_failures.append({'file':media,'reason':str(exc)})
d.setdefault('files',{}).setdefault('document',[]).extend(transcripts)
(out/'.graphify_transcripts.json').write_text(json.dumps(transcripts,ensure_ascii=False),encoding='utf-8')
for kind in ('document','paper'):
 for f in d.get('files',{}).get(kind,[]):
  try:
   p=Path(f).resolve(); p.relative_to((out/'converted').resolve())
   text=p.read_text(encoding='utf-8',errors='ignore')[:1000]
   import re
   m=re.search(r'^source_file:\s*["\']?([^"\'\n]+)',text,re.M)
   if m: generated_sources[str(f)]=m.group(1)
   else:
    m=re.search(r'converted from ([^>\n]+)',text)
    matches=list(root.rglob(m.group(1).strip())) if m else []
    if len(matches)==1: generated_sources[str(f)]=str(matches[0])
  except (ValueError,OSError): pass
semantic=[f for kind in ('document','paper','image') for f in d.get('files',{}).get(kind,[]) if (allowed_source(f) or str(f) in generated_sources) and (deep or str(f) in generated_sources or kind != 'document' or Path(f).suffix.lower() not in structural_exts)]
cn,ce,ch,uncached=check_semantic_cache(semantic,root=root,prompt_file=%s)
(out/'.graphify_cached.json').write_text(json.dumps({'nodes':cn,'edges':ce,'hyperedges':ch},ensure_ascii=False),encoding='utf-8')
print(json.dumps({'detect':d,'uncached':uncached,'generated_sources':generated_sources,'transcription_failures':transcription_failures,'ast_nodes':len(r.get('nodes',[])),'ast_edges':len(r.get('edges',[]))}))`, string(repoJSON), string(dirJSON), incrementalPython, string(excludesJSON), string(managedJSON), string(structuralJSON), pythonBool(opts.Deep), googlePython, gitignorePython, googlePython, gitignorePython, maxWorkersPython, string(whisperJSON), string(promptJSON))
	out, err := runPython(ctx, python, repoRoot, dir, script)
	if err != nil {
		return SemanticRun{}, err
	}
	var summary struct {
		Detect struct {
			Files            map[string][]string `json:"files"`
			DeletedFiles     []string            `json:"deleted_files"`
			SkippedSensitive []any               `json:"skipped_sensitive"`
		} `json:"detect"`
		Uncached              []string          `json:"uncached"`
		GeneratedSources      map[string]string `json:"generated_sources"`
		TranscriptionFailures []struct {
			File   string `json:"file"`
			Reason string `json:"reason"`
		} `json:"transcription_failures"`
	}
	if err := json.Unmarshal(out, &summary); err != nil {
		return SemanticRun{}, fmt.Errorf("decode Graphify detection: %w", err)
	}
	imageSet := map[string]bool{}
	for _, f := range summary.Detect.Files["image"] {
		imageSet[f] = true
	}
	files, images := []string{}, []string{}
	for _, f := range summary.Uncached {
		if imageSet[f] {
			images = append(images, f)
		} else {
			files = append(files, f)
		}
	}
	sort.Slice(files, func(i, j int) bool {
		di, dj := filepath.Dir(files[i]), filepath.Dir(files[j])
		if di == dj {
			return files[i] < files[j]
		}
		return di < dj
	})
	chunks := []SemanticChunk{}
	for len(files) > 0 {
		n := 22
		if len(files) < n {
			n = len(files)
		}
		chunk := append([]string{}, files[:n]...)
		files = files[n:]
		chunks = append(chunks, SemanticChunk{Number: len(chunks) + 1, Files: chunk})
	}
	for _, image := range images {
		chunks = append(chunks, SemanticChunk{Number: len(chunks) + 1, Files: []string{image}})
	}
	status := "needs_agent_semantic"
	if len(chunks) == 0 {
		status = "ready_to_finalize"
		_ = os.WriteFile(filepath.Join(dir, ".graphify_semantic.json"), []byte(`{"nodes":[],"edges":[],"hyperedges":[],"input_tokens":0,"output_tokens":0}`), 0o600)
	}
	excluded := []string{}
	for _, failure := range summary.TranscriptionFailures {
		excluded = append(excluded, failure.File+": "+failure.Reason)
	}
	for _, item := range summary.Detect.SkippedSensitive {
		excluded = append(excluded, fmt.Sprint(item))
	}
	kind := "full"
	baseHash := ""
	if opts.Incremental {
		kind = "incremental"
		baseHash = GraphHash(repoRoot)
	}
	run := SemanticRun{SchemaVersion: 3, RunID: id, Status: status, RepoRoot: repoRoot, EngineVersion: PinnedVersion, Kind: kind, BaseGraphHash: baseHash, SourceFingerprint: sourceFingerprint(repoRoot), ChangedFiles: summary.Detect.Files, DeletedFiles: summary.Detect.DeletedFiles, PromptPath: prompt, Options: opts, GeneratedSources: summary.GeneratedSources, Chunks: chunks, Excluded: excluded, CreatedAt: time.Now().UTC()}
	if err := saveSemanticRun(dir, run); err != nil {
		return SemanticRun{}, err
	}
	return run, nil
}

func SemanticBriefs(paths harness.Paths, id string) ([]SemanticBrief, error) {
	run, dir, err := loadSemanticRun(paths, id)
	if err != nil {
		return nil, err
	}
	if current := sourceFingerprint(run.RepoRoot); run.SourceFingerprint != "" && current != run.SourceFingerprint {
		return nil, rebaseSemanticRun(paths, run, dir)
	}
	prompt, err := os.ReadFile(run.PromptPath)
	if err != nil {
		return nil, err
	}
	briefs := []SemanticBrief{}
	for _, ch := range run.Chunks {
		if ch.Done {
			continue
		}
		p := string(prompt)
		files, _ := json.Marshal(ch.Files)
		p = strings.ReplaceAll(p, "FILE_LIST", string(files))
		p = strings.ReplaceAll(p, "CHUNK_NUM", fmt.Sprint(ch.Number))
		p = strings.ReplaceAll(p, "TOTAL_CHUNKS", fmt.Sprint(len(run.Chunks)))
		p = strings.ReplaceAll(p, "DEEP_MODE", strconv.FormatBool(run.Options.Deep))
		briefs = append(briefs, SemanticBrief{Number: ch.Number, Prompt: p})
	}
	return briefs, nil
}

func DiscardSemanticRun(paths harness.Paths, id string) error {
	run, dir, err := loadSemanticRun(paths, id)
	if err != nil {
		return err
	}
	if run.Status == "published" {
		return fmt.Errorf("semantic run %s is already published", id)
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	stateBody, stateErr := os.ReadFile(paths.GraphState)
	if stateErr != nil {
		return nil
	}
	var state map[string]any
	if json.Unmarshal(stateBody, &state) != nil || state["pending_semantic_run_id"] != id {
		return nil
	}
	if _, graphErr := os.Stat(paths.GraphJSON); graphErr != nil {
		return os.Remove(paths.GraphState)
	}
	delete(state, "pending_semantic_run_id")
	state["status"] = "ready"
	state["last_build_result"] = "success"
	updated, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return replaceBytes(paths.GraphState, append(updated, '\n'))
}

func SemanticStatus(paths harness.Paths, id string) (SemanticRun, string, error) {
	return loadSemanticRun(paths, id)
}

func ApplySemanticChunk(paths harness.Paths, id string, number int, data []byte) error {
	run, dir, err := loadSemanticRun(paths, id)
	if err != nil {
		return err
	}
	if current := sourceFingerprint(run.RepoRoot); run.SourceFingerprint != "" && current != run.SourceFingerprint {
		return rebaseSemanticRun(paths, run, dir)
	}
	idx := -1
	for i, c := range run.Chunks {
		if c.Number == number {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("chunk %d is not part of run", number)
	}
	if run.Chunks[idx].Attempts >= 3 && !run.Chunks[idx].Done {
		return fmt.Errorf("chunk %d exhausted its initial attempt and two retries", number)
	}
	fail := func(err error) error { run.Chunks[idx].Attempts++; _ = saveSemanticRun(dir, run); return err }
	var raw struct {
		Nodes        []map[string]any `json:"nodes"`
		Edges        []map[string]any `json:"edges"`
		Hyperedges   []map[string]any `json:"hyperedges"`
		InputTokens  int              `json:"input_tokens"`
		OutputTokens int              `json:"output_tokens"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fail(fmt.Errorf("chunk JSON: %w", err))
	}
	if memory.ContainsInjection(string(data)) {
		return fail(fmt.Errorf("chunk rejected by prompt-injection sentinel"))
	}
	allowed := map[string]bool{}
	for _, f := range run.Chunks[idx].Files {
		original, generated := run.GeneratedSources[f]
		if isHarnessSource(run.RepoRoot, f) && !generated {
			return fail(fmt.Errorf("chunk contains forbidden .so source %q", f))
		}
		allowed[filepath.Clean(f)] = true
		if generated {
			allowed[filepath.Clean(original)] = true
		}
	}
	remapSource := func(item map[string]any) {
		if sf, _ := item["source_file"].(string); sf != "" {
			if original, ok := run.GeneratedSources[sf]; ok {
				item["source_file"] = original
			}
		}
	}
	for _, n := range raw.Nodes {
		remapSource(n)
	}
	for _, e := range raw.Edges {
		remapSource(e)
	}
	for _, h := range raw.Hyperedges {
		remapSource(h)
	}
	ids := knownSemanticNodeIDs(dir)
	idPattern := regexp.MustCompile(`^[a-z0-9_]+$`)
	for _, n := range raw.Nodes {
		id, _ := n["id"].(string)
		sf, _ := n["source_file"].(string)
		if isHarnessSource(run.RepoRoot, sf) || !idPattern.MatchString(id) || !allowed[filepath.Clean(sf)] {
			return fail(fmt.Errorf("node has invalid id or source_file"))
		}
		ids[id] = true
	}
	for _, e := range raw.Edges {
		s, _ := e["source"].(string)
		t, _ := e["target"].(string)
		sf, _ := e["source_file"].(string)
		if isHarnessSource(run.RepoRoot, sf) || s == "" || t == "" || !ids[s] || !ids[t] || !allowed[filepath.Clean(sf)] || !normalizeConfidence(e) {
			return fail(fmt.Errorf("edge is not bound to this chunk's evidence or has invalid confidence"))
		}
	}
	for _, h := range raw.Hyperedges {
		members, _ := h["nodes"].([]any)
		sf, _ := h["source_file"].(string)
		if isHarnessSource(run.RepoRoot, sf) || len(members) < 3 || !allowed[filepath.Clean(sf)] || !normalizeConfidence(h) {
			return fail(fmt.Errorf("invalid hyperedge"))
		}
		for _, member := range members {
			node, _ := member.(string)
			if !ids[node] {
				return fail(fmt.Errorf("hyperedge references unknown node %q", node))
			}
		}
	}
	b, _ := json.MarshalIndent(raw, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf(".graphify_chunk_%02d.json", number)), append(b, '\n'), 0o600); err != nil {
		return err
	}
	run.Chunks[idx].Done = true
	run.Chunks[idx].Attempts++
	if allChunksDone(run) {
		run.Status = "ready_to_finalize"
	}
	return saveSemanticRun(dir, run)
}

func rebaseSemanticRun(paths harness.Paths, old SemanticRun, oldDir string) error {
	var replacement SemanticRun
	var err error
	if _, ok := ExistingResult(paths); ok {
		result, updateErr := UpdateAtomic(context.Background(), old.RepoRoot, paths, false, "agent")
		if updateErr != nil {
			return updateErr
		}
		if result.RunID == "" {
			return fmt.Errorf("sources changed but no semantic continuation remains after AST refresh")
		}
		replacement, _, err = loadSemanticRun(paths, result.RunID)
	} else {
		replacement, err = StartSemanticRunWithOptions(context.Background(), old.RepoRoot, old.Options)
	}
	if err != nil {
		return err
	}
	// UpdateAtomic classifies the changed corpus, while the original run remains
	// authoritative for user-selected extraction behavior.
	replacement.Options = old.Options
	replacement.Options.Incremental = replacement.Kind == "incremental"
	if replacementDir, dirErr := runDir(paths, replacement.RunID); dirErr == nil {
		if saveErr := saveSemanticRun(replacementDir, replacement); saveErr != nil {
			return saveErr
		}
	}
	old.Status = "superseded"
	_ = saveSemanticRun(oldDir, old)
	return &SemanticRebasedError{RunID: replacement.RunID}
}

func knownSemanticNodeIDs(dir string) map[string]bool {
	known := map[string]bool{}
	patterns := []string{filepath.Join(dir, ".graphify_ast.json"), filepath.Join(dir, ".graphify_cached.json")}
	chunks, _ := filepath.Glob(filepath.Join(dir, ".graphify_chunk_*.json"))
	patterns = append(patterns, chunks...)
	for _, path := range patterns {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var raw struct {
			Nodes []map[string]any `json:"nodes"`
		}
		if json.Unmarshal(b, &raw) != nil {
			continue
		}
		for _, n := range raw.Nodes {
			id, _ := n["id"].(string)
			if id != "" {
				known[id] = true
			}
		}
	}
	return known
}

func normalizeConfidence(item map[string]any) bool {
	kind, _ := item["confidence"].(string)
	score, ok := item["confidence_score"].(float64)
	if !ok || score < 0 || score > 1 {
		return false
	}
	switch kind {
	case "EXTRACTED":
		// The enum is authoritative. Models commonly pair EXTRACTED with a
		// high-but-not-exact score even though Graphify requires exactly 1.0.
		if score < .5 {
			return false
		}
		item["confidence_score"] = 1.0
		return true
	case "INFERRED":
		if score < .5 || score >= 1 {
			return false
		}
		// Graphify stores inferred confidence in discrete buckets. Accept a
		// model's continuous score and canonicalize it to the nearest bucket.
		buckets := [...]float64{.95, .85, .75, .65, .55}
		best := buckets[0]
		bestDistance := score - best
		if bestDistance < 0 {
			bestDistance = -bestDistance
		}
		for _, candidate := range buckets[1:] {
			distance := score - candidate
			if distance < 0 {
				distance = -distance
			}
			if distance < bestDistance {
				best, bestDistance = candidate, distance
			}
		}
		item["confidence_score"] = best
		return true
	case "AMBIGUOUS":
		return score >= .1 && score <= .3
	}
	return false
}
func allChunksDone(run SemanticRun) bool {
	for _, c := range run.Chunks {
		if !c.Done {
			return false
		}
	}
	return true
}

func pythonBool(value bool) string {
	if value {
		return "True"
	}
	return "False"
}

func FinalizeSemantic(ctx context.Context, paths harness.Paths, id string) error {
	run, dir, err := loadSemanticRun(paths, id)
	if err != nil {
		return err
	}
	if !allChunksDone(run) {
		return fmt.Errorf("semantic chunks are incomplete")
	}
	python, err := graphifyPython()
	if err != nil {
		return err
	}
	graphRoot := run.Options.Target
	if graphRoot == "" {
		graphRoot = run.RepoRoot
	}
	repoJSON, _ := json.Marshal(graphRoot)
	dirJSON, _ := json.Marshal(dir)
	promptJSON, _ := json.Marshal(run.PromptPath)
	allowedFiles := []string{}
	for _, chunk := range run.Chunks {
		allowedFiles = append(allowedFiles, chunk.Files...)
	}
	allowedJSON, _ := json.Marshal(allowedFiles)
	incrementalPython := "False"
	if run.Kind == "incremental" {
		incrementalPython = "True"
	}
	baseGraphJSON, _ := json.Marshal(paths.GraphJSON)
	deletedJSON, _ := json.Marshal(run.DeletedFiles)
	directedPython := pythonBool(run.Options.Directed)
	noClusterPython := pythonBool(run.Options.NoCluster)
	resolution := run.Options.Resolution
	if resolution <= 0 {
		resolution = 1
	}
	excludeHubsPython := "None"
	if run.Options.ExcludeHubs > 0 {
		excludeHubsPython = strconv.FormatFloat(run.Options.ExcludeHubs, 'f', -1, 64)
	}
	postgresJSON, _ := json.Marshal(run.Options.PostgresDSN)
	cargoPython := pythonBool(run.Options.Cargo)
	script := fmt.Sprintf(`import json,glob
from pathlib import Path
from graphify.build import build_from_json,build_merge
from graphify.cluster import cluster,score_all
from graphify.analyze import god_nodes,surprising_connections,suggest_questions
from graphify.report import generate
from graphify.export import to_json
from graphify.cache import save_semantic_cache
root=Path(%s);out=Path(%s);incremental=%s;base_graph=%s;deleted=json.loads(%q);directed=%s;no_cluster=%s;resolution=%s;exclude_hubs=%s;postgres=%s;use_cargo=%s
ast=json.loads((out/'.graphify_ast.json').read_text()); chunks=[json.loads(Path(p).read_text()) for p in sorted(glob.glob(str(out/'.graphify_chunk_*.json')))];cached=json.loads((out/'.graphify_cached.json').read_text()) if (out/'.graphify_cached.json').exists() else {'nodes':[],'edges':[],'hyperedges':[]}
sem={'nodes':cached.get('nodes',[])+sum((c.get('nodes',[]) for c in chunks),[]),'edges':cached.get('edges',[])+sum((c.get('edges',[]) for c in chunks),[]),'hyperedges':cached.get('hyperedges',[])+sum((c.get('hyperedges',[]) for c in chunks),[]),'input_tokens':sum(c.get('input_tokens',0) for c in chunks),'output_tokens':sum(c.get('output_tokens',0) for c in chunks)}
new={'nodes':sum((c.get('nodes',[]) for c in chunks),[]),'edges':sum((c.get('edges',[]) for c in chunks),[]),'hyperedges':sum((c.get('hyperedges',[]) for c in chunks),[])}
save_semantic_cache(new['nodes'],new['edges'],new['hyperedges'],root=root,allowed_source_files=%s,prompt_file=%s)
(out/'.graphify_semantic.json').write_text(json.dumps(sem,ensure_ascii=False))
seen={n['id'] for n in ast['nodes']};nodes=list(ast['nodes'])
for n in sem['nodes']:
 if n['id'] not in seen:nodes.append(n);seen.add(n['id'])
ex={'nodes':nodes,'edges':ast['edges']+sem['edges'],'hyperedges':sem['hyperedges'],'input_tokens':sem['input_tokens'],'output_tokens':sem['output_tokens']}
(lambda G: ex.update({'nodes':[{'id':n,**d} for n,d in G.nodes(data=True)],'edges':[dict(d,source=u,target=v) for u,v,d in G.edges(data=True)]}))(build_merge([ex],graph_path=base_graph,prune_sources=deleted or None,root=root,directed=directed)) if incremental else None
if postgres:
 from graphify.pg_introspect import introspect_postgres
 pg=introspect_postgres(postgres);ex['nodes']+=pg.get('nodes',[]);ex['edges']+=pg.get('edges',[])
if use_cargo:
 from graphify.cargo_introspect import introspect_cargo
 cg=introspect_cargo(root);ex['nodes']+=cg.get('nodes',[]);ex['edges']+=cg.get('edges',[])
(out/'.graphify_extract.json').write_text(json.dumps(ex,ensure_ascii=False))
G=build_from_json(ex,root=root,directed=directed)
if G.number_of_nodes()==0:raise SystemExit('empty graph')
cs={} if no_cluster else cluster(G,resolution=resolution,exclude_hubs_percentile=exclude_hubs);co={} if no_cluster else score_all(G,cs);labels={cid:'Community '+str(cid) for cid in cs};gods=god_nodes(G);sur=[] if no_cluster else surprising_connections(G,cs);qs=[] if no_cluster else suggest_questions(G,cs,labels);det=json.loads((out/'.graphify_detect.json').read_text())
if not to_json(G,cs,out/'graph.json',community_labels=labels):raise SystemExit('graph shrink guard refused publication')
(out/'.graphify_analysis.json').write_text(json.dumps({'communities':{str(k):v for k,v in cs.items()},'cohesion':{str(k):v for k,v in co.items()},'gods':gods,'surprises':sur,'questions':qs},ensure_ascii=False))
(out/'.graphify_labels.json').write_text(json.dumps({str(k):v for k,v in labels.items()}))
(out/'GRAPH_REPORT.md').write_text(generate(G,cs,co,labels,gods,sur,det,{'input':sem['input_tokens'],'output':sem['output_tokens']},str(root),suggested_questions=qs))`, string(repoJSON), string(dirJSON), incrementalPython, string(baseGraphJSON), string(deletedJSON), directedPython, noClusterPython, strconv.FormatFloat(resolution, 'f', -1, 64), excludeHubsPython, string(postgresJSON), cargoPython, string(allowedJSON), string(promptJSON))
	if _, err := runPython(ctx, python, graphRoot, dir, script); err != nil {
		return err
	}
	run.Status = "needs_agent_labels"
	if run.Options.NoCluster {
		run.Status = "ready_to_publish"
	}
	now := time.Now().UTC()
	run.SemanticCompletedAt = &now
	return saveSemanticRun(dir, run)
}

func LabelsBrief(paths harness.Paths, id string) (string, error) {
	run, dir, err := loadSemanticRun(paths, id)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(dir, ".graphify_analysis.json"))
	if err != nil {
		return "", err
	}
	return "Return one concise 2-5 word label for every community as a JSON object keyed by community id. Use only this Graphify analysis:\n" + string(b) + "\nRun: " + run.RunID, nil
}

func ApplyLabels(paths harness.Paths, id string, data []byte) error {
	run, dir, err := loadSemanticRun(paths, id)
	if err != nil {
		return err
	}
	var labels map[string]string
	if err := json.Unmarshal(data, &labels); err != nil {
		return err
	}
	if memory.ContainsInjection(string(data)) {
		return fmt.Errorf("labels rejected by prompt-injection sentinel")
	}
	var analysis struct {
		Communities map[string][]string `json:"communities"`
	}
	b, err := os.ReadFile(filepath.Join(dir, ".graphify_analysis.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &analysis); err != nil {
		return err
	}
	for id := range analysis.Communities {
		words := strings.Fields(labels[id])
		if len(words) < 1 || len(words) > 5 {
			return fmt.Errorf("community %s needs a concise label", id)
		}
	}
	out, _ := json.MarshalIndent(labels, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, ".graphify_labels.json"), append(out, '\n'), 0o600); err != nil {
		return err
	}
	python, err := graphifyPython()
	if err != nil {
		return err
	}
	graphRoot := run.Options.Target
	if graphRoot == "" {
		graphRoot = run.RepoRoot
	}
	repoJSON, _ := json.Marshal(graphRoot)
	dirJSON, _ := json.Marshal(dir)
	labelsJSON, _ := json.Marshal(labels)
	script := fmt.Sprintf(`import json
from pathlib import Path
from graphify.build import build_from_json
from graphify.analyze import suggest_questions
from graphify.report import generate
from graphify.export import to_json
root=Path(%s);out=Path(%s);labels={int(k):v for k,v in %s.items()};ex=json.loads((out/'.graphify_extract.json').read_text());det=json.loads((out/'.graphify_detect.json').read_text());an=json.loads((out/'.graphify_analysis.json').read_text());G=build_from_json(ex,root=root,directed=%s);cs={int(k):v for k,v in an['communities'].items()};co={int(k):v for k,v in an['cohesion'].items()};qs=suggest_questions(G,cs,labels)
if not to_json(G,cs,out/'graph.json',community_labels=labels):raise SystemExit('graph shrink guard refused labels')
(out/'GRAPH_REPORT.md').write_text(generate(G,cs,co,labels,an['gods'],an['surprises'],det,{'input':ex.get('input_tokens',0),'output':ex.get('output_tokens',0)},str(root),suggested_questions=qs))`, string(repoJSON), string(dirJSON), string(labelsJSON), pythonBool(run.Options.Directed))
	if _, err := runPython(context.Background(), python, graphRoot, dir, script); err != nil {
		return err
	}
	run.Status = "ready_to_publish"
	now := time.Now().UTC()
	run.LabelsCompletedAt = &now
	return saveSemanticRun(dir, run)
}

func PublishSemantic(ctx context.Context, paths harness.Paths, id string) (Result, error) {
	run, dir, err := loadSemanticRun(paths, id)
	if err != nil {
		return Result{}, err
	}
	if run.Status != "ready_to_publish" {
		return Result{}, fmt.Errorf("run is not ready to publish")
	}
	if current := sourceFingerprint(run.RepoRoot); current != run.SourceFingerprint {
		return Result{}, fmt.Errorf("sources changed during semantic run %s; refresh/rebase is required before publication", run.RunID)
	}
	if run.Kind == "incremental" && GraphHash(run.RepoRoot) != run.BaseGraphHash {
		return Result{}, fmt.Errorf("base graph changed during incremental run %s; refresh/rebase is required before publication", run.RunID)
	}
	python, pyErr := graphifyPython()
	if pyErr != nil {
		return Result{}, pyErr
	}
	repoJSON, _ := json.Marshal(run.RepoRoot)
	dirJSON, _ := json.Marshal(dir)
	manifestScript := fmt.Sprintf(`import json
from pathlib import Path
from datetime import datetime,timezone
from graphify.detect import save_manifest
root=Path(%s);out=Path(%s);det=json.loads((out/'.graphify_detect.json').read_text());ex=json.loads((out/'.graphify_extract.json').read_text());save_manifest(det.get('all_files',det.get('files',{})),root=root,scan_corpus={f for files in det.get('all_files',det.get('files',{})).values() for f in files});cost={'measurement':'unavailable','session_id':None,'run_id':%q,'phases':{'initialization':None,'semantic_extraction':{'input_tokens':None,'output_tokens':None,'cache_tokens':None,'cost':None,'measurement':'unavailable','payload_metadata':{'input_tokens':ex.get('input_tokens'),'output_tokens':ex.get('output_tokens')}},'labeling':None,'upgrade':None,'query':None,'coding_task':None},'amortized':{'successful_graph_assisted_tasks':0,'initialization_cost_per_successful_task':None}};(out/'cost.json').write_text(json.dumps(cost,indent=2))`, string(repoJSON), string(dirJSON), run.RunID)
	if _, err := runPython(ctx, python, run.RepoRoot, dir, manifestScript); err != nil {
		return Result{}, err
	}
	if err := writeHostSemanticCost(paths, dir, run); err != nil {
		return Result{}, err
	}
	bin, prefix, resolveErr := resolveGraphify()
	if resolveErr != nil {
		return Result{}, resolveErr
	}
	exportArgs := append(append([]string{}, prefix...), "export", "html", "--graph", filepath.Join(dir, "graph.json"))
	exportCmd := exec.CommandContext(ctx, bin, exportArgs...)
	exportCmd.Dir = run.RepoRoot
	exportCmd.Env = graphifyEnv(dir)
	if output, err := exportCmd.CombinedOutput(); err != nil {
		return Result{}, fmt.Errorf("Graphify HTML export: %w (%s)", err, truncateOut(output, 800))
	}
	data, err := os.ReadFile(filepath.Join(dir, "graph.json"))
	if err != nil {
		return Result{}, err
	}
	data = describeGraphJSON(data)
	if err := os.WriteFile(filepath.Join(dir, "graph.json"), data, 0o644); err != nil {
		return Result{}, err
	}
	if html, readErr := os.ReadFile(filepath.Join(dir, "graph.html")); readErr == nil && !strings.HasPrefix(string(html), graphHTMLComment) {
		_ = os.WriteFile(filepath.Join(dir, "graph.html"), append([]byte(graphHTMLComment), html...), 0o644)
	}
	nodes, edges := countGraph(data)
	if nodes == 0 {
		return Result{}, fmt.Errorf("refusing to publish an empty graph")
	}
	sp := paths
	sp.GraphDir = dir
	sp.GraphJSON = filepath.Join(dir, "graph.json")
	sp.GraphHTML = filepath.Join(dir, "graph.html")
	sp.GraphCorpus = filepath.Join(dir, "corpus.json")
	sp.GraphState = filepath.Join(dir, "state.json")
	_, _ = retrieve.Rebuild(run.RepoRoot, sp)
	if err := writeGraphState(sp, data, nodes, edges, "ready", semanticState{Required: true, Backend: "agent", TotalChunks: len(run.Chunks), CompletedChunks: len(run.Chunks)}); err != nil {
		return Result{}, err
	}
	if err := validateGraphArtifacts(sp); err != nil {
		return Result{}, err
	}
	if err := validateGraphQuery(run.RepoRoot, sp); err != nil {
		return Result{}, err
	}
	if err := installGraphStaging(dir, paths); err != nil {
		return Result{}, err
	}
	run.Status = "published"
	_ = saveSemanticRun(dir, run)
	return Result{NodeCount: nodes, EdgeCount: edges, Engine: "graphify", EngineVersion: PinnedVersion, Status: "ready", Path: paths.GraphJSON, HasHTML: true}, nil
}

type measuredUsage struct {
	InputTokens  *int64   `json:"input_tokens"`
	OutputTokens *int64   `json:"output_tokens"`
	CacheTokens  *int64   `json:"cache_tokens"`
	CostUSD      *float64 `json:"cost"`
	Measurement  string   `json:"measurement"`
}

func writeHostSemanticCost(paths harness.Paths, dir string, run SemanticRun) error {
	spans, err := tracestore.NewLocalJSONL(paths.TracesDir).Query(tracestore.QueryFilter{Since: run.CreatedAt, Until: time.Now().UTC()})
	if err != nil {
		spans = nil
	}
	sessionID := ""
	for _, span := range spans {
		joined := ""
		for _, key := range []string{"coding_agent.tool.command", "gen_ai.tool.call.arguments", "coding_agent.tool.arguments"} {
			joined += " " + span.Attributes[key]
		}
		if strings.Contains(joined, run.RunID) && span.SessionID != "" {
			if sessionID != "" && sessionID != span.SessionID {
				sessionID = ""
				break
			}
			sessionID = span.SessionID
		}
	}
	measurement := "unavailable"
	semanticUsage := measuredUsage{Measurement: measurement}
	labelUsage := measuredUsage{Measurement: measurement}
	if sessionID != "" && run.SemanticCompletedAt != nil {
		semanticUsage = measureUsage(spans, sessionID, run.CreatedAt, *run.SemanticCompletedAt)
		labelEnd := time.Now().UTC()
		if run.LabelsCompletedAt != nil {
			labelEnd = *run.LabelsCompletedAt
		}
		labelUsage = measureUsage(spans, sessionID, *run.SemanticCompletedAt, labelEnd)
	}
	cost := map[string]any{
		"measurement": semanticUsage.Measurement, "session_id": nil, "run_id": run.RunID,
		"phases":    map[string]any{"initialization": nil, "semantic_extraction": semanticUsage, "labeling": labelUsage, "upgrade": nil, "query": nil, "coding_task": nil},
		"amortized": map[string]any{"successful_graph_assisted_tasks": 0, "initialization_cost_per_successful_task": nil},
	}
	if sessionID != "" && semanticUsage.Measurement == "host_session" {
		cost["session_id"] = sessionID
	}
	body, err := json.MarshalIndent(cost, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "cost.json"), append(body, '\n'), 0o600)
}

func measureUsage(spans []tracestore.Span, sessionID string, start, end time.Time) measuredUsage {
	var input, output, cache int64
	var cost float64
	found := false
	for _, span := range spans {
		at := time.Unix(0, span.StartTimeUnixN)
		if span.SessionID != sessionID || at.Before(start) || at.After(end) {
			continue
		}
		for key, target := range map[string]*int64{
			"gen_ai.usage.input_tokens": &input, "gen_ai.usage.output_tokens": &output,
			"gen_ai.usage.cache.read_input_tokens": &cache, "gen_ai.usage.cache.creation_input_tokens": &cache,
		} {
			if raw := span.Attributes[key]; raw != "" {
				if value, parseErr := strconv.ParseInt(raw, 10, 64); parseErr == nil {
					*target += value
					found = true
				}
			}
		}
		for _, key := range []string{"gen_ai.usage.cost", "gen_ai.usage.cost_usd"} {
			if raw := span.Attributes[key]; raw != "" {
				if value, parseErr := strconv.ParseFloat(raw, 64); parseErr == nil {
					cost += value
					found = true
					break
				}
			}
		}
	}
	if !found {
		return measuredUsage{Measurement: "unavailable"}
	}
	return measuredUsage{InputTokens: &input, OutputTokens: &output, CacheTokens: &cache, CostUSD: &cost, Measurement: "host_session"}
}
