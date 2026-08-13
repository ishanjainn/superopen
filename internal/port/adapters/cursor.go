package adapters

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/artifactmeta"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/port"
	"github.com/ishanjainn/superopen/internal/session"
)

// CursorImport reads Superopen-materialized .so/sessions and Cursor so-port packs.
type CursorImport struct {
	SORoot string // absolute .so root; empty = discover from cwd
}

func (c CursorImport) Harness() port.HarnessID { return port.HarnessCursor }

func (c CursorImport) soRoot() string {
	if c.SORoot != "" {
		return c.SORoot
	}
	wd, _ := os.Getwd()
	return harness.Resolve(wd).Root
}

func (c CursorImport) repoRoot() string {
	return filepath.Dir(c.soRoot())
}

func (c CursorImport) Detect() (bool, error) {
	p := c.soRoot()
	_, err := os.Stat(p)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		// Still detectable if Cursor project dir exists.
		if _, err2 := os.Stat(filepath.Join(c.repoRoot(), ".cursor")); err2 == nil {
			return true, nil
		}
		return false, nil
	}
	return false, err
}

func (c CursorImport) Discover() ([]port.SessionRef, error) {
	var refs []port.SessionRef
	dir := filepath.Join(c.soRoot(), "sessions")
	ents, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, e := range ents {
		if !e.IsDir() || e.Name() == "." {
			continue
		}
		metaPath := filepath.Join(dir, e.Name(), "session.json")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}
		var meta session.Meta
		if json.Unmarshal(data, &meta) != nil {
			continue
		}
		title := meta.Title
		if title == "" {
			title = meta.PromptPreview
		}
		refs = append(refs, port.SessionRef{
			Harness:         port.HarnessCursor,
			SourceSessionID: meta.ID,
			SourcePath:      filepath.Join(dir, e.Name()),
			Title:           firstLine(title, 80),
			CWD:             meta.RepoRoot,
			UpdatedAt:       meta.StartedAt.UnixMilli(),
		})
	}
	// Also discover first-class port packs under .cursor/so-port/
	portRoot := filepath.Join(c.repoRoot(), ".cursor", "so-port")
	if ents, err := os.ReadDir(portRoot); err == nil {
		for _, e := range ents {
			if !e.IsDir() {
				continue
			}
			metaPath := filepath.Join(portRoot, e.Name(), "session.json")
			data, err := os.ReadFile(metaPath)
			if err != nil {
				continue
			}
			var meta map[string]any
			if json.Unmarshal(data, &meta) != nil {
				continue
			}
			title, _ := meta["title"].(string)
			refs = append(refs, port.SessionRef{
				Harness:         port.HarnessCursor,
				SourceSessionID: e.Name(),
				SourcePath:      filepath.Join(portRoot, e.Name()),
				Title:           firstLine(title, 80),
				CWD:             c.repoRoot(),
				UpdatedAt:       time.Now().UnixMilli(),
			})
		}
	}
	return refs, nil
}

func (c CursorImport) Parse(ref port.SessionRef) (port.PortableSession, error) {
	sess := port.NewPortableSession(port.HarnessCursor, ref.SourceSessionID, ref.SourcePath, ref.CWD, ref.Title)
	transcript := filepath.Join(ref.SourcePath, "events.jsonl")
	data, err := os.ReadFile(transcript)
	if err != nil {
		metaPath := filepath.Join(ref.SourcePath, "session.json")
		if md, err2 := os.ReadFile(metaPath); err2 == nil {
			var meta session.Meta
			_ = json.Unmarshal(md, &meta)
			if meta.PromptPreview != "" {
				sess.Turns = append(sess.Turns, port.PortableTurn{Role: "user", Text: meta.PromptPreview})
				sess.Title = firstLine(meta.PromptPreview, 80)
			}
		}
		return sess, nil
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row map[string]any
		if json.Unmarshal([]byte(line), &row) != nil {
			continue
		}
		role, _ := row["role"].(string)
		text, _ := row["text"].(string)
		if text == "" {
			text, _ = row["content"].(string)
		}
		if role == "" {
			if name, _ := row["name"].(string); strings.Contains(name, "prompt") {
				role = "user"
			} else {
				role = "assistant"
			}
		}
		if text == "" {
			if attrs, ok := row["attributes"].(map[string]any); ok {
				if v, ok := attrs["gen_ai.prompt"].(string); ok {
					text = v
					role = "user"
				}
			}
		}
		if strings.TrimSpace(text) == "" {
			sess.DroppedTurns++
			continue
		}
		if role != "user" && role != "assistant" {
			role = "assistant"
		}
		sess.Turns = append(sess.Turns, port.PortableTurn{Role: role, Text: text})
	}
	ensureMeta(&sess)
	loadWorkingStateSidecar(&sess, ref.SourcePath)
	return sess, nil
}

