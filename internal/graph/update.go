package graph

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/retrieve"
)

// NeedsRefresh compares the published graph fingerprint with current indexed
// sources. A pending agent-semantic run is already the refresh operation, so it
// is not replaced by another background run.
func NeedsRefresh(repoRoot string, paths harness.Paths) bool {
	if _, err := os.Stat(paths.GraphJSON); err != nil {
		return false
	}
	body, err := os.ReadFile(paths.GraphState)
	if err != nil {
		return true
	}
	var state struct {
		SourceFingerprint string `json:"source_file_fingerprint"`
		LastBuildResult   string `json:"last_build_result"`
		PendingRunID      string `json:"pending_semantic_run_id"`
	}
	if json.Unmarshal(body, &state) != nil {
		return true
	}
	if state.LastBuildResult == "continuation_required" && state.PendingRunID != "" {
		return false
	}
	return state.SourceFingerprint == "" || state.SourceFingerprint != SourceFingerprint(repoRoot)
}

// ClaimLifecycleRefresh prevents overlapping SessionEnd and SessionStart
// maintenance processes from running Graphify concurrently. Manual graph
// commands remain independent and explicit.
func ClaimLifecycleRefresh(repoRoot string) (func(), bool) {
	sum := sha256.Sum256([]byte(filepath.Clean(repoRoot)))
	lock := filepath.Join(os.TempDir(), "superopen-graph-refresh-"+hex.EncodeToString(sum[:12])+".lock")
	if err := os.Mkdir(lock, 0o700); err != nil {
		if info, statErr := os.Stat(lock); statErr == nil && time.Since(info.ModTime()) > 30*time.Minute {
			_ = os.RemoveAll(lock)
			if err = os.Mkdir(lock, 0o700); err == nil {
				return func() { _ = os.RemoveAll(lock) }, true
			}
		}
		return func() {}, false
	}
	return func() { _ = os.RemoveAll(lock) }, true
}

// UpdateAtomic gives Graphify a staged copy of its native manifest/cache,
// validates the updated graph there, then publishes it over the prior graph.
func UpdateAtomic(ctx context.Context, repoRoot string, paths harness.Paths, codeOnly bool, backend string) (Result, error) {
	return updateAtomic(ctx, repoRoot, repoRoot, paths, codeOnly, backend, nil)
}

func UpdateAtomicWithFlags(ctx context.Context, repoRoot string, paths harness.Paths, codeOnly bool, backend string, flags []string) (Result, error) {
	return updateAtomic(ctx, repoRoot, repoRoot, paths, codeOnly, backend, flags)
}

func UpdateAtomicTargetWithFlags(ctx context.Context, repoRoot, target string, paths harness.Paths, codeOnly bool, backend string, flags []string) (Result, error) {
	return updateAtomic(ctx, repoRoot, target, paths, codeOnly, backend, flags)
}

