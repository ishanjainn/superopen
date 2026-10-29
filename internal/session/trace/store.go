package trace

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ishanjainn/superopen/internal/agent/pricing"
	"github.com/ishanjainn/superopen/internal/artifact"
)

// Span is a normalized OpenTelemetry span record stored in JSONL.
type Span struct {
	TraceID        string            `json:"trace_id"`
	SpanID         string            `json:"span_id"`
	ParentSpanID   string            `json:"parent_span_id,omitempty"`
	Name           string            `json:"name"`
	StartTimeUnixN int64             `json:"start_time_unix_nano"`
	EndTimeUnixN   int64             `json:"end_time_unix_nano"`
	Attributes     map[string]string `json:"attributes,omitempty"`
	Status         string            `json:"status,omitempty"`
	SessionID      string            `json:"session_id,omitempty"`
}

// Store writes and queries spans. MVP: LocalJSONL.
type Store interface {
	Write(spans []Span) error
	Query(filter QueryFilter) ([]Span, error)
	SessionCost(sessionID string) (tokens int64, costUSD float64, err error)
}

type QueryFilter struct {
	SessionID string
	Since     time.Time
	Until     time.Time
	Limit     int
}

// LocalJSONL stores normalized spans in <sessions>/<id>/events.jsonl. Events
// without a resolvable session id are held lazily in sessions/inbox.jsonl.
type LocalJSONL struct {
	Dir string
	mu  sync.Mutex
}

func NewLocalJSONL(dir string) *LocalJSONL {
	return &LocalJSONL{Dir: dir}
}

