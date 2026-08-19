// Package graphmcp exposes the native Superopen graph over MCP stdio.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/ishanjainn/superopen/internal/graph/client"
	"github.com/ishanjainn/superopen/internal/graph/format"
	"github.com/ishanjainn/superopen/internal/graph/watch"
)

type Server struct {
	Root   string
	Client client.Client
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func (s Server) Serve(ctx context.Context, input io.Reader, output io.Writer) error {
	runner := &watch.Runner{Root: s.Root, Client: s.Client}
	runner.Start(ctx)
	defer runner.Stop()

	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 16<<20)
	encoder := json.NewEncoder(output)
	for scanner.Scan() {
		var request rpcRequest
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			if encodeErr := encoder.Encode(rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: err.Error()}}); encodeErr != nil {
				return encodeErr
			}
			continue
		}
		// MCP notifications intentionally have no response.
		if len(request.ID) == 0 {
			continue
		}
		response := rpcResponse{JSONRPC: "2.0", ID: request.ID}
		switch request.Method {
		case "initialize":
			response.Result = map[string]any{
				"protocolVersion": "2025-06-18",
				"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
				"serverInfo":      map[string]any{"name": "superopen-graph", "version": "1"},
				"instructions":    mcpInstructions,
			}
		case "ping":
			response.Result = map[string]any{}
		case "tools/list":
			response.Result = map[string]any{"tools": nativeTools()}
		case "tools/call":
			var call toolCall
			if err := json.Unmarshal(request.Params, &call); err != nil {
				response.Error = &rpcError{Code: -32602, Message: err.Error()}
				break
			}
			text, err := s.callTool(ctx, call)
			if err != nil {
				response.Result = map[string]any{
					"content": []map[string]string{{"type": "text", "text": err.Error()}},
					"isError": true,
				}
			} else {
				response.Result = map[string]any{"content": []map[string]string{{"type": "text", "text": text}}}
			}
		default:
			response.Error = &rpcError{Code: -32601, Message: "method not found"}
		}
		if err := encoder.Encode(response); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (s Server) callTool(ctx context.Context, call toolCall) (string, error) {
	var operation api.Operation
	var params any
	switch call.Name {
	case "graph_query":
		var value api.QueryRequest
		if err := decodeArguments(call.Arguments, &value); err != nil {
			return "", err
		}
		value.RepoRoot = s.Root
		operation, params = api.OpQuery, value
	case "graph_search":
		var value api.SearchRequest
		if err := decodeArguments(call.Arguments, &value); err != nil {
			return "", err
		}
		applySearchAliases(call.Arguments, &value)
		value.RepoRoot = s.Root
		operation, params = api.OpSearch, value
	case "code_search":
		var value api.CodeSearchRequest
		if err := decodeArguments(call.Arguments, &value); err != nil {
			return "", err
		}
		value.RepoRoot = s.Root
		operation, params = api.OpCodeSearch, value
	case "graph_trace":
		var value api.TraceRequest
		if err := decodeArguments(call.Arguments, &value); err != nil {
			return "", err
		}
		applyTraceAliases(call.Arguments, &value)
		value.RepoRoot = s.Root
		operation, params = api.OpTrace, value
	case "graph_snippet":
		var value api.SnippetRequest
		if err := decodeArguments(call.Arguments, &value); err != nil {
			return "", err
		}
		applySnippetAliases(call.Arguments, &value)
		value.RepoRoot = s.Root
		operation, params = api.OpSnippet, value
	case "graph_architecture":
		var value api.ArchitectureRequest
		if err := decodeArguments(call.Arguments, &value); err != nil {
			return "", err
		}
		value.RepoRoot = s.Root
		operation, params = api.OpArchitecture, value
	case "graph_impact":
		var value api.ImpactRequest
		if err := decodeArguments(call.Arguments, &value); err != nil {
			return "", err
		}
		value.RepoRoot = s.Root
		operation, params = api.OpImpact, value
	case "graph_schema":
		operation, params = api.OpSchema, api.SchemaRequest{RepoRoot: s.Root}
	default:
		return "", fmt.Errorf("unknown graph tool %q", call.Name)
	}
	var result json.RawMessage
	if err := s.Client.Call(ctx, operation, params, &result); err != nil {
		return "", err
	}
	return formatMCPResult(operation, result)
}