// loadWorkingStateSidecar restores working state written by SOHubExport, since
// events.jsonl's role/text rows have no field for it.
func loadWorkingStateSidecar(sess *port.PortableSession, sourceDir string) {
	if raw, err := os.ReadFile(filepath.Join(sourceDir, "session.json")); err == nil {
		var doc struct {
			Port struct {
				WorkingState port.WorkingState `json:"working_state"`
				DroppedTurns int               `json:"dropped_turns"`
			} `json:"port"`
		}
		if json.Unmarshal(raw, &doc) == nil && (!doc.Port.WorkingState.Empty() || doc.Port.DroppedTurns > 0) {
			sess.WorkingState = doc.Port.WorkingState
			if doc.Port.DroppedTurns > sess.DroppedTurns {
				sess.DroppedTurns = doc.Port.DroppedTurns
			}
			return
		}
	}
	raw, err := os.ReadFile(filepath.Join(sourceDir, "working-state.json"))
	if err != nil {
		return
	}
	var decoded struct {
		WorkingState port.WorkingState `json:"working_state"`
		DroppedTurns int               `json:"dropped_turns"`
	}
	if json.Unmarshal(raw, &decoded) != nil {
		return
	}
	sess.WorkingState = decoded.WorkingState
	if decoded.DroppedTurns > sess.DroppedTurns {
		sess.DroppedTurns = decoded.DroppedTurns
	}
}

// CursorExport writes a resumable Cursor session pack under .cursor/so-port/
// and mirrors into .so/sessions. Cursor has no public chat-import API; the
// resume pack is first-class: SessionStart injects conversation.md when PENDING
// is set (so sessions resume --vendor=cursor --id=…).
type CursorExport struct {
	SORoot   string
	RepoRoot string
}

func (c CursorExport) Harness() port.HarnessID { return port.HarnessCursor }

func (c CursorExport) paths() harness.Paths {
	wd, _ := os.Getwd()
	if c.RepoRoot != "" {
		wd = c.RepoRoot
	} else if c.SORoot != "" {
		wd = filepath.Dir(c.SORoot)
	}
	return harness.Resolve(wd)
}

func (c CursorExport) Detect() (bool, error) {
	p := c.paths()
	if !p.Exists() {
		return false, nil
	}
	return true, nil
}

