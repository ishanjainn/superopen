package adapters

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/superopen/so/internal/port"
)

// ClaudeImport reads ~/.claude/projects/<encoded-cwd>/*.jsonl
type ClaudeImport struct{}

func (ClaudeImport) Harness() port.HarnessID { return port.HarnessClaude }

func (ClaudeImport) Detect() (bool, error) {
	_, err := os.Stat(filepath.Join(home(), ".claude", "projects"))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (c ClaudeImport) Discover() ([]port.SessionRef, error) {
	root := filepath.Join(home(), ".claude", "projects")
	var refs []port.SessionRef
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() && info.Name() == "subagents" {
			return filepath.SkipDir
		}
		if info.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		// Skip nested agent rollups under session folders (…/<uuid>/….jsonl depth oddities).
		rel, _ := filepath.Rel(root, path)
		parts := strings.Split(rel, string(os.PathSeparator))
		if len(parts) != 2 {
			return nil
		}
		sid := strings.TrimSuffix(info.Name(), ".jsonl")
		title, cwd, updated := peekClaude(path)
		refs = append(refs, port.SessionRef{
			Harness:         port.HarnessClaude,
			SourceSessionID: sid,
			SourcePath:      path,
			Title:           title,
			CWD:             cwd,
			UpdatedAt:       updated,
		})
		return nil
	})
	sort.Slice(refs, func(i, j int) bool { return refs[i].UpdatedAt > refs[j].UpdatedAt })
	return refs, nil
}

func peekClaude(path string) (title, cwd string, updated int64) {
	f, err := os.Open(path)
	if err != nil {
		return filepath.Base(path), "", 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		var row map[string]any
		if json.Unmarshal(sc.Bytes(), &row) != nil {
			continue
		}
		if c, ok := row["cwd"].(string); ok && cwd == "" {
			cwd = c
		}
		if ts, ok := row["timestamp"].(string); ok {
			if ms := parseTimeMs(ts); ms > updated {
				updated = ms
			}
		}
		if typ, _ := row["type"].(string); typ == "user" && title == "" {
			if msg, ok := row["message"].(map[string]any); ok {
				t, _ := textFromContent(msg["content"])
				title = firstLine(t, 80)
			}
		}
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}
	return title, cwd, updated
}

func (ClaudeImport) Parse(ref port.SessionRef) (port.PortableSession, error) {
	sess := port.NewPortableSession(port.HarnessClaude, ref.SourceSessionID, ref.SourcePath, ref.CWD, ref.Title)
	f, err := os.Open(ref.SourcePath)
	if err != nil {
		return sess, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for sc.Scan() {
		var row map[string]any
		if json.Unmarshal(sc.Bytes(), &row) != nil {
			continue
		}
		if c, ok := row["cwd"].(string); ok && sess.CWD == "" {
			sess.CWD = c
		}
		if ts, ok := row["timestamp"].(string); ok {
			if ms := parseTimeMs(ts); ms > 0 {
				if sess.CreatedAt == 0 || ms < sess.CreatedAt {
					sess.CreatedAt = ms
				}
				if ms > sess.UpdatedAt {
					sess.UpdatedAt = ms
				}
			}
		}
		typ, _ := row["type"].(string)
		msg, _ := row["message"].(map[string]any)
		if msg == nil {
			continue
		}
		role, _ := msg["role"].(string)
		if role == "" {
			role = typ
		}
		if role != "user" && role != "assistant" {
			continue
		}
		text, dropped := textFromContent(msg["content"])
		if dropped && strings.TrimSpace(text) == "" {
			sess.DroppedTurns++
			continue
		}
		if strings.TrimSpace(text) == "" {
			sess.DroppedTurns++
			continue
		}
		model, _ := msg["model"].(string)
		turn := port.PortableTurn{Role: role, Text: text, Model: model}
		if ts, ok := row["timestamp"].(string); ok {
			turn.Timestamp = parseTimeMs(ts)
		}
		sess.Turns = append(sess.Turns, turn)
		if sess.Title == "" && role == "user" {
			sess.Title = firstLine(text, 80)
		}
	}
	ensureMeta(&sess)
	sess.SourceMetadata["soSource"] = map[string]any{
		"harness": "claude", "sessionId": ref.SourceSessionID,
	}
	return sess, nil
}

// ClaudeExport writes a new JSONL under ~/.claude/projects/<encoded-cwd>/
type ClaudeExport struct{}

func (ClaudeExport) Harness() port.HarnessID { return port.HarnessClaude }

func (ClaudeExport) Detect() (bool, error) {
	p := filepath.Join(home(), ".claude")
	if err := os.MkdirAll(filepath.Join(p, "projects"), 0o755); err != nil {
		return false, err
	}
	return true, nil
}

func encodeClaudeProjectDir(cwd string) string {
	// Claude encodes absolute paths by replacing / with -
	cwd = filepath.Clean(cwd)
	if cwd == "" {
		cwd = home()
	}
	return strings.ReplaceAll(cwd, string(os.PathSeparator), "-")
}

func (ClaudeExport) Write(session port.PortableSession, opts port.WriteOptions) (port.ExportResult, error) {
	cwd := session.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	dir := filepath.Join(home(), ".claude", "projects", encodeClaudeProjectDir(cwd))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return port.ExportResult{}, err
	}
	destID := opts.ExistingDestID
	if destID == "" || opts.Force {
		destID = fmt.Sprintf("so-port-%d", time.Now().UnixNano())
	}
	path := filepath.Join(dir, destID+".jsonl")
	var rows []any
	var parent string
	now := time.Now().UTC()
	for i, t := range session.Turns {
		uuid := fmt.Sprintf("%s-%d", destID, i)
		ts := now.Add(time.Duration(i) * time.Second).Format(time.RFC3339Nano)
		if t.Timestamp > 0 {
			ts = time.UnixMilli(t.Timestamp).UTC().Format(time.RFC3339Nano)
		}
		var content any
		if t.Role == "assistant" {
			content = []map[string]any{{"type": "text", "text": t.Text}}
		} else {
			content = t.Text
		}
		row := map[string]any{
			"parentUuid": nil,
			"type":       t.Role,
			"message":    map[string]any{"role": t.Role, "content": content},
			"uuid":       uuid,
			"timestamp":  ts,
			"cwd":        cwd,
			"sessionId":  destID,
			"gitBranch":  "",
		}
		if parent != "" {
			row["parentUuid"] = parent
		}
		if t.Model != "" && t.Role == "assistant" {
			row["message"].(map[string]any)["model"] = t.Model
		}
		// provenance
		row["soSource"] = map[string]any{
			"sourceHarness":   session.SourceHarness,
			"sourceSessionId": session.SourceSessionID,
		}
		rows = append(rows, row)
		parent = uuid
	}
	if err := writeJSONL(path, rows); err != nil {
		return port.ExportResult{}, err
	}
	return port.ExportResult{DestSessionID: destID}, nil
}
