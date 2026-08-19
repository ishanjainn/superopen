package engine

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestServerDispatchesSchemaAndCypher(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	repo := t.TempDir()
	paths, err := CachePaths(repo)
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenWritable(paths.Database)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Build(context.Background(), func(builder *Builder) error {
		if err := builder.PutProject(ProjectRecord{Name: "fixture", RootPath: repo, Generation: "one", EngineVersion: "test", IndexedAt: time.Now()}); err != nil {
			return err
		}
		_, err := builder.PutNode(api.Node{Project: "fixture", Label: "Function", Name: "Run", QualifiedName: "fixture.Run", Location: api.Location{File: filepath.Join("src", "run.go")}})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	server := Server{EngineVersion: "test"}
	for operation, params := range map[api.Operation]any{
		api.OpSchema: api.SchemaRequest{RepoRoot: repo, Project: "fixture"},
		api.OpCypher: api.CypherRequest{RepoRoot: repo, Project: "fixture", Query: `MATCH (n:Function) RETURN n.name AS name`},
	} {
		body, _ := json.Marshal(params)
		response := server.Handle(context.Background(), api.Request{Protocol: api.ProtocolVersion, Operation: operation, Params: body})
		if !response.OK {
			t.Fatalf("%s failed: %+v", operation, response.Error)
		}
	}
}

func TestServerRejectsProtocolMismatch(t *testing.T) {
	response := (Server{}).Handle(context.Background(), api.Request{Protocol: 99, Operation: api.OpCapabilities})
	if response.OK || response.Error == nil || response.Error.Code != "protocol_mismatch" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestServerReportsMissingGraphWithoutCreatingRepositoryState(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	repo := t.TempDir()
	params, _ := json.Marshal(api.StatusRequest{RepoRoot: repo})
	response := (Server{EngineVersion: "test"}).Handle(context.Background(), api.Request{
		Protocol: api.ProtocolVersion, Operation: api.OpStatus, Params: params,
	})
	if !response.OK {
		t.Fatalf("unexpected error: %+v", response.Error)
	}
	var status api.Status
	if err := json.Unmarshal(response.Result, &status); err != nil {
		t.Fatal(err)
	}
	if status.State != "missing" || status.Capabilities.Complete {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestServerRejectsUnknownParams(t *testing.T) {
	response := (Server{}).Handle(context.Background(), api.Request{
		Protocol: api.ProtocolVersion, Operation: api.OpStatus,
		Params: json.RawMessage(`{"repo_root":".","surprise":true}`),
	})
	if response.OK || response.Error == nil || response.Error.Code != "invalid_params" {
		t.Fatalf("unexpected response: %+v", response)
	}
}
