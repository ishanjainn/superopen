package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func mcpBody(t *testing.T, raw []byte) []byte {
	t.Helper()
	if i := bytes.Index(raw, []byte("\r\n\r\n")); i >= 0 && bytes.HasPrefix(bytes.ToLower(raw), []byte("content-length:")) {
		return raw[i+4:]
	}
	return raw
}

func decodeMCPJSON(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	body := mcpBody(t, raw)
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode: %v body=%q", err, body)
	}
	return resp
}

func TestInitializeIncludesInstructions(t *testing.T) {
	var out bytes.Buffer
	in := bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n")
	srv := Server{Root: t.TempDir()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = srv.Serve(ctx, in, &out)
	resp := decodeMCPJSON(t, out.Bytes())
	result, _ := resp["result"].(map[string]any)
	info, _ := result["serverInfo"].(map[string]any)
	if info["name"] != "superopen" {
		t.Fatalf("serverInfo.name = %v", info["name"])
	}
	instr, _ := result["instructions"].(string)
	if !strings.Contains(instr, "graph_search") {
		t.Fatalf("instructions missing graph_search: %q", instr)
	}
	if !strings.Contains(instr, "memory_search") {
		t.Fatalf("instructions missing memory_search: %q", instr)
	}
}

func TestInitializeContentLengthFraming(t *testing.T) {
	payload := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`)
	var in bytes.Buffer
	fmt.Fprintf(&in, "Content-Length: %d\r\n\r\n", len(payload))
	in.Write(payload)
	var out bytes.Buffer
	srv := Server{Root: t.TempDir()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = srv.Serve(ctx, &in, &out)
	raw := out.Bytes()
	if bytes.HasPrefix(bytes.ToLower(raw), []byte("content-length:")) {
		t.Fatalf("Cursor hangs on Content-Length replies; want NDJSON: %q", raw)
	}
	resp := decodeMCPJSON(t, raw)
	result, _ := resp["result"].(map[string]any)
	if result["protocolVersion"] != "2024-11-05" {
		t.Fatalf("protocolVersion = %v", result["protocolVersion"])
	}
	info, _ := result["serverInfo"].(map[string]any)
	if info["name"] != "superopen" {
		t.Fatalf("serverInfo.name = %v", info["name"])
	}
}

func TestToolsListIncludesMemory(t *testing.T) {
	var out bytes.Buffer
	in := bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}` + "\n")
	srv := Server{Root: t.TempDir()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = srv.Serve(ctx, in, &out)
	raw := out.String()
	if !strings.Contains(raw, `"memory_search"`) || !strings.Contains(raw, `"memory_get"`) || !strings.Contains(raw, `"memory_recall"`) || !strings.Contains(raw, `"memory_when"`) || !strings.Contains(raw, `"memory_temporal_recall"`) {
		t.Fatalf("tools/list missing memory tools: %s", raw)
	}
}

func TestToolsListRequiredIsAlwaysArray(t *testing.T) {
	var out bytes.Buffer
	in := bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}` + "\n")
	srv := Server{Root: t.TempDir()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = srv.Serve(ctx, in, &out)
	body := mcpBody(t, out.Bytes())
	var resp struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				InputSchema struct {
					Required json.RawMessage `json:"required"`
				} `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode: %v body=%q", err, body)
	}
	if len(resp.Result.Tools) == 0 {
		t.Fatal("no tools")
	}
	for _, tool := range resp.Result.Tools {
		raw := bytes.TrimSpace(tool.InputSchema.Required)
		if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
			t.Fatalf("%s inputSchema.required is null (Cursor drops the whole tools list)", tool.Name)
		}
		var required []string
		if err := json.Unmarshal(raw, &required); err != nil {
			t.Fatalf("%s inputSchema.required must be an array: %s", tool.Name, raw)
		}
	}
}
