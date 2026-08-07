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
)

// Span is a simplified OTLP-compatible span record stored in JSONL.
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

// LocalJSONL stores one JSON object per line under dir/YYYY-MM-DD.jsonl.
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
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	day := time.Now().UTC().Format("2006-01-02")
	path := filepath.Join(s.Dir, day+".jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, sp := range spans {
		if err := enc.Encode(sp); err != nil {
			return err
		}
	}
	return nil
}

func (s *LocalJSONL) Query(filter QueryFilter) ([]Span, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Span
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(s.Dir, e.Name())
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
				sid := sp.SessionID
				if sid == "" {
					sid = sp.Attributes["gen_ai.conversation.id"]
				}
				if sid == "" {
					sid = sp.Attributes["coding_agent.session.id"]
				}
				if sid == "" {
					sid = sp.Attributes["coding_agent.session_id"]
				}
				// Match either the rollup conversation id or the process session id
				// so cost/finalize still find spans stamped before conversation-first
				// ResolveSessionID.
				if sid != filter.SessionID &&
					sp.Attributes["gen_ai.conversation.id"] != filter.SessionID &&
					sp.Attributes["coding_agent.session.id"] != filter.SessionID &&
					sp.SessionID != filter.SessionID {
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

// Fanout writes to multiple stores (local + future remote OTLP).
type Fanout struct {
	Stores []Store
}

func (f *Fanout) Write(spans []Span) error {
	var first error
	for _, s := range f.Stores {
		if err := s.Write(spans); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (f *Fanout) Query(filter QueryFilter) ([]Span, error) {
	if len(f.Stores) == 0 {
		return nil, nil
	}
	return f.Stores[0].Query(filter)
}

func (f *Fanout) SessionCost(sessionID string) (int64, float64, error) {
	if len(f.Stores) == 0 {
		return 0, 0, nil
	}
	return f.Stores[0].SessionCost(sessionID)
}
