package session

import (
	"os"
	"strings"

	"github.com/ishanjainn/superopen/internal/session/trace"
)

// SpansHaveActivity reports whether spans include a real user turn or tool work.
// Identity-only telemetry (vendor/model/user/branch) does not count - those create
// empty "opened chat then closed" sessions.
func SpansHaveActivity(spans []trace.Span) bool {
	for _, sp := range spans {
		attrs := sp.Attributes
		if attrs == nil {
			continue
		}
		if attrs["gen_ai.prompt"] != "" || attrs["gen_ai.content.prompt"] != "" {
			return true
		}
		if raw := attrs["gen_ai.input.messages"]; raw != "" {
			low := strings.ToLower(raw)
			if strings.Contains(low, `"role":"user"`) ||
				strings.Contains(low, `"role": "user"`) ||
				strings.Contains(low, `"role":"user_prompt"`) {
				return true
			}
		}
		if attrs["coding_agent.file_path"] != "" || attrs["coding_agent.command"] != "" {
			return true
		}
		name := strings.ToLower(sp.Name)
		switch {
		case strings.Contains(name, "prompt"),
			strings.Contains(name, "completion"),
			strings.Contains(name, "tool"),
			strings.Contains(name, "edit"),
			strings.Contains(name, "write"),
			strings.Contains(name, "read"),
			strings.Contains(name, "search"),
			strings.Contains(name, "grep"),
			strings.Contains(name, "glob"),
			strings.Contains(name, "exec"):
			return true
		}
	}
	return false
}

// IsEmptyListItem is true when a session never got real turns/work.
func IsEmptyListItem(item ListItem) bool {
	if item.hasActivity {
		return false
	}
	if item.Turns > 0 || item.Tokens > 0 {
		return false
	}
	if len(item.Files) > 0 {
		return false
	}
	if strings.TrimSpace(item.PromptPreview) != "" {
		return false
	}
	return true
}

// IsEmpty reports a session with no real turns/work on disk.
func (s *Store) IsEmpty(id string) bool {
	meta, err := s.Get(id)
	if err != nil {
		return true
	}
	return IsEmptyListItem(s.enrich(meta))
}

// Delete removes a session directory, index row, and live state file.
func (s *Store) Delete(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	dir := s.Paths.SessionDir(id)
	_ = os.RemoveAll(dir)
	_ = NewStateStore(s.Paths).Delete(id)

	entries, err := s.List()
	if err != nil {
		return err
	}
	kept := make([]IndexEntry, 0, len(entries))
	for _, e := range entries {
		if e.ID == id {
			continue
		}
		kept = append(kept, e)
	}
	return writeJSON(s.Paths.SessionsIndex, indexFile{About: indexAbout, Sessions: kept})
}
