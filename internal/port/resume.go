package port

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const pendingFileName = "PENDING.json"
const pendingConvName = "pending-conversation.md"

// PendingResume is the one-shot SessionStart inject armed after Port.
type PendingResume struct {
	To              HarnessID `json:"to"`
	DestSessionID   string    `json:"dest_session_id"`
	SourceHarness   HarnessID `json:"source_harness,omitempty"`
	SourceSessionID string    `json:"source_session_id,omitempty"`
	Title           string    `json:"title,omitempty"`
	ConversationPath string   `json:"conversation_path,omitempty"`
	ArmedAt         time.Time `json:"armed_at"`
}

func portRunDir(repoRoot string) string {
	return filepath.Join(repoRoot, ".so", "port")
}

// ArmResume writes a one-shot pending conversation for the next coding-agent
// SessionStart (any vendor). Call after a successful Port export.
func ArmResume(repoRoot string, to HarnessID, destID string, sess PortableSession) error {
	if repoRoot == "" || destID == "" {
		return nil
	}
	dir := portRunDir(repoRoot)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	convPath := filepath.Join(dir, pendingConvName)
	var b strings.Builder
	b.WriteString("# Ported conversation resume\n\n")
	b.WriteString(fmt.Sprintf("Source: %s / %s\n", sess.SourceHarness, sess.SourceSessionID))
	b.WriteString(fmt.Sprintf("Destination: %s / %s\n\n", to, destID))
	if sess.Title != "" {
		b.WriteString(fmt.Sprintf("Title: %s\n\n", sess.Title))
	}
	for _, t := range sess.Turns {
		role := strings.TrimSpace(t.Role)
		if role == "" {
			role = "user"
		}
		b.WriteString("## " + role + "\n\n")
		b.WriteString(strings.TrimSpace(t.Text))
		b.WriteString("\n\n")
	}
	if err := os.WriteFile(convPath, []byte(b.String()), 0o644); err != nil {
		return err
	}
	meta := PendingResume{
		To:               to,
		DestSessionID:    destID,
		SourceHarness:    sess.SourceHarness,
		SourceSessionID:  sess.SourceSessionID,
		Title:            sess.Title,
		ConversationPath: convPath,
		ArmedAt:          time.Now().UTC(),
	}
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, pendingFileName), raw, 0o644); err != nil {
		return err
	}
	// Keep Cursor's legacy PENDING marker in sync when destination is Cursor
	// (older hooks / RESUME.md still reference .cursor/so-port/PENDING).
	if to == HarnessCursor {
		cursorDir := filepath.Join(repoRoot, ".cursor", "so-port")
		_ = os.MkdirAll(cursorDir, 0o755)
		_ = os.WriteFile(filepath.Join(cursorDir, "PENDING"), []byte(destID+"\n"), 0o644)
		// Mirror conversation for Cursor pack consumers.
		packDir := filepath.Join(cursorDir, destID)
		_ = os.MkdirAll(packDir, 0o755)
		_ = os.WriteFile(filepath.Join(packDir, "conversation.md"), []byte(b.String()), 0o644)
	}
	return nil
}

// ConsumePendingResume returns the conversation markdown for SessionStart
// inject and clears the pending marker (one-shot). Empty string if none.
func ConsumePendingResume(repoRoot string) string {
	if repoRoot == "" {
		return ""
	}
	// Prefer harness-agnostic .so/port pending.
	if body := consumeSOPortPending(repoRoot); body != "" {
		return body
	}
	// Legacy Cursor-only PENDING.
	return consumeLegacyCursorPending(repoRoot)
}

func consumeSOPortPending(repoRoot string) string {
	metaPath := filepath.Join(portRunDir(repoRoot), pendingFileName)
	raw, err := os.ReadFile(metaPath)
	if err != nil {
		return ""
	}
	var meta PendingResume
	_ = json.Unmarshal(raw, &meta)
	convPath := meta.ConversationPath
	if convPath == "" {
		convPath = filepath.Join(portRunDir(repoRoot), pendingConvName)
	}
	body, err := os.ReadFile(convPath)
	if err != nil {
		return ""
	}
	_ = os.Remove(metaPath)
	_ = os.Remove(convPath)
	// Clear legacy Cursor marker if present.
	_ = os.Remove(filepath.Join(repoRoot, ".cursor", "so-port", "PENDING"))
	if len(body) > 8000 {
		body = body[:8000]
	}
	return string(body)
}

func consumeLegacyCursorPending(repoRoot string) string {
	pending := filepath.Join(repoRoot, ".cursor", "so-port", "PENDING")
	data, err := os.ReadFile(pending)
	if err != nil {
		return ""
	}
	id := strings.TrimSpace(string(data))
	if id == "" {
		return ""
	}
	conv := filepath.Join(repoRoot, ".cursor", "so-port", id, "conversation.md")
	body, err := os.ReadFile(conv)
	if err != nil {
		return ""
	}
	_ = os.Remove(pending)
	if len(body) > 8000 {
		body = body[:8000]
	}
	return string(body)
}
