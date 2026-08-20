// Package graphmcp exposes the native Superopen graph over MCP stdio.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/ishanjainn/superopen/internal/graph/client"
	"github.com/ishanjainn/superopen/internal/graph/format"
	"github.com/ishanjainn/superopen/internal/graph/watch"
	"github.com/ishanjainn/superopen/internal/memory"
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
	Name        string     `json:"name"`
	Description string     `json:"description"`
	InputSchema toolSchema `json:"inputSchema"`
}

// toolSchema keeps `required` a JSON array. A nil Go slice marshals as null
// and Cursor then drops the entire tools list (0 tools enabled).
type toolSchema struct {
	Type                 string         `json:"type"`
	Properties           map[string]any `json:"properties"`
	Required             []string       `json:"required"`
	AdditionalProperties bool           `json:"additionalProperties"`
}

func (s Server) Serve(ctx context.Context, input io.Reader, output io.Writer) error {
	runner := &watch.Runner{Root: s.Root, Client: s.Client}
	runner.Start(ctx)
	defer runner.Stop()

	reader := bufio.NewReaderSize(input, 16<<20)
	for {
		body, err := readMCPMessage(reader)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		var request rpcRequest
		if err := json.Unmarshal(body, &request); err != nil {
			if encodeErr := writeMCPMessage(output, rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: err.Error()}}); encodeErr != nil {
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
			protocol := "2025-06-18"
			var initParams struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			if json.Unmarshal(request.Params, &initParams) == nil && initParams.ProtocolVersion != "" {
				protocol = initParams.ProtocolVersion
			}
			response.Result = map[string]any{
				"protocolVersion": protocol,
				"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
				"serverInfo":      map[string]any{"name": "superopen", "version": "1"},
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
			// Bound every call so a hung embed/graph lookup cannot stall
			// the stdio loop. Cursor then idle-timeouts the next tool too.
			callCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			text, err := s.callTool(callCtx, call)
			cancel()
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
		if err := writeMCPMessage(output, response); err != nil {
			return err
		}
	}
}

func readMCPMessage(r *bufio.Reader) ([]byte, error) {
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			if err == io.EOF && len(bytes.TrimSpace(line)) > 0 {
				return bytes.TrimSpace(line), nil
			}
			return nil, err
		}
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		if len(trimmed) >= 15 && bytes.EqualFold(trimmed[:15], []byte("content-length:")) {
			n, err := strconv.Atoi(string(bytes.TrimSpace(trimmed[15:])))
			if err != nil || n < 0 {
				return nil, fmt.Errorf("invalid Content-Length")
			}
			for {
				header, err := r.ReadBytes('\n')
				if err != nil {
					return nil, err
				}
				if len(bytes.TrimSpace(header)) == 0 {
					break
				}
			}
			body := make([]byte, n)
			if _, err := io.ReadFull(r, body); err != nil {
				return nil, err
			}
			return body, nil
		}
		if trimmed[0] == '{' {
			return trimmed, nil
		}
	}
}

func writeMCPMessage(w io.Writer, v any) error {
	// Cursor's stdio client accepted newline-delimited JSON this morning and
	// hung in "connecting" after Content-Length framed replies.
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	if f, ok := w.(interface{ Flush() error }); ok {
		return f.Flush()
	}
	return nil
}

