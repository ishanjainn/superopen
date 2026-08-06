// Package audit writes append-only SEL-style events under .so/audit/.
package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/superopen/so/internal/harness"
)

type Event struct {
	At       time.Time         `json:"at"`
	Action   string            `json:"action"` // deny|allow|create|update|conflict_skip|injection_blocked|session.start_from_memory|…
	Key      string            `json:"key,omitempty"`
	Type     string            `json:"type,omitempty"` // semantic|episodic|lesson|policy|session|…
	Detail   string            `json:"detail,omitempty"`
	Vendor   string            `json:"vendor,omitempty"`
	Session  string            `json:"session_id,omitempty"`
	Attrs    map[string]string `json:"attrs,omitempty"`
}

var mu sync.Mutex

func Path(paths harness.Paths) string {
	return filepath.Join(paths.AuditDir, "events.jsonl")
}

func Append(paths harness.Paths, ev Event) error {
	if err := os.MkdirAll(paths.AuditDir, 0o755); err != nil {
		return err
	}
	if ev.At.IsZero() {
		ev.At = time.Now().UTC()
	}
	mu.Lock()
	defer mu.Unlock()
	f, err := os.OpenFile(Path(paths), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(ev)
}

func List(paths harness.Paths, limit int) ([]Event, error) {
	data, err := os.ReadFile(Path(paths))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Event
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var ev Event
		if json.Unmarshal(line, &ev) == nil {
			out = append(out, ev)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	// newest first
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
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
