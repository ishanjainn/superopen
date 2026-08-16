package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/redact"
)

// InvocationResult keeps Graphify's stdout and stderr separate so AXI JSON can
// never be corrupted by progress diagnostics.
type InvocationResult struct {
	Stdout []byte
	Stderr []byte
}

func Run(ctx context.Context, repoRoot string, args ...string) (InvocationResult, error) {
	bin, prefix, err := resolveGraphify()
	if err != nil {
		return InvocationResult{}, err
	}
	paths := ResolvePaths(repoRoot)
	cmdArgs := append(append([]string{}, prefix...), args...)
	backend := backendFromArgs(args)
	if backend == "" && len(args) > 0 && (args[0] == "label" || args[0] == "cluster-only" || args[0] == "add" || args[0] == "prs") {
		if cfg, loadErr := config.Load(paths.Config); loadErr == nil {
			backend = cfg.Graph.SemanticBackend
		}
	}
	if len(args) > 0 && (args[0] == "label" || args[0] == "cluster-only") && backend != "" && backend != "agent" && backend != "none" && backendFromArgs(args) == "" {
		cmdArgs = append(cmdArgs, "--backend="+backend)
	}
	cmd := exec.CommandContext(ctx, bin, cmdArgs...)
	cmd.Dir = repoRoot
	cmd.Env = graphifyEnvWithBackend(paths.GraphDir, backend)
	if len(args) > 0 && args[0] == "prs" && containsArg(args, "--triage") && backend != "" && backend != "agent" && backend != "none" {
		cmd.Env = append(cmd.Env, "GRAPHIFY_TRIAGE_BACKEND="+backend)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	result := InvocationResult{Stdout: []byte(redact.StringFull(stdout.String())), Stderr: []byte(redact.StringFull(stderr.String()))}
	if err != nil {
		return result, fmt.Errorf("graphify %s: %w (%s)", filepath.Base(bin), err, truncateOut(result.Stderr, 1200))
	}
	return result, nil
}

func backendFromArgs(args []string) string {
	for i, arg := range args {
		if strings.HasPrefix(arg, "--backend=") {
			return strings.TrimPrefix(arg, "--backend=")
		}
		if arg == "--backend" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func GraphHash(repoRoot string) string {
	data, err := os.ReadFile(ResolvePaths(repoRoot).GraphState)
	if err != nil {
		return ""
	}
	var state struct {
		GraphSHA256 string `json:"graph_sha256"`
	}
	_ = json.Unmarshal(data, &state)
	return state.GraphSHA256
}

func ExportCanvas(ctx context.Context, repoRoot, output string) error {
	paths := ResolvePaths(repoRoot)
	if output == "" {
		output = filepath.Join(paths.GraphDir, "exports", "graph.canvas")
	}
	if !filepath.IsAbs(output) {
		output = filepath.Join(repoRoot, output)
	}
	if !strings.EqualFold(filepath.Ext(output), ".canvas") {
		return fmt.Errorf("canvas output must end in .canvas")
	}
	if rel, relErr := filepath.Rel(filepath.Join(repoRoot, ".so"), output); relErr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		graphRel, graphErr := filepath.Rel(paths.GraphDir, output)
		if graphErr != nil || graphRel == ".." || strings.HasPrefix(graphRel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("canvas output inside .so must remain under .so/graph")
		}
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	python, err := graphifyPython()
	if err != nil {
		return err
	}
	graphJSON, _ := json.Marshal(paths.GraphJSON)
	labelsJSON, _ := json.Marshal(filepath.Join(paths.GraphDir, ".graphify_labels.json"))
	outputJSON, _ := json.Marshal(output)
	script := fmt.Sprintf(`import json
from pathlib import Path
from networkx.readwrite import json_graph
from graphify.export import to_canvas
gp=Path(%s);raw=json.loads(gp.read_text(encoding='utf-8'))
if 'links' not in raw and 'edges' in raw: raw=dict(raw,links=raw['edges'])
try:G=json_graph.node_link_graph(raw,edges='links')
except TypeError:G=json_graph.node_link_graph(raw)
communities={}
for node,data in G.nodes(data=True):
 cid=data.get('community')
 if cid is not None: communities.setdefault(int(cid),[]).append(str(node))
lp=Path(%s);labels={int(k):v for k,v in json.loads(lp.read_text(encoding='utf-8')).items()} if lp.exists() else {}
to_canvas(G,communities,%s,community_labels=labels or None)`, string(graphJSON), string(labelsJSON), string(outputJSON))
	_, err = runPython(ctx, python, repoRoot, paths.GraphDir, script)
	return err
}

func Serve(ctx context.Context, repoRoot string, args ...string) error {
	python, err := graphifyPython()
	if err != nil {
		return err
	}
	paths := ResolvePaths(repoRoot)
	cmdArgs := append([]string{"-m", "graphify.serve", "--graph", paths.GraphJSON}, args...)
	cmd := exec.CommandContext(ctx, python, cmdArgs...)
	cmd.Dir, cmd.Env = repoRoot, graphifyEnv(paths.GraphDir)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
