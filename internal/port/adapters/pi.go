package adapters

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/port"
)

// PiImport reads ~/.pi/agent/sessions/<encoded-cwd>/*.jsonl.
type PiImport struct{}

func (PiImport) Harness() port.HarnessID { return port.HarnessPi }

func piHome() string {
	if v := os.Getenv("PI_CODING_AGENT_DIR"); v != "" {
		return v
	}
	return filepath.Join(home(), ".pi", "agent")
}

func (PiImport) Detect() (bool, error) {
	_, err := os.Stat(filepath.Join(piHome(), "sessions"))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (PiImport) Discover() ([]port.SessionRef, error) {
	root := filepath.Join(piHome(), "sessions")
	var refs []port.SessionRef
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		title, cwd, updated := peekPi(path)
		sid := strings.TrimSuffix(info.Name(), ".jsonl")
		refs = append(refs, port.SessionRef{
			Harness: port.HarnessPi, SourceSessionID: sid, SourcePath: path,
			Title: title, CWD: cwd, UpdatedAt: updated,
		})
		return nil
	})
	return refs, nil
}

func peekPi(path string) (title, cwd string, updated int64) {
	f, err := os.Open(path)
	if err != nil {
		return filepath.Base(path), "", 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	var (
		infoTitle string
		sawInfo   bool
		firstUser string
	)
	for sc.Scan() {
		var row map[string]any
		if json.Unmarshal(sc.Bytes(), &row) != nil {
			continue
		}
		if ts, ok := row["timestamp"].(string); ok {
			if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
				updated = t.UnixMilli()
			}
		}
		if typ, _ := row["type"].(string); typ == "session" {
			if c, _ := row["cwd"].(string); c != "" && cwd == "" {
				cwd = c
			}
		}
		// Latest session_info.name wins; empty clears.
		if typ, _ := row["type"].(string); typ == "session_info" {
			sawInfo = true
			infoTitle, _ = row["name"].(string)
			infoTitle = strings.TrimSpace(infoTitle)
			continue
		}
		msg, _ := row["message"].(map[string]any)
		if msg == nil {
			continue
		}
		role, _ := msg["role"].(string)
		if role == "user" && firstUser == "" {
			firstUser = firstLine(piText(msg["content"]), 80)
		}
	}
	switch {
	case sawInfo && infoTitle != "":
		title = infoTitle
	case firstUser != "":
		title = firstUser
	default:
		title = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}
	return title, cwd, updated
}

func piText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var b strings.Builder
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if t, _ := m["type"].(string); t == "text" {
				if s, ok := m["text"].(string); ok {
					b.WriteString(s)
				}
			}
		}
		return b.String()
	default:
		return ""
	}
}

func (PiImport) Parse(ref port.SessionRef) (port.PortableSession, error) {
	sess := port.NewPortableSession(port.HarnessPi, ref.SourceSessionID, ref.SourcePath, ref.CWD, ref.Title)
	f, err := os.Open(ref.SourcePath)
	if err != nil {
		return sess, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		var row map[string]any
		if json.Unmarshal(sc.Bytes(), &row) != nil {
			continue
		}
		typ, _ := row["type"].(string)
		if typ != "" && typ != "message" {
			sess.DroppedTurns++
			continue
		}
		msg, _ := row["message"].(map[string]any)
		if msg == nil {
			sess.DroppedTurns++
			continue
		}
		role, _ := msg["role"].(string)
		text := piText(msg["content"])
		switch role {
		case "user":
			role = "user"
		case "assistant":
			role = "assistant"
		default:
			sess.DroppedTurns++
			continue
		}
		if strings.TrimSpace(text) == "" {
			sess.DroppedTurns++
			continue
		}
		var ts int64
		if t, ok := row["timestamp"].(string); ok {
			if parsed, err := time.Parse(time.RFC3339Nano, t); err == nil {
				ts = parsed.UnixMilli()
			}
		}
		sess.Turns = append(sess.Turns, port.PortableTurn{Role: role, Text: text, Timestamp: ts})
	}
	ensureMeta(&sess)
	return sess, nil
}

// PiExport writes a resumable JSONL session under ~/.pi/agent/sessions/so-port/.
type PiExport struct{}

func (PiExport) Harness() port.HarnessID { return port.HarnessPi }

func (PiExport) Detect() (bool, error) {
	if err := os.MkdirAll(filepath.Join(piHome(), "sessions", "so-port"), 0o755); err != nil {
		return false, err
	}
	return true, nil
}

func (PiExport) Write(ps port.PortableSession, opts port.WriteOptions) (port.ExportResult, error) {
	destID := opts.ExistingDestID
	if destID == "" || opts.Force {
		destID = fmt.Sprintf("ses_so_port_%d", time.Now().UnixNano())
	}
	dir := filepath.Join(piHome(), "sessions", "so-port")
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, destID+".jsonl")
	f, err := os.Create(path)
	if err != nil {
		return port.ExportResult{}, err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i, t := range ps.Turns {
		ts := now
		if t.Timestamp > 0 {
			ts = time.UnixMilli(t.Timestamp).UTC().Format(time.RFC3339Nano)
		}
		_ = enc.Encode(map[string]any{
			"type": "message", "id": fmt.Sprintf("msg_%d", i), "timestamp": ts,
			"message": map[string]any{
				"role":    t.Role,
				"content": []map[string]any{{"type": "text", "text": t.Text}},
			},
		})
	}
	_ = os.WriteFile(filepath.Join(dir, destID+".RESUME.md"), []byte(fmt.Sprintf(
		"# Ported Pi session\n\nResume pack: %s\nSource: %s / %s\n\nOpen Pi in the repo and continue from this transcript, or:\nso sessions resume --vendor=pi --id=%s\n",
		path, ps.SourceHarness, ps.SourceSessionID, destID,
	)), 0o644)
	return port.ExportResult{DestSessionID: destID}, nil
}