func (c CursorExport) Write(ps port.PortableSession, opts port.WriteOptions) (port.ExportResult, error) {
	paths := c.paths()
	repoRoot := filepath.Dir(paths.Root)
	destID := opts.ExistingDestID
	if destID == "" || opts.Force {
		destID = fmt.Sprintf("cursor-port-%d", time.Now().UnixNano())
	}

	// Hub mirror (.so/sessions) - same fidelity as SO hub spoke.
	store := session.NewStore(paths)
	preview := ""
	if len(ps.Turns) > 0 {
		preview = firstLine(ps.Turns[0].Text, 160)
	}
	meta := session.Meta{
		ID: destID, Vendor: "cursor", Title: ps.Title, PromptPreview: preview,
		StartedAt: time.Now().UTC(), RepoRoot: ps.CWD, Status: session.StatusActive,
	}
	if meta.Title == "" {
		meta.Title = preview
	}
	_ = store.Start(meta)

	dir := paths.SessionDir(destID)
	_ = os.MkdirAll(dir, 0o755)
	if err := writeTranscript(dir, ps); err != nil {
		return port.ExportResult{}, err
	}
	if prov, err := json.Marshal(map[string]any{
		"source_harness": ps.SourceHarness, "source_session_id": ps.SourceSessionID, "source_path": ps.SourcePath,
		"working_state": ps.WorkingState, "dropped_turns": ps.DroppedTurns,
	}); err == nil {
		_ = store.WriteDocument(destID, func(d *session.Document) { d.Port = prov })
	}

	// Native Cursor resume pack under project .cursor/so-port/<id>/
	portDir := filepath.Join(repoRoot, ".cursor", "so-port", destID)
	_ = os.MkdirAll(portDir, 0o755)
	if err := writeTranscript(portDir, ps); err != nil {
		return port.ExportResult{}, err
	}
	writeWorkingStateSidecar(portDir, ps)
	portMeta, _ := json.MarshalIndent(map[string]any{
		"id": destID, "title": meta.Title, "source": ps.SourceHarness,
		"source_session_id": ps.SourceSessionID, "turns": len(ps.Turns),
		"resume": fmt.Sprintf("so sessions resume --vendor=cursor --id=%s", destID),
	}, "", "  ")
	_ = os.WriteFile(filepath.Join(portDir, "session.json"), portMeta, 0o644)

	var conv strings.Builder
	conv.WriteString("# Ported conversation (Cursor resume pack)\n\n")
	conv.WriteString(fmt.Sprintf("Source: %s / %s\n\n", ps.SourceHarness, ps.SourceSessionID))
	for _, t := range ps.Turns {
		conv.WriteString("## " + t.Role + "\n\n")
		conv.WriteString(t.Text)
		conv.WriteString("\n\n")
	}
	_ = os.WriteFile(filepath.Join(portDir, "conversation.md"), []byte(conv.String()), 0o644)

	resume := fmt.Sprintf(`# Resume this ported session

Port armed a one-shot SessionStart inject under .so/port/ (and .cursor/so-port/
when the destination is Cursor). Start any coding agent with Superopen hooks in:

   %s

Optional manual re-arm:
   so sessions resume --vendor=cursor --id=%s

Transcript: .cursor/so-port/%s/events.jsonl
Hub mirror: .so/sessions/%s/
`, repoRoot, destID, destID, destID)
	_ = os.WriteFile(filepath.Join(portDir, "RESUME.md"), []byte(resume), 0o644)
	_ = store.UpdateMeta(meta)
	// PENDING / SessionStart inject is armed by port.ArmResume in the orchestrator
	// for every destination harness (including Cursor).
	return port.ExportResult{DestSessionID: destID}, nil
}

func writeTranscript(dir string, ps port.PortableSession) error {
	tf, err := os.Create(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		return err
	}
	defer tf.Close()
	enc := json.NewEncoder(tf)
	if err := enc.Encode(artifactmeta.JSONLManifest{
		Type: "superopen.file_manifest", Purpose: "Normalized prompts, responses, tool calls, file activity, usage, lifecycle, and audit events for this session.",
		Authority: "authoritative session event stream", UpdatedBy: "vendor telemetry adapter",
	}); err != nil {
		return err
	}
	for _, t := range ps.Turns {
		if err := enc.Encode(map[string]any{"role": t.Role, "text": t.Text, "timestamp": t.Timestamp}); err != nil {
			return err
		}
	}
	return nil
}

// writeWorkingStateSidecar persists recovered files/commands in native Cursor
// resume packs outside .so. Hub sessions embed this data in session.json.
func writeWorkingStateSidecar(dir string, ps port.PortableSession) {
	if ps.WorkingState.Empty() && ps.DroppedTurns == 0 {
		return
	}
	ws, err := json.Marshal(map[string]any{
		"working_state": ps.WorkingState,
		"dropped_turns": ps.DroppedTurns,
	})
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, "working-state.json"), ws, 0o644)
}
