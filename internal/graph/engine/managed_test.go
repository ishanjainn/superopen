package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestBuildRefusesUnmanagedRepo(t *testing.T) {
	repo := t.TempDir()
	params, err := json.Marshal(api.BuildRequest{RepoRoot: repo})
	if err != nil {
		t.Fatal(err)
	}
	resp := (Server{EngineVersion: "test"}).Handle(context.Background(), api.Request{
		Protocol:  api.ProtocolVersion,
		Operation: api.OpBuild,
		Params:    params,
	})
	if resp.OK {
		t.Fatal("expected not_managed")
	}
	if resp.Error == nil || resp.Error.Code != "not_managed" {
		t.Fatalf("got %+v", resp.Error)
	}
	if _, err := os.Stat(filepath.Join(repo, ".so")); !os.IsNotExist(err) {
		t.Fatalf(".so must not be created, stat=%v", err)
	}
}
