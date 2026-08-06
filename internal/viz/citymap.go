package viz

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/superopen/so/internal/harness"
	"github.com/superopen/so/internal/session"
	"github.com/superopen/so/internal/tracestore"
)

// Citymap is a simplified spatial layout of the repo.
type Citymap struct {
	Root  string         `json:"root"`
	Nodes []CitymapNode  `json:"nodes"`
}

type CitymapNode struct {
	ID       string  `json:"id"`
	Path     string  `json:"path"`
	Parent   string  `json:"parent,omitempty"`
	IsDir    bool    `json:"is_dir"`
	Lines    int     `json:"lines,omitempty"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Z        float64 `json:"z"`
}

// Replay is a touch trace built from session telemetry.
type Replay struct {
	SessionID string       `json:"session_id"`
	Events    []ReplayEvent `json:"events"`
}

type ReplayEvent struct {
	T     int64  `json:"t"` // unix ms
	Path  string `json:"path"`
	State string `json:"state"` // seen | read | edited
	Span  string `json:"span,omitempty"`
}

// BuildCitymap walks the repo and writes .so/viz/citymap.json.
func BuildCitymap(repoRoot string, paths harness.Paths) error {
	if err := os.MkdirAll(paths.VizDir, 0o755); err != nil {
		return err
	}
	cm := Citymap{Root: repoRoot}
	idx := 0
	_ = filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(repoRoot, path)
		if rel == "." {
			return nil
		}
		base := filepath.Base(path)
		if d.IsDir() {
			if base == ".git" || base == "node_modules" || base == "vendor" || base == ".so" || base == "graphify-out" || base == "dist" {
				return filepath.SkipDir
			}
		}
		parent := filepath.Dir(rel)
		if parent == "." {
			parent = ""
		}
		x := float64(idx%32) * 12
		y := float64(idx/32) * 12
		z := float64(strings.Count(rel, string(os.PathSeparator))) * 8
		cm.Nodes = append(cm.Nodes, CitymapNode{
			ID: rel, Path: rel, Parent: parent, IsDir: d.IsDir(),
			X: x, Y: y, Z: z,
		})
		idx++
		return nil
	})
	data, err := json.MarshalIndent(cm, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(paths.Citymap, data, 0o644)
}

// BuildReplayFromSpans creates replay.json for a session from OTLP spans (post-session).
func BuildReplayFromSpans(paths harness.Paths, sessionID string, spans []tracestore.Span) (Replay, error) {
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
	dir := paths.SessionDir(sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return r, err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return r, err
	}
	return r, os.WriteFile(filepath.Join(dir, "replay.json"), data, 0o644)
}

// BuildReplayFromFootprint is a helper when only footprint is available.
func BuildReplayFromFootprint(paths harness.Paths, meta session.Meta, fp session.Footprint) error {
	r := Replay{SessionID: meta.ID}
	t := meta.StartedAt.UnixMilli()
	for i, f := range fp.Files {
		r.Events = append(r.Events, ReplayEvent{
			T: t + int64(i)*1000, Path: f.Path, State: f.State,
		})
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(paths.SessionDir(meta.ID), "replay.json"), data, 0o644)
}
