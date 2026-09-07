package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPostEditStaysSilent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	root := t.TempDir()
	writeHookSession(t, root, "post-silent")
	src := filepath.Join(root, "pkg/handler.py")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("def handle():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	payload := claudeEditPayload(t, root, "post-silent", src)
	cases := []struct{ vendor, event string }{
		{"claude-code", "PostToolUse"},
		{"cursor", "postToolUse"},
		{"gemini", "afterTool"},
		{"codex", "PostToolUse"},
		{"opencode", "tool.execute.after"},
		{"pi", "tool_execution_end"},
	}
	for _, c := range cases {
		if text, _, ok := steerTextFor(c.vendor, c.event, payload); ok {
			t.Fatalf("%s %s must stay silent after edits, got %q", c.vendor, c.event, text)
		}
	}
}

func claudeEditPayload(t *testing.T, root, session, path string) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"session_id": session,
		"cwd":        root,
		"tool_name":  "Edit",
		"tool_input": map[string]string{"file_path": path},
	})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}
