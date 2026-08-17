package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/graph"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/session"
)

func TestReconcileLifecycleGraphRecordsRefreshBesideEvaluationState(t *testing.T) {
	root := t.TempDir()
	paths := harness.Resolve(root)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.GraphJSON, []byte(`{"nodes":[{"id":"old"}],"edges":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.GraphState, []byte(`{"source_file_fingerprint":"stale","last_build_result":"success"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	store := session.NewStore(paths)
	if err := store.Start(session.Meta{ID: "session-1", Vendor: "codex", StartedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}

	previous := lifecycleGraphUpdate
	called := 0
	lifecycleGraphUpdate = func(context.Context, string, harness.Paths, bool, string) (graph.Result, error) {
		called++
		return graph.Result{Status: "needs_agent_semantic", RunID: "run-1"}, nil
	}
	t.Cleanup(func() { lifecycleGraphUpdate = previous })

	if err := reconcileLifecycleGraph(root, paths, config.Default(), "session_end", "session-1"); err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Fatalf("graph update calls = %d, want 1", called)
	}
	doc, err := store.ReadDocument("session-1")
	if err != nil {
		t.Fatal(err)
	}
	if doc.GraphRefresh.Status != "continuation_required" || doc.GraphRefresh.RunID != "run-1" || doc.GraphRefresh.CompletedAt == nil {
		t.Fatalf("unexpected graph refresh state: %+v", doc.GraphRefresh)
	}
}

func TestLifecycleGraphRefreshCanBeManual(t *testing.T) {
	cfg := config.Default()
	cfg.Graph.RefreshPolicy = "manual"
	if graphLifecycleRefreshEnabled(cfg) {
		t.Fatal("manual refresh policy must disable session lifecycle refresh")
	}
}
