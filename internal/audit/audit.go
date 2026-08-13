// Package audit writes repository events to .so/audit/events.jsonl and
// session-associated audit events to that session's unified events.jsonl.
package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/ishanjainn/superopen/internal/artifactmeta"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/session"
)

type Event struct {
	At      time.Time         `json:"at"`
	Action  string            `json:"action"` // deny|allow|create|update|conflict_skip|injection_blocked|session.start_from_memory|…
	Key     string            `json:"key,omitempty"`
	Type    string            `json:"type,omitempty"` // semantic|episodic|lesson|policy|session|…
	Detail  string            `json:"detail,omitempty"`
	Vendor  string            `json:"vendor,omitempty"`
	Session string            `json:"session_id,omitempty"`
	Attrs   map[string]string `json:"attrs,omitempty"`
}

var mu sync.Mutex

func Path(paths harness.Paths) string {
	return paths.AuditEvents
}

// Ensure creates the self-describing repository audit stream even before the
// first event, making the initialized harness layout predictable.
func Ensure(paths harness.Paths) error {
	return artifactmeta.EnsureJSONL(paths.AuditEvents, artifactmeta.About{
		Purpose: "Audit events that are not associated with a coding session.", Authority: "append-only runtime history", UpdatedBy: "Superopen CLI and hooks",
	})
}

func Append(paths harness.Paths, ev Event) error {
	if ev.At.IsZero() {
		ev.At = time.Now().UTC()
	}
	mu.Lock()
	defer mu.Unlock()
	path := Path(paths)
	about := artifactmeta.About{Purpose: "Audit events that are not associated with a coding session.", Authority: "append-only runtime history", UpdatedBy: "Superopen CLI and hooks"}
	if ev.Session != "" {
		store := session.NewStore(paths)
		if _, err := store.Get(ev.Session); os.IsNotExist(err) {
			_ = store.Start(session.Meta{ID: ev.Session, Vendor: ev.Vendor, StartedAt: ev.At})
		}
		path = filepath.Join(paths.SessionDir(ev.Session), "events.jsonl")
		about = artifactmeta.About{Purpose: "Normalized prompts, responses, tool calls, file activity, usage, lifecycle, and audit events for this session.", Authority: "authoritative session event stream", UpdatedBy: "vendor telemetry adapter"}
	}
	if err := artifactmeta.EnsureJSONL(path, about); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(ev)
}

func List(paths harness.Paths, limit int) ([]Event, error) {
	var out []Event
	files := []string{paths.AuditEvents}
	_ = filepath.WalkDir(paths.SessionsDir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == "events.jsonl" {
			files = append(files, path)
		}
		return nil
	})
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range splitLines(data) {
			var ev Event
			if len(line) > 0 && json.Unmarshal(line, &ev) == nil && ev.Action != "" {
				out = append(out, ev)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