func updateAtomic(ctx context.Context, repoRoot, target string, paths harness.Paths, codeOnly bool, backend string, flags []string) (Result, error) {
	if target == "" {
		target = repoRoot
	}
	if !codeOnly && backend == "agent" {
		semanticOpts := SemanticStartOptions{Incremental: true, Target: target}
		if cfg, loadErr := config.Load(paths.Config); loadErr == nil {
			semanticOpts.Deep = strings.EqualFold(strings.TrimSpace(cfg.Graph.Mode), "deep")
		}
		run, err := StartSemanticRunWithOptions(ctx, repoRoot, semanticOpts)
		if err != nil {
			recordRefreshFailure(paths, err)
			return Result{}, err
		}
		semanticChanges := 0
		for kind, files := range run.ChangedFiles {
			if kind != "code" {
				semanticChanges += len(files)
			}
		}
		if semanticChanges > 0 {
			// Graphify update is itself the local AST/deletion refresh. Publish it
			// first so coding sessions see current code while semantic work waits.
			if _, updateErr := updateAtomic(ctx, repoRoot, target, paths, true, backend, flags); updateErr != nil {
				return Result{}, updateErr
			}
			run.BaseGraphHash = GraphHash(repoRoot)
			if dir, dirErr := runDir(paths, run.RunID); dirErr == nil {
				if saveErr := saveSemanticRun(dir, run); saveErr != nil {
					return Result{}, saveErr
				}
			}
			if err := recordSemanticContinuation(paths, run); err != nil {
				return Result{}, err
			}
			return Result{Engine: "graphify", EngineVersion: PinnedVersion, Status: "needs_agent_semantic", Path: paths.GraphJSON, RunID: run.RunID}, nil
		}
		if dir, dirErr := runDir(paths, run.RunID); dirErr == nil {
			_ = os.RemoveAll(dir)
		}
		return updateAtomic(ctx, repoRoot, target, paths, true, backend, flags)
	}
	if _, ok := ExistingResult(paths); !ok {
		return RefreshAtomicWithOptions(repoRoot, paths, BuildOptions{CodeOnly: codeOnly, SemanticBackend: backend, Target: target})
	}
	SweepStaleGraphWork(paths)
	stage, err := os.MkdirTemp(paths.GraphDir, ".staging-update-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(stage)
	entries, err := os.ReadDir(paths.GraphDir)
	if err != nil {
		return Result{}, err
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".staging-") {
			continue
		}
		src, dst := filepath.Join(paths.GraphDir, e.Name()), filepath.Join(stage, e.Name())
		if e.IsDir() {
			if err := copyDir(src, dst); err != nil {
				return Result{}, err
			}
		} else {
			if err := replaceFile(src, dst); err != nil {
				return Result{}, err
			}
		}
	}
	bin, prefix, err := resolveGraphify()
	if err != nil {
		return Result{}, err
	}
	args := append(append([]string{}, prefix...), "update", target)
	args = append(args, flags...)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = repoRoot
	cmd.Env = graphifyEnvWithBackend(stage, backend)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		recordRefreshFailure(paths, fmt.Errorf("graphify update: %w (%s)", err, truncateOut(stderr.Bytes(), 800)))
		return Result{}, fmt.Errorf("graphify update: %w (%s)", err, truncateOut(stderr.Bytes(), 800))
	}
	sanitized, err := sanitizeManagedGraphArtifacts(stage)
	if err != nil {
		return Result{}, err
	}
	noCluster := containsArg(flags, "--no-cluster")
	if sanitized && !noCluster {
		labelBackend := backend
		if codeOnly || labelBackend == "agent" || labelBackend == "none" {
			labelBackend = ""
		}
		if err := finalizeGraphifyArtifacts(bin, prefix, repoRoot, stage, labelBackend); err != nil {
			return Result{}, err
		}
	} else if noCluster {
		_ = os.Remove(filepath.Join(stage, "graph.html"))
	}
	data, err := os.ReadFile(filepath.Join(stage, "graph.json"))
	if err != nil {
		return Result{}, fmt.Errorf("Graphify update produced no graph: %w", err)
	}
	data = describeGraphJSON(data)
	if err := os.WriteFile(filepath.Join(stage, "graph.json"), data, 0o644); err != nil {
		return Result{}, err
	}
	nodes, edges := countGraph(data)
	if nodes == 0 {
		return Result{}, fmt.Errorf("Graphify update produced an empty graph")
	}
	sp := paths
	sp.GraphDir = stage
	sp.GraphJSON = filepath.Join(stage, "graph.json")
	sp.GraphHTML = filepath.Join(stage, "graph.html")
	sp.GraphCorpus = filepath.Join(stage, "corpus.json")
	sp.GraphState = filepath.Join(stage, "state.json")
	_, _ = retrieve.Rebuild(repoRoot, sp)
	semantic := semanticState{Required: !codeOnly, Backend: backend}
	if codeOnly {
		semantic = priorSemanticState(paths)
	}
	_ = writeGraphState(sp, data, nodes, edges, "ready", semantic)
	if html, readErr := os.ReadFile(sp.GraphHTML); readErr == nil && !strings.HasPrefix(string(html), graphHTMLComment) {
		_ = os.WriteFile(sp.GraphHTML, append([]byte(graphHTMLComment), html...), 0o644)
	}
	if err := validateGraphArtifactsWithOptions(sp, noCluster); err != nil {
		return Result{}, err
	}
	if err := validateGraphQuery(repoRoot, sp); err != nil {
		return Result{}, err
	}
	if err := installGraphStaging(stage, paths); err != nil {
		return Result{}, err
	}
	_, htmlErr := os.Stat(paths.GraphHTML)
	return Result{NodeCount: nodes, EdgeCount: edges, Engine: "graphify", EngineVersion: PinnedVersion, Status: "ready", Path: paths.GraphJSON, HasHTML: htmlErr == nil}, nil
}
