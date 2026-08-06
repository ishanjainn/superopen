package adapters

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/superopen/so/internal/port"
)

func home() string {
	h, _ := os.UserHomeDir()
	return h
}

func firstLine(s string, n int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if n > 0 && len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func parseTimeMs(s string) int64 {
	if s == "" {
		return 0
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UnixMilli()
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UnixMilli()
	}
	return 0
}

func writeJSONL(path string, rows []any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, r := range rows {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	return nil
}

func textFromContent(v any) (text string, dropped bool) {
	switch c := v.(type) {
	case string:
		return c, false
	case []any:
		var b strings.Builder
		for _, part := range c {
			m, ok := part.(map[string]any)
			if !ok {
				continue
			}
			typ, _ := m["type"].(string)
			switch typ {
			case "text":
				if t, ok := m["text"].(string); ok {
					b.WriteString(t)
				}
			case "tool_use", "tool_result", "thinking":
				dropped = true
			}
		}
		return b.String(), dropped
	default:
		return "", false
	}
}

func ensureMeta(s *port.PortableSession) {
	if s.SourceMetadata == nil {
		s.SourceMetadata = map[string]any{}
	}
}
