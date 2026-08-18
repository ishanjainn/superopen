package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestServerDispatchesCodeSearchIncrementalAndTraceIngest(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	repo := t.TempDir()
	server := Server{EngineVersion: "test"}
	for operation, params := range map[api.Operation]any{
		api.OpCodeSearch: api.CodeSearchRequest{RepoRoot: repo, Pattern: "func", Limit: 5},
		api.OpIncremental: api.IncrementalRequest{
			BuildRequest: api.BuildRequest{RepoRoot: repo},
			Changes:      api.ChangeSet{RequiresFull: true},
		},
		api.OpTraceIngest: api.TraceIngestRequest{
			RepoRoot: repo,
			Traces: []api.RuntimeTrace{
				{Source: "caller.main", Target: "callee.run", Type: "CALLS"},
			},
		},
	} {
		body, _ := json.Marshal(params)
		response := server.Handle(context.Background(), api.Request{Protocol: api.ProtocolVersion, Operation: operation, Params: body})
		if operation == api.OpIncremental {
			if response.OK || response.Error == nil || response.Error.Code != "incremental" {
				t.Fatalf("incremental without assets should fail: %+v", response.Error)
			}
			continue
		}
		if !response.OK {
			t.Fatalf("%s failed: %+v", operation, response.Error)
		}
	}
}

func TestTraceIngestAcceptsValidTraces(t *testing.T) {
	result, err := TraceIngest(context.Background(), t.TempDir(), api.TraceIngestRequest{
		Traces: []api.RuntimeTrace{
			{Source: "a.run", Target: "b.run"},
			{Source: "", Target: "missing"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Accepted != 1 || result.Rejected != 1 {
		t.Fatalf("result = %+v", result)
	}
}

func TestCodeSearchReturnsEmptyWithoutMatches(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := CodeSearch(context.Background(), root, api.CodeSearchRequest{Pattern: "missing-symbol-xyz"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 0 {
		t.Fatalf("matches = %#v", result.Matches)
	}
}

func TestCodeSearchFallbackFindsMatchesWithoutRipgrep(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\nfunc HelloWorld() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir()) // ensure LookPath("rg") fails
	result, err := CodeSearch(context.Background(), root, api.CodeSearchRequest{Pattern: "HelloWorld", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 1 {
		t.Fatalf("matches = %#v", result.Matches)
	}
	if result.Matches[0].Location.File != "main.go" {
		t.Fatalf("file = %q", result.Matches[0].Location.File)
	}
}
