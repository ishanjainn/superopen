package adapters

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/port"
)

type OpenCodeImport struct{}

func (OpenCodeImport) Harness() port.HarnessID { return port.HarnessOpenCode }

func opencodeDataDir() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, "opencode")
	}
	return filepath.Join(home(), ".local", "share", "opencode")
}

func (OpenCodeImport) Detect() (bool, error) {
	candidates := []string{
		os.Getenv("OPENCODE_DB"),
		filepath.Join(opencodeDataDir(), "opencode.db"),
		filepath.Join(home(), ".opencode"),
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if _, err := os.Stat(c); err == nil {
			return true, nil
		}
	}
	// Also accept export JSON dumps under ~/.opencode/sessions
	p := filepath.Join(home(), ".opencode", "sessions")
	if _, err := os.Stat(p); err == nil {
		return true, nil
	}
	return false, nil
}

func (OpenCodeImport) Discover() ([]port.SessionRef, error) {
	// Prefer JSON session dumps; also best-effort export from sqlite via sqlite3 CLI.
	_ = exportOpenCodeSQLiteJSON()
	root := filepath.Join(home(), ".opencode", "sessions")
	var refs []port.SessionRef
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".json") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var doc map[string]any
		if json.Unmarshal(data, &doc) != nil {
			return nil
		}
		infoM, _ := doc["info"].(map[string]any)
		if infoM == nil {
			return nil
		}
		id, _ := infoM["id"].(string)
		title, _ := infoM["title"].(string)
		cwd, _ := infoM["directory"].(string)
		if cwd == "" {
			cwd, _ = infoM["path"].(string)
		}
		var updated int64
		if tm, ok := infoM["time"].(map[string]any); ok {
			switch v := tm["updated"].(type) {
			case float64:
				updated = int64(v)
			}
		}
		refs = append(refs, port.SessionRef{
			Harness: port.HarnessOpenCode, SourceSessionID: id, SourcePath: path,
			Title: title, CWD: cwd, UpdatedAt: updated,
		})
		return nil
	})
	return refs, nil
}

func (OpenCodeImport) Parse(ref port.SessionRef) (port.PortableSession, error) {
	sess := port.NewPortableSession(port.HarnessOpenCode, ref.SourceSessionID, ref.SourcePath, ref.CWD, ref.Title)
	data, err := os.ReadFile(ref.SourcePath)
	if err != nil {
		return sess, err
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return sess, err
	}
	if info, ok := doc["info"].(map[string]any); ok {
		if t, ok := info["title"].(string); ok {
			sess.Title = t
		}
		if c, ok := info["directory"].(string); ok {
			sess.CWD = c
		}
		if tm, ok := info["time"].(map[string]any); ok {
			if v, ok := tm["created"].(float64); ok {
				sess.CreatedAt = int64(v)
			}
			if v, ok := tm["updated"].(float64); ok {
				sess.UpdatedAt = int64(v)
			}
		}
	}
	msgs, _ := doc["messages"].([]any)
	for _, m := range msgs {
		mm, ok := m.(map[string]any)
		if !ok {
			continue
		}
		info, _ := mm["info"].(map[string]any)
		role, _ := info["role"].(string)
		if role != "user" && role != "assistant" {
			continue
		}
		var text strings.Builder
		dropped := false
		parts, _ := mm["parts"].([]any)
		for _, p := range parts {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
			}
			typ, _ := pm["type"].(string)
			switch typ {
			case "text":
				if t, ok := pm["text"].(string); ok {
					text.WriteString(t)
				}
			case "reasoning", "tool", "step-start", "step-finish":
				dropped = true
			}
		}
		if dropped && text.Len() == 0 {
			sess.DroppedTurns++
			continue
		}
		if text.Len() == 0 {
			sess.DroppedTurns++
			continue
		}
		var ts int64
		if tm, ok := info["time"].(map[string]any); ok {
			if v, ok := tm["created"].(float64); ok {
				ts = int64(v)
			}
		}
		sess.Turns = append(sess.Turns, port.PortableTurn{Role: role, Text: text.String(), Timestamp: ts})
	}
	ensureMeta(&sess)
	return sess, nil
}