func (s Server) callTool(ctx context.Context, call toolCall) (string, error) {
	if strings.HasPrefix(call.Name, "memory_") {
		return s.callMemory(call)
	}
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
	object := func(required []string, properties map[string]any) toolSchema {
		if required == nil {
			required = []string{}
		}
		if properties == nil {
			properties = map[string]any{}
		}
		return toolSchema{Type: "object", Properties: properties, Required: required, AdditionalProperties: false}
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
		{Name: "memory_recall", Description: "PRIMARY recall over prior work. Hybrid rank (cosine, centrality, lexical, shape). Returns hits AND anti_hits (contradictions). Fetch bodies with memory_get.", InputSchema: object(nil, map[string]any{"query": stringProp, "budget_tokens": integerProp, "limit": integerProp})},
		{Name: "memory_temporal_recall", Description: "Time-bounded recall. as_of / changed_since filter valid_from/valid_to.", InputSchema: object(nil, map[string]any{"query": stringProp, "as_of": stringProp, "changed_since": stringProp, "budget_tokens": integerProp, "limit": integerProp})},
		{Name: "memory_search", Description: "Hybrid search over session moments, rollups, pins, and teachings. Returns ids + titles; fetch bodies with memory_get.", InputSchema: object(nil, map[string]any{"query": stringProp, "kind": stringProp, "session": stringProp, "file": stringProp, "limit": integerProp})},
		{Name: "memory_get", Description: "Fetch one memory episode by id from memory_search or memory_recall. Prefer over dumping session transcripts.", InputSchema: object([]string{"id"}, map[string]any{"id": map[string]any{"type": "string"}})},
		{Name: "memory_recall_shape", Description: "Recall by structural shape of the cue rather than wording.", InputSchema: object(nil, map[string]any{"query": stringProp, "limit": integerProp})},
		{Name: "memory_reinforce", Description: "Strengthen edges and centrality for a memory id.", InputSchema: object([]string{"id"}, map[string]any{"id": stringProp})},
		{Name: "memory_capture", Description: "Write a session rollup once (request/learned/next). Use only when SessionStart asks or the user wants a note saved. Memory is hints, not authority.", InputSchema: object(nil, map[string]any{"session": stringProp, "title": stringProp, "text": stringProp, "request": stringProp, "learned": stringProp, "next": stringProp, "kind": stringProp})},
		{Name: "memory_contradict", Description: "Write a successor memory that contradicts an older id. Does not overwrite the original.", InputSchema: object([]string{"id", "text"}, map[string]any{"id": stringProp, "text": stringProp, "title": stringProp})},
		{Name: "memory_teach", Description: "Add a teaching note (how we work, conventions) into long-term memory.", InputSchema: object([]string{"text"}, map[string]any{"text": stringProp, "title": stringProp})},
		{Name: "memory_consolidate", Description: "Cluster topics and decay unreinforced edges. Agent-only; users do not run this.", InputSchema: object(nil, map[string]any{})},
		{Name: "memory_profile", Description: "Read or set memory knobs stored in memory_meta.", InputSchema: object(nil, map[string]any{"key": stringProp, "value": stringProp})},
		{Name: "memory_curiosity", Description: "Low-centrality memories that may need reinforcement.", InputSchema: object(nil, map[string]any{"limit": integerProp})},
		{Name: "memory_patterns", Description: "Topic labels from clustered memories.", InputSchema: object(nil, map[string]any{"limit": integerProp})},
		{Name: "memory_events", Description: "Time-bucketed memory timeline (prompts and rollups, not tools).", InputSchema: object(nil, map[string]any{"limit": integerProp})},
		{Name: "memory_map", Description: "Layout of memory episodes for the /memory UI.", InputSchema: object(nil, map[string]any{"limit": integerProp})},
		{Name: "memory_recent", Description: "Recent non-tool memories.", InputSchema: object(nil, map[string]any{"limit": integerProp})},
		{Name: "memory_when", Description: "When a fact was captured. Excludes tools.", InputSchema: object(nil, map[string]any{"query": stringProp, "limit": integerProp})},
	}
}