func (s *LocalJSONL) Write(spans []Span) error {
	spans = DedupSpans(spans)
	if len(spans) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	byPath := map[string][]Span{}
	resolvedByTrace := map[string]string{}
	for _, sp := range spans {
		sid := storedSessionID(sp)
		path := filepath.Join(s.Dir, "inbox.jsonl")
		if sid != "" {
			path = filepath.Join(s.Dir, safeID(sid), "events.jsonl")
			if sp.TraceID != "" {
				resolvedByTrace[sp.TraceID] = sid
			}
		}
		byPath[path] = append(byPath[path], sp)
	}
	for path, group := range byPath {
		about := artifact.About{Purpose: "Temporary event spool for telemetry whose session ID has not been resolved.", Authority: "temporary", UpdatedBy: "telemetry ingestion"}
		if filepath.Base(path) == "events.jsonl" {
			about = artifact.About{Purpose: "Normalized prompts, responses, tool calls, file activity, usage, lifecycle, and audit events for this session.", Authority: "authoritative session event stream", UpdatedBy: "vendor telemetry adapter"}
		}
		if err := artifact.EnsureJSONL(path, about); err != nil {
			return err
		}
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		enc := json.NewEncoder(f)
		for _, sp := range group {
			if err := enc.Encode(sp); err != nil {
				_ = f.Close()
				return err
			}
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return s.resolveInbox(resolvedByTrace)
}

func (s *LocalJSONL) resolveInbox(resolvedByTrace map[string]string) error {
	if len(resolvedByTrace) == 0 {
		return nil
	}
	inbox := filepath.Join(s.Dir, "inbox.jsonl")
	f, err := os.Open(inbox)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var remaining []Span
	move := map[string][]Span{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var sp Span
		if json.Unmarshal(sc.Bytes(), &sp) != nil || sp.Name == "" {
			continue // file manifest
		}
		if sid := resolvedByTrace[sp.TraceID]; sid != "" {
			sp.SessionID = sid
			move[sid] = append(move[sid], sp)
		} else {
			remaining = append(remaining, sp)
		}
	}
	closeErr := f.Close()
	if sc.Err() != nil {
		return sc.Err()
	}
	if closeErr != nil {
		return closeErr
	}
	for sid, group := range move {
		path := filepath.Join(s.Dir, safeID(sid), "events.jsonl")
		if err := artifact.EnsureJSONL(path, artifact.About{Purpose: "Normalized prompts, responses, tool calls, file activity, usage, lifecycle, and audit events for this session.", Authority: "authoritative session event stream", UpdatedBy: "vendor telemetry adapter"}); err != nil {
			return err
		}
		out, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		enc := json.NewEncoder(out)
		for _, sp := range group {
			if err := enc.Encode(sp); err != nil {
				_ = out.Close()
				return err
			}
		}
		if err := out.Close(); err != nil {
			return err
		}
	}
	if len(remaining) == 0 {
		return os.Remove(inbox)
	}
	tmp := inbox + ".tmp"
	_ = os.Remove(tmp)
	if err := artifact.EnsureJSONL(tmp, artifact.About{Purpose: "Temporary event spool for telemetry whose session ID has not been resolved.", Authority: "temporary", UpdatedBy: "telemetry ingestion"}); err != nil {
		return err
	}
	out, err := os.OpenFile(tmp, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(out)
	for _, sp := range remaining {
		if err := enc.Encode(sp); err != nil {
			_ = out.Close()
			return err
		}
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, inbox)
}

func (s *LocalJSONL) Query(filter QueryFilter) ([]Span, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := os.Stat(s.Dir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Span
	var files []string
	_ = filepath.WalkDir(s.Dir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && (d.Name() == "events.jsonl" || d.Name() == "inbox.jsonl") {
			files = append(files, path)
		}
		return nil
	})
	for _, path := range files {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
		for sc.Scan() {
			var sp Span
			if err := json.Unmarshal(sc.Bytes(), &sp); err != nil {
				continue
			}
			if filter.SessionID != "" {
				sid := resolvedSessionID(sp)
				// Match either the rollup conversation id or the process session id
				// so cost/finalize still find spans stamped before conversation-first
				// ResolveSessionID.
				if sid != filter.SessionID &&
					sp.Attributes["gen_ai.conversation.id"] != filter.SessionID &&
					sp.Attributes["coding_agent.session.id"] != filter.SessionID &&
					sp.Attributes["coding_agent.session_id"] != filter.SessionID &&
					sp.SessionID != filter.SessionID && sp.TraceID != filter.SessionID {
					continue
				}
			}
			if !filter.Since.IsZero() && time.Unix(0, sp.StartTimeUnixN).Before(filter.Since) {
				continue
			}
			if !filter.Until.IsZero() && time.Unix(0, sp.StartTimeUnixN).After(filter.Until) {
				continue
			}
			out = append(out, sp)
			if filter.Limit > 0 && len(out) >= filter.Limit {
				f.Close()
				return out, nil
			}
		}
		f.Close()
		if err := sc.Err(); err != nil {
			return out, err
		}
	}
	return out, nil
}

// UpdateUsage fills missing turn or session counters in the saved event stream
// without appending a duplicate span or dropping tool events.
func (s *LocalJSONL) UpdateUsage(sessionID string, spans []Span) error {
	updates := make(map[string]map[string]string)
	for _, sp := range spans {
		if (sp.Name == "coding_agent.llm.turn" || sp.Name == "coding_agent.session") && sp.SpanID != "" && hasUsageTokens(&sp) {
			updates[sp.SpanID] = sp.Attributes
		}
	}
	if len(updates) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.Dir, safeID(sessionID), "events.jsonl")
	src, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return err
	}
	dst, err := os.CreateTemp(filepath.Dir(path), ".usage-*.jsonl")
	if err != nil {
		return err
	}
	defer os.Remove(dst.Name())
	defer dst.Close()
	if err := dst.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	scanner := bufio.NewScanner(src)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		var sp Span
		if json.Unmarshal(line, &sp) == nil && (sp.Name == "coding_agent.llm.turn" || sp.Name == "coding_agent.session") {
			if attrs, ok := updates[sp.SpanID]; ok {
				if sp.Attributes == nil {
					sp.Attributes = make(map[string]string)
				}
				for key, value := range attrs {
					if strings.HasPrefix(key, "gen_ai.usage.") {
						sp.Attributes[key] = value
					}
				}
				if value := attrs["coding_agent.session.cost_usd"]; value != "" && sp.Name == "coding_agent.session" {
					sp.Attributes["coding_agent.session.cost_usd"] = value
				}
				line, err = json.Marshal(sp)
				if err != nil {
					return err
				}
			}
		}
		if _, err := dst.Write(append(line, '\n')); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if err := src.Close(); err != nil {
		return err
	}
	if err := dst.Close(); err != nil {
		return err
	}
	return os.Rename(dst.Name(), path)
}

// LatestSessionID returns the session owning the newest stored event without
// applying a global result cap. Callers can then query that session exactly.
func (s *LocalJSONL) LatestSessionID() (string, error) {
	spans, err := s.Query(QueryFilter{})
	if err != nil {
		return "", err
	}
	var latestID string
	var latestAt int64
	for _, sp := range spans {
		id := resolvedSessionID(sp)
		if id == "" {
			continue
		}
		at := sp.StartTimeUnixN
		if sp.EndTimeUnixN > at {
			at = sp.EndTimeUnixN
		}
		if latestID == "" || at > latestAt {
			latestID, latestAt = id, at
		}
	}
	return latestID, nil
}

func resolvedSessionID(sp Span) string {
	if id := storedSessionID(sp); id != "" {
		return id
	}
	return strings.TrimSpace(sp.TraceID)
}

func storedSessionID(sp Span) string {
	if strings.TrimSpace(sp.SessionID) != "" {
		return strings.TrimSpace(sp.SessionID)
	}
	for _, k := range []string{"gen_ai.conversation.id", "coding_agent.session.id", "coding_agent.session_id"} {
		if v := strings.TrimSpace(sp.Attributes[k]); v != "" {
			return v
		}
	}
	return ""
}

func safeID(id string) string {
	id = strings.TrimSpace(id)
	id = strings.ReplaceAll(id, "/", "_")
	id = strings.ReplaceAll(id, "\\", "_")
	if id == "." || id == ".." || id == "" {
		return "unknown"
	}
	return id
}

func (s *LocalJSONL) SessionCost(sessionID string) (int64, float64, error) {
	spans, err := s.Query(QueryFilter{SessionID: sessionID})
	if err != nil {
		return 0, 0, err
	}
	tokens, cost := CostFromSpans(spans)
	return tokens, cost, nil
}

// CostFromSpans prefers a session rollup and falls back to individual LLM turns.
// The fallback also prices turns captured before their model entered the rate table.
func CostFromSpans(spans []Span) (int64, float64) {
	var root, stop *Span
	var turnTokens int64
	var turnCost float64
	seenTurns := make(map[string]bool)
	for i := range spans {
		sp := &spans[i]
		if !hasUsageTokens(sp) {
			continue
		}
		switch sp.Name {
		case "coding_agent.session":
			root = sp
		case "coding_agent.session.loop.stop":
			if stop == nil || sp.StartTimeUnixN >= stop.StartTimeUnixN {
				stop = sp
			}
		case "coding_agent.llm.turn":
			if sp.SpanID != "" && seenTurns[sp.SpanID] {
				continue
			}
			seenTurns[sp.SpanID] = true
			tokens, _ := strconv.ParseInt(sp.Attributes["gen_ai.usage.total_tokens"], 10, 64)
			turnTokens += tokens
			cost, _ := strconv.ParseFloat(sp.Attributes["gen_ai.usage.cost"], 64)
			if cost == 0 {
				model := sp.Attributes["gen_ai.request.model"]
				if model == "" {
					model = sp.Attributes["gen_ai.response.model"]
				}
				input, _ := strconv.ParseInt(sp.Attributes["gen_ai.usage.input_tokens"], 10, 64)
				output, _ := strconv.ParseInt(sp.Attributes["gen_ai.usage.output_tokens"], 10, 64)
				cacheRead, _ := strconv.ParseInt(sp.Attributes["gen_ai.usage.cache.read_input_tokens"], 10, 64)
				cacheWrite, _ := strconv.ParseInt(sp.Attributes["gen_ai.usage.cache.creation_input_tokens"], 10, 64)
				cost = pricing.Lookup(model).Cost(input, output, cacheRead, cacheWrite)
			}
			turnCost += cost
		}
	}
	chosen := root
	if chosen == nil {
		chosen = stop
	}
	if chosen == nil {
		return turnTokens, turnCost
	}
	var tokens int64
	fmt.Sscanf(chosen.Attributes["gen_ai.usage.total_tokens"], "%d", &tokens)
	var cost float64
	for _, key := range []string{"gen_ai.usage.cost", "gen_ai.usage.cost_usd", "coding_agent.session.cost_usd"} {
		if v := chosen.Attributes[key]; v != "" {
			if _, err := fmt.Sscanf(v, "%f", &cost); err == nil {
				break
			}
		}
	}
	if cost == 0 {
		cost = turnCost
	}
	return tokens, cost
}

func hasUsageTokens(sp *Span) bool {
	if sp == nil || sp.Attributes == nil {
		return false
	}
	return strings.TrimSpace(sp.Attributes["gen_ai.usage.total_tokens"]) != ""
}
