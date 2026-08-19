// Package viz builds session map replay traces from local session telemetry.
package replay

import (
	"encoding/json"
	"strings"

	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/session"
	"github.com/ishanjainn/superopen/internal/session/trace"
)

// Replay is a touch trace built from session telemetry.
type Replay struct {
	SessionID string        `json:"session_id"`
	Events    []ReplayEvent `json:"events"`
}

type ReplayEvent struct {
	T     int64  `json:"t"` // unix ms
	Path  string `json:"path"`
	State string `json:"state"` // seen | read | edited
	Span  string `json:"span,omitempty"`
}

// BuildReplayFromSpans embeds replay data in session.json from local hook spans.
func BuildReplayFromSpans(paths paths.Paths, sessionID string, spans []trace.Span) (Replay, error) {
	r := Replay{SessionID: sessionID}
	for _, sp := range spans {
		path := sp.Attributes["coding_agent.file_path"]
		if path == "" {
			continue
		}
		state := "seen"
		name := strings.ToLower(sp.Name)
		switch {
		case strings.Contains(name, "edit") || strings.Contains(name, "write"):
			state = "edited"
		case strings.Contains(name, "read"):
			state = "read"
		case strings.Contains(name, "search") || strings.Contains(name, "glob") || strings.Contains(name, "grep"):
			state = "seen"
		}
		r.Events = append(r.Events, ReplayEvent{
			T:     sp.StartTimeUnixN / 1e6,
			Path:  path,
			State: state,
			Span:  sp.SpanID,
		})
	}
	data, err := json.Marshal(r)
	if err != nil {
		return r, err
	}
	return r, session.NewStore(paths).WriteDocument(sessionID, func(d *session.Document) { d.Replay = data })
}

// BuildReplayFromFootprint is a helper when only footprint is available.
func BuildReplayFromFootprint(paths paths.Paths, meta session.Meta, fp session.Footprint) error {
	r := Replay{SessionID: meta.ID}
	t := meta.StartedAt.UnixMilli()
	for i, f := range fp.Files {
		r.Events = append(r.Events, ReplayEvent{
			T: t + int64(i)*1000, Path: f.Path, State: f.State,
		})
	}
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return session.NewStore(paths).WriteDocument(meta.ID, func(d *session.Document) { d.Replay = data })
}
