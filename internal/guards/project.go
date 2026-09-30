package guards

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/session/trace"
)

// EventsFromSpans projects recorded tool-call spans into events the rules can match.
// It reads spans only. It does not write session files.
func EventsFromSpans(spans []trace.Span) []Event {
	out := make([]Event, 0, len(spans))
	for _, sp := range spans {
		if sp.Name != "coding_agent.tool.call" {
			continue
		}
		ev, ok := eventFromSpan(sp)
		if ok {
			out = append(out, ev)
		}
	}
	return out
}

// ReadSpans reads a JSONL span file. A missing file is an empty slice.
func ReadSpans(path string) ([]trace.Span, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var spans []trace.Span
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var sp trace.Span
		if err := json.Unmarshal([]byte(line), &sp); err != nil {
			continue
		}
		spans = append(spans, sp)
	}
	return spans, sc.Err()
}

func eventFromSpan(sp trace.Span) (Event, bool) {
	tool := strings.TrimSpace(sp.Attributes["gen_ai.tool.name"])
	rawArgs := sp.Attributes["gen_ai.tool.call.arguments"]
	action, ok := actionForTool(tool)
	if !ok {
		return Event{}, false
	}
	ts := time.Now().UTC()
	if sp.StartTimeUnixN > 0 {
		ts = time.Unix(0, sp.StartTimeUnixN).UTC()
	}
	ev := Event{
		Timestamp:     FormatTimestamp(ts),
		Vendor:        Vendor,
		Product:       Product,
		SchemaVersion: SchemaVersion,
		Severity:      SeverityInfo,
		Event:         EventInfo{Kind: "tool", Action: action, Fidelity: FidelityObserved},
		Harness:       HarnessInfo{Name: "superopen", CollectionMethod: CollectionMethodHook},
	}
	if sp.SessionID != "" {
		ev.Session = &SessionInfo{ID: sp.SessionID}
	}
	switch action {
	case "command.executed":
		cmd := argumentField(rawArgs, "command")
		if cmd == "" && rawArgs != "" && !strings.HasPrefix(strings.TrimSpace(rawArgs), "{") {
			cmd = strings.TrimSpace(rawArgs)
		}
		ev.Command = &CommandInfo{Command: cmd}
	case "file.read", "file.edited":
		ev.File = &FileInfo{Path: argumentField(rawArgs, "file_path")}
	}
	return ev, true
}

func actionForTool(name string) (string, bool) {
	switch name {
	case "Bash", "Shell":
		return "command.executed", true
	case "Read":
		return "file.read", true
	case "Edit", "Write", "MultiEdit":
		return "file.edited", true
	default:
		return "", false
	}
}

func argumentField(raw, field string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return ""
	}
	s, _ := m[field].(string)
	return strings.TrimSpace(s)
}