func formatMCPResult(operation api.Operation, result json.RawMessage) (string, error) {
	switch operation {
	case api.OpQuery:
		var query api.QueryResult
		if json.Unmarshal(result, &query) == nil && query.Text != "" {
			return query.Text, nil
		}
	case api.OpSearch:
		var search api.SearchResult
		if json.Unmarshal(result, &search) == nil {
			return format.SearchCompact(search), nil
		}
	case api.OpTrace:
		var trace api.TraceResult
		if json.Unmarshal(result, &trace) == nil {
			return format.TraceCompact(trace), nil
		}
	case api.OpSnippet:
		var snippet api.SnippetResult
		if json.Unmarshal(result, &snippet) == nil {
			return format.SnippetCompact(snippet), nil
		}
	case api.OpArchitecture:
		var architecture api.ArchitectureResult
		if json.Unmarshal(result, &architecture) == nil {
			return format.ArchitectureCompact(architecture), nil
		}
	}
	var pretty any
	if err := json.Unmarshal(result, &pretty); err != nil {
		return string(result), nil
	}
	body, err := json.MarshalIndent(pretty, "", "  ")
	return string(body), err
}

func decodeArguments(raw json.RawMessage, value any) error {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	return json.Unmarshal(raw, value)
}

func argString(raw json.RawMessage, keys ...string) string {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	for _, key := range keys {
		if v, ok := m[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func applySearchAliases(raw json.RawMessage, value *api.SearchRequest) {
	if value.Query == "" {
		value.Query = argString(raw, "query", "name", "symbol")
	}
	if value.NamePattern == "" {
		value.NamePattern = argString(raw, "name_pattern", "pattern")
	}
}

func applyTraceAliases(raw json.RawMessage, value *api.TraceRequest) {
	if value.Start == "" {
		value.Start = argString(raw, "start", "function_name", "symbol", "qualified_name")
	}
	if value.Target == "" {
		value.Target = argString(raw, "target", "to", "destination")
	}
}

func applySnippetAliases(raw json.RawMessage, value *api.SnippetRequest) {
	if value.QualifiedName == "" {
		value.QualifiedName = argString(raw, "qualified_name", "function_name", "symbol", "name")
	}
}

func nativeTools() []tool {
	object := func(required []string, properties map[string]any) map[string]any {
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	}
	stringProp := map[string]any{"type": "string"}
	integerProp := map[string]any{"type": "integer"}
	searchSchema := object(nil, map[string]any{"query": stringProp, "name_pattern": stringProp, "name": stringProp, "labels": map[string]any{"type": "array", "items": stringProp}, "limit": integerProp})
	// function_name is accepted as an alternate start identifier for agents.
	traceSchema := object(nil, map[string]any{"start": stringProp, "function_name": stringProp, "target": stringProp, "direction": stringProp, "depth": integerProp, "limit": integerProp})
	snippetSchema := object(nil, map[string]any{"qualified_name": stringProp, "function_name": stringProp, "file": stringProp, "start_line": integerProp, "end_line": integerProp, "context_lines": integerProp})
	archSchema := object(nil, map[string]any{"path": stringProp, "aspects": map[string]any{"type": "array", "items": stringProp}, "limit": integerProp})
	return []tool{
		{Name: "graph_query", Description: "PRIMARY for natural-language structural questions. Prefer over Read/Grep. Returns focused native code-graph context.", InputSchema: object([]string{"question"}, map[string]any{"question": stringProp, "terms": map[string]any{"type": "array", "items": stringProp}, "depth": integerProp, "budget": integerProp})},
		{Name: "graph_search", Description: "PRIMARY symbol lookup. Prefer over Grep/Glob when finding functions, types, or packages by name.", InputSchema: searchSchema},
		{Name: "code_search", Description: "Literal/regex source search when graph_search is insufficient.", InputSchema: object([]string{"pattern"}, map[string]any{"pattern": stringProp, "file_pattern": stringProp, "regex": map[string]any{"type": "boolean"}, "limit": integerProp})},
		{Name: "graph_trace", Description: "PRIMARY for callers/callees and call/config paths. Prefer over manual Read chains.", InputSchema: traceSchema},
		{Name: "graph_snippet", Description: "PRIMARY for reading a known symbol body. Prefer over Read when you have a qualified name.", InputSchema: snippetSchema},
		{Name: "graph_architecture", Description: "PRIMARY architecture overview from the native graph.", InputSchema: archSchema},
		{Name: "graph_impact", Description: "Impact analysis from symbols, files, or a git base.", InputSchema: object(nil, map[string]any{"base": stringProp, "symbols": map[string]any{"type": "array", "items": stringProp}, "files": map[string]any{"type": "array", "items": stringProp}, "depth": integerProp})},
		{Name: "graph_schema", Description: "Show native graph labels, edge types, and patterns.", InputSchema: object(nil, map[string]any{})},
	}
}

const mcpInstructions = `Superopen native code graph. For architecture, callers/callees, symbol lookup, and "how does X work" questions: call graph_query first (stop if answered), else graph_search → graph_snippet → graph_trace (qualified names), or graph_architecture — BEFORE broad Read/Grep/Glob. Graph builds are local (no LLM). Root is resolved from the agent working directory.`
