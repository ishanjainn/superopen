package graph

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/ishanjainn/superopen/internal/harness"
)

func TestNeedsRefreshUsesFingerprintAndPreservesSemanticContinuation(t *testing.T) {
	root := t.TempDir()
	paths := harness.Resolve(root)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.GraphJSON, []byte(`{"nodes":[{"id":"n"}],"edges":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	writeState := func(fingerprint, result, runID string) {
		t.Helper()
		body, _ := json.Marshal(map[string]any{
			"source_file_fingerprint": fingerprint,
			"last_build_result":       result,
			"pending_semantic_run_id": runID,
		})
		if err := os.WriteFile(paths.GraphState, body, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	current := SourceFingerprint(root)
	writeState(current, "success", "")
	if NeedsRefresh(root, paths) {
		t.Fatal("current graph must not refresh at session boundaries")
	}
	if err := os.WriteFile(root+"/service.go", []byte("package service\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !NeedsRefresh(root, paths) {
		t.Fatal("source change must request lifecycle refresh")
	}
	writeState("stale", "continuation_required", "run-pending")
	if NeedsRefresh(root, paths) {
		t.Fatal("pending semantic continuation must be resumed, not replaced")
	}
}