type OpenCodeExport struct{}

func (OpenCodeExport) Harness() port.HarnessID { return port.HarnessOpenCode }

func (OpenCodeExport) Detect() (bool, error) {
	dir := filepath.Join(home(), ".opencode", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, err
	}
	return true, nil
}

func (OpenCodeExport) Write(session port.PortableSession, opts port.WriteOptions) (port.ExportResult, error) {
	destID := opts.ExistingDestID
	if destID == "" || opts.Force {
		destID = fmt.Sprintf("ses_so_port_%d", time.Now().UnixNano())
	}
	cwd := session.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	now := time.Now().UnixMilli()
	msgs := []any{}
	for i, t := range session.Turns {
		id := fmt.Sprintf("msg_%d", i)
		ts := t.Timestamp
		if ts == 0 {
			ts = now + int64(i)
		}
		msgs = append(msgs, map[string]any{
			"info":  map[string]any{"id": id, "role": t.Role, "time": map[string]any{"created": ts}},
			"parts": []map[string]any{{"type": "text", "text": t.Text, "id": "prt_" + id, "messageID": id}},
		})
	}
	doc := map[string]any{
		"info": map[string]any{
			"id": destID, "title": session.Title, "directory": cwd, "path": cwd,
			"time": map[string]any{"created": session.CreatedAt, "updated": session.UpdatedAt},
			"soSource": map[string]any{
				"sourceHarness": session.SourceHarness, "sourceSessionId": session.SourceSessionID,
			},
		},
		"messages": msgs,
	}
	path := filepath.Join(home(), ".opencode", "sessions", destID+".json")
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return port.ExportResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return port.ExportResult{}, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return port.ExportResult{}, err
	}
	return port.ExportResult{DestSessionID: destID}, nil
}

// exportOpenCodeSQLiteJSON best-effort dumps sessions from opencode.db using sqlite3 CLI
// into ~/.opencode/sessions/*.json so Import/Discover have equal fidelity without a Go sqlite dep.
func exportOpenCodeSQLiteJSON() error {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		return err
	}
	db := os.Getenv("OPENCODE_DB")
	if db == "" {
		db = filepath.Join(opencodeDataDir(), "opencode.db")
	}
	if _, err := os.Stat(db); err != nil {
		return err
	}
	outDir := filepath.Join(home(), ".opencode", "sessions")
	_ = os.MkdirAll(outDir, 0o755)
	// Dump id+title; schema varies - tolerate failure.
	out, err := exec.Command("sqlite3", "-json", db, "SELECT id, title, directory FROM session LIMIT 500;").Output()
	if err != nil {
		out, err = exec.Command("sqlite3", "-json", db, "SELECT id, title, directory FROM sessions LIMIT 500;").Output()
		if err != nil {
			return err
		}
	}
	var rows []map[string]any
	if json.Unmarshal(out, &rows) != nil {
		return fmt.Errorf("parse sqlite json")
	}
	for _, row := range rows {
		id, _ := row["id"].(string)
		if id == "" {
			continue
		}
		dest := filepath.Join(outDir, id+".json")
		if _, err := os.Stat(dest); err == nil {
			continue
		}
		title, _ := row["title"].(string)
		if strings.TrimSpace(title) == "" {
			title = id
		}
		dir, _ := row["directory"].(string)
		doc := map[string]any{
			"info": map[string]any{
				"id": id, "title": title, "directory": dir, "path": dir,
				"time": map[string]any{"updated": time.Now().UnixMilli()},
			},
			"messages": []any{},
			"source":   "sqlite_export",
		}
		data, _ := json.MarshalIndent(doc, "", "  ")
		_ = os.WriteFile(dest, data, 0o644)
	}
	return nil
}