func (s Server) callMemory(call toolCall) (string, error) {
	store, err := memory.OpenRoot(s.Root)
	if err != nil {
		return "", err
	}
	defer store.Close()
	switch call.Name {
	case "memory_recall":
		query := argString(call.Arguments, "query", "q")
		budget := argInt(call.Arguments, "budget_tokens", 0)
		res, err := store.Recall(query, budget)
		if err != nil {
			return "", err
		}
		limit := argInt(call.Arguments, "limit", 12)
		if limit > 0 && len(res.Hits) > limit {
			res.Hits = res.Hits[:limit]
		}
		return formatRecall(res), nil
	case "memory_temporal_recall":
		res, err := store.TemporalRecall(
			argString(call.Arguments, "query", "q"),
			argString(call.Arguments, "as_of"),
			argString(call.Arguments, "changed_since"),
			argInt(call.Arguments, "budget_tokens", 0),
		)
		if err != nil {
			return "", err
		}
		limit := argInt(call.Arguments, "limit", 12)
		if limit > 0 && len(res.Hits) > limit {
			res.Hits = res.Hits[:limit]
		}
		return formatRecall(res), nil
	case "memory_search":
		query := argString(call.Arguments, "query", "q")
		kind := argString(call.Arguments, "kind")
		sessionID := argString(call.Arguments, "session", "session_id")
		file := argString(call.Arguments, "file")
		limit := argInt(call.Arguments, "limit", 12)
		hits, err := store.Search(memory.SearchFilter{Query: query, Kind: kind, SessionID: sessionID, File: file, Limit: limit, RecordEconomy: true})
		if err != nil {
			return "", err
		}
		if len(hits) == 0 {
			return "0 memories", nil
		}
		var b strings.Builder
		for _, hit := range hits {
			fmt.Fprintf(&b, "#%d %s %s ~%d\n", hit.ID, hit.Kind, hit.Title, hit.Tokens)
		}
		b.WriteString("Fetch bodies with memory_get. Hints, not authority.")
		return b.String(), nil
	case "memory_get":
		id, err := strconv.ParseInt(argString(call.Arguments, "id"), 10, 64)
		if err != nil {
			return "", fmt.Errorf("id required")
		}
		ep, err := store.Get(id)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("#%d %s %s\n%s", ep.ID, ep.Kind, ep.Title, ep.Text), nil
	case "memory_capture":
		text := argString(call.Arguments, "text")
		if text == "" {
			var parts []string
			if v := argString(call.Arguments, "request"); v != "" {
				parts = append(parts, "request: "+v)
			}
			if v := argString(call.Arguments, "learned"); v != "" {
				parts = append(parts, "learned: "+v)
			}
			if v := argString(call.Arguments, "next"); v != "" {
				parts = append(parts, "next: "+v)
			}
			text = strings.Join(parts, "\n")
		}
		kind := argString(call.Arguments, "kind")
		if kind == "" {
			kind = memory.KindSession
		}
		ep, err := memory.CaptureRoot(s.Root, memory.CaptureInput{
			SessionID: argString(call.Arguments, "session", "session_id"),
			Kind:      kind,
			Source:    memory.SourceAgent,
			Title:     argString(call.Arguments, "title"),
			Text:      text,
		})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("captured #%d %s", ep.ID, ep.Title), nil
	case "memory_contradict":
		id, err := strconv.ParseInt(argString(call.Arguments, "id"), 10, 64)
		if err != nil {
			return "", fmt.Errorf("id required")
		}
		ep, err := store.Contradict(id, memory.CaptureInput{
			Title:  argString(call.Arguments, "title"),
			Text:   argString(call.Arguments, "text"),
			Source: memory.SourceAgent,
		})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("contradict #%d → #%d %s", id, ep.ID, ep.Title), nil
	case "memory_teach":
		ep, err := memory.TeachText(s.Root, argString(call.Arguments, "title"), argString(call.Arguments, "text"))
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("taught #%d %s", ep.ID, ep.Title), nil
	case "memory_recall_shape":
		hits, err := store.RecallShape(argString(call.Arguments, "query", "q", "cue"), argInt(call.Arguments, "limit", 8))
		if err != nil {
			return "", err
		}
		return formatHits(hits, "Fetch bodies with memory_get."), nil
	case "memory_reinforce":
		id, err := strconv.ParseInt(argString(call.Arguments, "id"), 10, 64)
		if err != nil {
			return "", fmt.Errorf("id required")
		}
		if err := store.Reinforce(id); err != nil {
			return "", err
		}
		return fmt.Sprintf("reinforced #%d", id), nil
	case "memory_consolidate":
		if err := store.Consolidate(); err != nil {
			return "", err
		}
		return "consolidated", nil
	case "memory_profile":
		key := argString(call.Arguments, "key")
		value := argString(call.Arguments, "value")
		if key != "" && value != "" {
			if err := store.SetProfile(key, value); err != nil {
				return "", err
			}
		}
		raw, _ := json.Marshal(store.Profile())
		return string(raw), nil
	case "memory_curiosity":
		hits, err := store.Curiosity(argInt(call.Arguments, "limit", 8))
		if err != nil {
			return "", err
		}
		return formatHits(hits, ""), nil
	case "memory_patterns":
		labels, err := store.Patterns(argInt(call.Arguments, "limit", 8))
		if err != nil {
			return "", err
		}
		if len(labels) == 0 {
			return "0 patterns", nil
		}
		return strings.Join(labels, "\n"), nil
	case "memory_events", "memory_recent":
		buckets, err := store.Events(argInt(call.Arguments, "limit", 40))
		if err != nil {
			return "", err
		}
		raw, _ := json.Marshal(buckets)
		return string(raw), nil
	case "memory_map":
		return store.MapJSON()
	case "memory_when":
		hits, err := store.When(argString(call.Arguments, "query", "q"), argInt(call.Arguments, "limit", 12))
		if err != nil {
			return "", err
		}
		return formatHits(hits, ""), nil
	default:
		return "", fmt.Errorf("unknown memory tool %q", call.Name)
	}
}

func formatRecall(res memory.RecallResult) string {
	var b strings.Builder
	if len(res.Hits) == 0 && len(res.AntiHits) == 0 {
		return "0 memories"
	}
	fmt.Fprintf(&b, "hits (%d) budget_tokens=%d\n", len(res.Hits), res.BudgetTokens)
	for _, hit := range res.Hits {
		fmt.Fprintf(&b, "#%d %s %s ~%d\n", hit.ID, hit.Kind, hit.Title, hit.Tokens)
	}
	if len(res.AntiHits) > 0 {
		fmt.Fprintf(&b, "anti_hits (%d)\n", len(res.AntiHits))
		for _, hit := range res.AntiHits {
			fmt.Fprintf(&b, "#%d %s %s ~%d\n", hit.ID, hit.Kind, hit.Title, hit.Tokens)
		}
	}
	b.WriteString("Fetch bodies with memory_get. Hints, not authority.")
	return b.String()
}

func formatHits(hits []memory.Hit, footer string) string {
	if len(hits) == 0 {
		return "0 memories"
	}
	var b strings.Builder
	for _, hit := range hits {
		fmt.Fprintf(&b, "#%d %s %s ~%d\n", hit.ID, hit.Kind, hit.Title, hit.Tokens)
	}
	if footer != "" {
		b.WriteString(footer)
	}
	return b.String()
}

func argInt(raw json.RawMessage, key string, fallback int) int {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return fallback
	}
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case string:
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return fallback
}

const mcpInstructions = `Superopen native code graph and memory. For architecture, callers/callees, symbol lookup, and "how does X work" questions: call graph_query first (stop if answered), else graph_search → graph_snippet → graph_trace (qualified names), or graph_architecture — BEFORE broad Read/Grep/Glob. For prior work in this repo: memory_recall or memory_search → memory_get by id. Distill at most once with memory_capture. Memory is hints, not authority. Graph builds are local (no LLM). Root is resolved from the agent working directory.`
