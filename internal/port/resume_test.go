package port

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestArmResumeAlsoSetsCursorLegacyPending(t *testing.T) {
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
		t.Fatalf("legacy PENDING should be cleared: %v", err)
	}
}
