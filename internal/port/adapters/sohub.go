package adapters

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ishanjainn/superopen/internal/artifactmeta"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/port"
	"github.com/ishanjainn/superopen/internal/session"
)

// SOHubImport reads .so/sessions as a portable hub.
type SOHubImport struct {
	RepoRoot string
}

func (s SOHubImport) Harness() port.HarnessID { return port.HarnessSOHub }

func (s SOHubImport) paths() harness.Paths {
	wd, _ := os.Getwd()
	if s.RepoRoot != "" {
		wd = s.RepoRoot
	}
	return harness.Resolve(wd)
}

func (s SOHubImport) Detect() (bool, error) {
	return s.paths().Exists(), nil
}

func (s SOHubImport) Discover() ([]port.SessionRef, error) {
	ci := CursorImport{SORoot: s.paths().Root}
	refs, err := ci.Discover()
	for i := range refs {
		refs[i].Harness = port.HarnessSOHub
	}
	return refs, err
}

func (s SOHubImport) Parse(ref port.SessionRef) (port.PortableSession, error) {
	ci := CursorImport{SORoot: s.paths().Root}
	ps, err := ci.Parse(ref)
	ps.SourceHarness = port.HarnessSOHub
	return ps, err
}

// SOHubExport materializes PortableSession into .so/sessions.
type SOHubExport struct {
	RepoRoot string
}

func (s SOHubExport) Harness() port.HarnessID { return port.HarnessSOHub }

func (s SOHubExport) paths() harness.Paths {
	wd, _ := os.Getwd()
	if s.RepoRoot != "" {
		wd = s.RepoRoot
	}
	return harness.Resolve(wd)
}

func (s SOHubExport) Detect() (bool, error) {
	return s.paths().Exists(), nil
}

func (s SOHubExport) Write(ps port.PortableSession, opts port.WriteOptions) (port.ExportResult, error) {
	paths := s.paths()
	destID := opts.ExistingDestID
	if destID == "" || opts.Force {
		destID = fmt.Sprintf("port-%s-%d", ps.SourceHarness, time.Now().UnixNano())
	}
	store := session.NewStore(paths)
	preview := ""
	if len(ps.Turns) > 0 {
		preview = firstLine(ps.Turns[0].Text, 160)
	}
	meta := session.Meta{
		ID: destID, Vendor: string(ps.SourceHarness), Title: ps.Title, PromptPreview: preview,
		StartedAt: time.Now().UTC(), RepoRoot: ps.CWD,
	}
	_ = store.Start(meta)
	dir := paths.SessionDir(destID)
	_ = os.MkdirAll(dir, 0o755)
	tf, err := os.Create(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		return port.ExportResult{}, err
	}
	enc := json.NewEncoder(tf)
	_ = enc.Encode(artifactmeta.JSONLManifest{
		Type: "superopen.file_manifest", Purpose: "Normalized prompts, responses, tool calls, file activity, usage, lifecycle, and audit events for this session.",
		Authority: "authoritative session event stream", UpdatedBy: "vendor telemetry adapter",
	})
	for _, t := range ps.Turns {
		_ = enc.Encode(map[string]any{"role": t.Role, "text": t.Text, "timestamp": t.Timestamp, "model": t.Model})
	}
	_ = tf.Close()
	prov, _ := json.Marshal(map[string]any{
		"source_harness": ps.SourceHarness, "source_session_id": ps.SourceSessionID, "source_path": ps.SourcePath,
		"working_state": ps.WorkingState, "dropped_turns": ps.DroppedTurns,
	})
	_ = store.WriteDocument(destID, func(d *session.Document) { d.Port = prov })
	_ = store.UpdateMeta(meta)
	return port.ExportResult{DestSessionID: destID}, nil
}
