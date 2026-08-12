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
	"github.com/ishanjainn/superopen/internal/userpaths"
)

type CodexImport struct{}

func (CodexImport) Harness() port.HarnessID { return port.HarnessCodex }

func codexRoot() string {
	if root, err := userpaths.CodexHome(); err == nil {
		return root
	}
	return filepath.Join(home(), ".codex")
}

func (CodexImport) Detect() (bool, error) {
	_, err := os.Stat(codexRoot())
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (CodexImport) Discover() ([]port.SessionRef, error) {
	root := filepath.Join(codexRoot(), "sessions")
	var refs []port.SessionRef
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		sid, title, cwd, updated := peekCodex(path)
		if sid == "" {
			sid = strings.TrimSuffix(info.Name(), ".jsonl")
		}
		refs = append(refs, port.SessionRef{
			Harness: port.HarnessCodex, SourceSessionID: sid, SourcePath: path,
			Title: title, CWD: cwd, UpdatedAt: updated,
		})
		return nil
	})
	return refs, nil
}

func peekCodex(path string) (sid, title, cwd string, updated int64) {
	f, err := os.Open(path)
	if err != nil {
		return "", filepath.Base(path), "", 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		var row map[string]any
		if json.Unmarshal(sc.Bytes(), &row) != nil {
			continue
		}
		if ts, ok := row["timestamp"].(string); ok {
			if ms := parseTimeMs(ts); ms > updated {
				updated = ms
			}
		}
		typ, _ := row["type"].(string)
		payload, _ := row["payload"].(map[string]any)
		if typ == "session_meta" && payload != nil {
			if id, ok := payload["session_id"].(string); ok {
				sid = id
			} else if id, ok := payload["id"].(string); ok {
				sid = id
			}
			if c, ok := payload["cwd"].(string); ok {
				cwd = c
			}
		}
		if typ == "response_item" && payload != nil && title == "" {
			if payload["type"] == "message" && payload["role"] == "user" {
				title = firstLine(codexMessageText(payload), 80)
			}
		}
		if typ == "event_msg" && payload != nil && title == "" {
			if payload["type"] == "user_message" {
				if m, ok := payload["message"].(string); ok {
					title = firstLine(m, 80)
				}
			}
		}
	}
	if title == "" {
		title = sid
	}
	return sid, title, cwd, updated
}

func codexMessageText(payload map[string]any) string {
	content, _ := payload["content"].([]any)
	var b strings.Builder
	for _, part := range content {
		m, ok := part.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := m["type"].(string)
		if typ == "input_text" || typ == "output_text" {
			if t, ok := m["text"].(string); ok {
				if b.Len() > 0 {
					b.WriteByte(' ')
				}
				b.WriteString(t)
			}
		}
	}
	return b.String()
}

// codexCallArgs decodes a function_call's arguments, which Codex stores as a
// JSON-encoded string rather than a nested object.
func codexCallArgs(payload map[string]any) map[string]any {
	if m, ok := payload["arguments"].(map[string]any); ok {
		return m
	}
	if m, ok := payload["input"].(map[string]any); ok {
		return m
	}
	raw := firstStringField(payload, "arguments", "input")
	if raw == "" {
		return nil
	}
	var out map[string]any
	if json.Unmarshal([]byte(raw), &out) != nil {
		return nil
	}
	return out
}

