package tracestore

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ishanjainn/superopen/internal/artifactmeta"
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
		about := artifactmeta.About{Purpose: "Temporary event spool for telemetry whose session ID has not been resolved.", Authority: "temporary", UpdatedBy: "telemetry ingestion"}
		if filepath.Base(path) == "events.jsonl" {
			about = artifactmeta.About{Purpose: "Normalized prompts, responses, tool calls, file activity, usage, lifecycle, and audit events for this session.", Authority: "authoritative session event stream", UpdatedBy: "vendor telemetry adapter"}
		}
		if err := artifactmeta.EnsureJSONL(path, about); err != nil {
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
		if err := artifactmeta.EnsureJSONL(path, artifactmeta.About{Purpose: "Normalized prompts, responses, tool calls, file activity, usage, lifecycle, and audit events for this session.", Authority: "authoritative session event stream", UpdatedBy: "vendor telemetry adapter"}); err != nil {
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
	if err := artifactmeta.EnsureJSONL(tmp, artifactmeta.About{Purpose: "Temporary event spool for telemetry whose session ID has not been resolved.", Authority: "temporary", UpdatedBy: "telemetry ingestion"}); err != nil {
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
	var tokens int64
	var cost float64
	for _, sp := range spans {
		if v := sp.Attributes["gen_ai.usage.total_tokens"]; v != "" {
			var n int64
			fmt.Sscanf(v, "%d", &n)
			tokens += n
		}
		// Prefer gen_ai.usage.cost (semconv); accept alternate aliases.
		for _, key := range []string{"gen_ai.usage.cost", "gen_ai.usage.cost_usd", "coding_agent.session.cost_usd"} {
			if v := sp.Attributes[key]; v != "" {
				var c float64
				if _, err := fmt.Sscanf(v, "%f", &c); err == nil {
					cost += c
					break
				}
			}
		}
	}
	return tokens, cost, nil
}
