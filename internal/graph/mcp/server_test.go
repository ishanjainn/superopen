package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestInitializeIncludesInstructions(t *testing.T) {
	var out bytes.Buffer
	in := bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n")
	srv := Server{Root: t.TempDir()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = srv.Serve(ctx, in, &out)
	var resp map[string]any
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%q", err, out.String())
	}
	result, _ := resp["result"].(map[string]any)
	instr, _ := result["instructions"].(string)
	if !strings.Contains(instr, "graph_search") {
		t.Fatalf("instructions missing graph_search: %q", instr)
	}
}