func (CodexImport) Parse(ref port.SessionRef) (port.PortableSession, error) {
	sess := port.NewPortableSession(port.HarnessCodex, ref.SourceSessionID, ref.SourcePath, ref.CWD, ref.Title)
	ws := newWSCollector(ref.CWD)
	// call_id → command index in wsCollector, so a later function_call_output
	// attaches its exit code to the matching command even when another shell
	// call was observed in between (parallel / out-of-order outputs).
	pendingCmdIdx := map[string]int{}
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
		ts := parseTimeMs(fmt.Sprint(row["timestamp"]))
		if s, ok := row["timestamp"].(string); ok {
			ts = parseTimeMs(s)
		}
		if ts > 0 {
			if sess.CreatedAt == 0 || ts < sess.CreatedAt {
				sess.CreatedAt = ts
			}
			if ts > sess.UpdatedAt {
				sess.UpdatedAt = ts
			}
		}
		typ, _ := row["type"].(string)
		payload, _ := row["payload"].(map[string]any)
		if payload == nil {
			continue
		}
		switch typ {
		case "session_meta":
			if c, ok := payload["cwd"].(string); ok {
				sess.CWD = c
				ws.cwd = c
			}
			if id, ok := payload["session_id"].(string); ok {
				sess.SourceSessionID = id
			}
		case "response_item":
			pt, _ := payload["type"].(string)
			switch pt {
			case "message":
				role, _ := payload["role"].(string)
				if role == "developer" {
					sess.DroppedTurns++
					continue
				}
				if role != "user" && role != "assistant" {
					sess.DroppedTurns++
					continue
				}
				text := codexMessageText(payload)
				if strings.TrimSpace(text) == "" {
					sess.DroppedTurns++
					continue
				}
				sess.Turns = append(sess.Turns, port.PortableTurn{Role: role, Text: text, Timestamp: ts})
				if sess.Title == "" && role == "user" {
					sess.Title = firstLine(text, 80)
				}
			case "function_call", "custom_tool_call":
				sess.DroppedTurns++
				name := firstStringField(payload, "name", "tool_name")
				args := codexCallArgs(payload)
				idx := ws.observe(name, args, nil)
				if id := firstStringField(payload, "call_id", "id"); id != "" && idx >= 0 {
					pendingCmdIdx[id] = idx
				}
			case "function_call_output", "custom_tool_call_output":
				sess.DroppedTurns++
				// Outputs carry the exit status; attach by call_id, not "last command".
				id := firstStringField(payload, "call_id", "id")
				if idx, ok := pendingCmdIdx[id]; ok {
					if exit, ok := extractExitCode(payload["output"]); ok {
						ws.attachExitAt(idx, exit)
					}
				}
				delete(pendingCmdIdx, id)
			case "reasoning":
				sess.DroppedTurns++
			}
		case "event_msg":
			et, _ := payload["type"].(string)
			msg, _ := payload["message"].(string)
			if strings.TrimSpace(msg) == "" {
				continue
			}
			switch et {
			case "user_message":
				sess.Turns = append(sess.Turns, port.PortableTurn{Role: "user", Text: msg, Timestamp: ts})
				if sess.Title == "" {
					sess.Title = firstLine(msg, 80)
				}
			case "agent_message", "assistant_message":
				sess.Turns = append(sess.Turns, port.PortableTurn{Role: "assistant", Text: msg, Timestamp: ts})
			default:
				sess.DroppedTurns++
			}
		}
	}
	ensureMeta(&sess)
	sess.WorkingState = ws.result()
	return sess, nil
}

type CodexExport struct{}

func (CodexExport) Harness() port.HarnessID { return port.HarnessCodex }

func (CodexExport) Detect() (bool, error) {
	if err := os.MkdirAll(filepath.Join(codexRoot(), "sessions"), 0o755); err != nil {
		return false, err
	}
	return true, nil
}

func (CodexExport) Write(session port.PortableSession, opts port.WriteOptions) (port.ExportResult, error) {
	destID := opts.ExistingDestID
	if destID == "" || opts.Force {
		destID = fmt.Sprintf("so-port-%d", time.Now().UnixNano())
	}
	cwd := session.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	dir := filepath.Join(codexRoot(), "sessions", "so-port")
	path := filepath.Join(dir, destID+".jsonl")
	now := time.Now().UTC()
	rows := []any{
		map[string]any{
			"timestamp": now.Format(time.RFC3339Nano),
			"type":      "session_meta",
			"payload": map[string]any{
				"session_id": destID, "id": destID, "cwd": cwd,
				"originator": "superopen-port", "source": "cli",
				"soSource": map[string]any{
					"sourceHarness": session.SourceHarness, "sourceSessionId": session.SourceSessionID,
				},
			},
		},
	}
	for i, t := range session.Turns {
		ts := now.Add(time.Duration(i+1) * time.Second).Format(time.RFC3339Nano)
		if t.Timestamp > 0 {
			ts = time.UnixMilli(t.Timestamp).UTC().Format(time.RFC3339Nano)
		}
		partType := "input_text"
		if t.Role == "assistant" {
			partType = "output_text"
		}
		rows = append(rows, map[string]any{
			"timestamp": ts,
			"type":      "response_item",
			"payload": map[string]any{
				"type": "message", "role": t.Role,
				"content": []map[string]any{{"type": partType, "text": t.Text}},
			},
		})
	}
	if err := writeJSONL(path, rows); err != nil {
		return port.ExportResult{}, err
	}
	return port.ExportResult{DestSessionID: destID}, nil
}
