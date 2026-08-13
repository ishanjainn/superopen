package port

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/userpaths"
)

func TestArmAndConsumePendingResume(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".so"), 0o755)
	sess := NewPortableSession(HarnessClaude, "src-1", "/x", root, "hello")
	sess.Turns = []PortableTurn{
		{Role: "user", Text: "where are sessions?"},
		{Role: "assistant", Text: "under .so/sessions"},
	}
	if err := ArmResume(root, HarnessCodex, "so-port-99", sess); err != nil {
		t.Fatal(err)
	}
	body := ConsumePendingResume(root)
	if !strings.Contains(body, "where are sessions?") {
		t.Fatalf("missing user turn: %q", body)
	}
	if !strings.Contains(body, "under .so/sessions") {
		t.Fatalf("missing assistant turn: %q", body)
	}
	if second := ConsumePendingResume(root); second != "" {
		t.Fatalf("expected one-shot consume, got %q", second)
	}
}

func TestArmResumeIncludesWorkingState(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".so"), 0o755)
	sess := NewPortableSession(HarnessCodex, "src-3", "/x", root, "hello")
	sess.DroppedTurns = 4
	sess.WorkingState = WorkingState{
		FilesEdited: []string{"internal/port/ir.go"},
		FilesRead:   []string{"README.md"},
		Commands:    []RanCommand{{Cmd: "go test ./...", ExitCode: intPtr(0)}},
		GitBranch:   "main",
	}
	sess.Turns = []PortableTurn{{Role: "user", Text: "fix the bug"}}
	if err := ArmResume(root, HarnessClaude, "so-port-1", sess); err != nil {
		t.Fatal(err)
	}
	body := ConsumePendingResume(root)
	for _, want := range []string{
		"Working state carried from codex",
		"internal/port/ir.go",
		"README.md",
		"go test ./...",
		"exit 0",
		"main",
		"4 non-text turns omitted",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in body: %q", want, body)
		}
	}
}

func TestArmResumeOmitsWorkingStateWhenClean(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".so"), 0o755)
	sess := NewPortableSession(HarnessClaude, "src-4", "/x", root, "hello")
	sess.Turns = []PortableTurn{{Role: "user", Text: "hi"}}
	if err := ArmResume(root, HarnessCodex, "so-port-2", sess); err != nil {
		t.Fatal(err)
	}
	body := ConsumePendingResume(root)
	if strings.Contains(body, "Working state carried from") {
		t.Fatalf("expected no working-state block for a clean port: %q", body)
	}
}

func TestTrimResumeBodyKeepsRecentTurns(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 500; i++ {
		b.WriteString("## user\n\nfiller turn number filler filler filler\n\n")
	}
	b.WriteString("## user\n\nTHE MOST RECENT TURN\n\n")
	body := trimResumeBody([]byte(b.String()), "/tmp/full.md")
	if len(body) > maxResumeInject+500 {
		t.Fatalf("trimmed body too large: %d bytes", len(body))
	}
	if !strings.Contains(body, "THE MOST RECENT TURN") {
		t.Fatalf("most recent turn was dropped: tail=%q", body[max(0, len(body)-200):])
	}
	if !strings.Contains(body, "Earlier turns omitted") {
		t.Fatalf("missing truncation note: %q", body[:200])
	}
	if !strings.Contains(body, "/tmp/full.md") {
		t.Fatalf("missing pointer to full transcript: %q", body[:200])
	}
}

func TestConsumePendingResumeArchivesFullTranscriptWhenTrimmed(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".so"), 0o755)
	sess := NewPortableSession(HarnessCodex, "src-5", "/x", root, "long one")
	for i := 0; i < 500; i++ {
		sess.Turns = append(sess.Turns, PortableTurn{Role: "user", Text: "filler turn filler filler filler filler"})
	}
	sess.Turns = append(sess.Turns, PortableTurn{Role: "assistant", Text: "THE FINAL WORD"})
	if err := ArmResume(root, HarnessClaude, "so-port-3", sess); err != nil {
		t.Fatal(err)
	}
	body := ConsumePendingResume(root)
	if !strings.Contains(body, "THE FINAL WORD") {
		t.Fatalf("expected most recent turn to survive trimming")
	}
	runtimeDir, err := userpaths.RuntimeDir(root)
	if err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(runtimeDir, "port", "last-conversation.md")
	raw, err := os.ReadFile(archive)
	if err != nil {
		t.Fatalf("expected archived full transcript at %s: %v", archive, err)
	}
	if !strings.Contains(string(raw), "filler turn") || !strings.Contains(string(raw), "THE FINAL WORD") {
		t.Fatalf("archive missing content")
	}
	if !strings.Contains(body, archive) {
		t.Fatalf("resume body should point at archive path: %q", body[:200])
	}
}

func intPtr(i int) *int { return &i }

func TestArmResumeAlsoSetsCursorPending(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".so"), 0o755)
	sess := NewPortableSession(HarnessClaude, "src-2", "/x", root, "t")
	sess.Turns = []PortableTurn{{Role: "user", Text: "hi"}}
	if err := ArmResume(root, HarnessCursor, "cursor-port-1", sess); err != nil {
		t.Fatal(err)
	}
	pending := filepath.Join(root, ".cursor", "so-port", "PENDING")
	raw, err := os.ReadFile(pending)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(raw)) != "cursor-port-1" {
		t.Fatalf("PENDING=%q", raw)
	}
	_ = ConsumePendingResume(root)
	if _, err := os.Stat(pending); !os.IsNotExist(err) {
		t.Fatalf("Cursor PENDING should be cleared: %v", err)
	}
}
