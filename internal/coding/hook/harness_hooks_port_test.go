package hook

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/port"
)

// Port resume must inject even when memory.enabled is false - otherwise
// any→any handoff silently drops after Port.
func TestMaybeInjectMemory_PortResumeWithoutMemory(t *testing.T) {
	root := t.TempDir()
	soDir := filepath.Join(root, ".so")
	if err := os.MkdirAll(filepath.Join(soDir, "memory"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "memory:\n  enabled: false\n"
	if err := os.WriteFile(filepath.Join(soDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	sess := port.NewPortableSession(port.HarnessClaude, "src-1", "/x", root, "handoff")
	sess.Turns = []port.PortableTurn{
		{Role: "user", Text: "continue the ported work"},
		{Role: "assistant", Text: "armed for next agent"},
	}
	if err := port.ArmResume(root, port.HarnessCodex, "dest-99", sess); err != nil {
		t.Fatal(err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	maybeInjectMemory("codex", "SessionStart", "sess-new", root)
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()
	out := buf.String()
	if !strings.Contains(out, "continue the ported work") {
		t.Fatalf("expected ported user turn in hook JSON, got: %s", out)
	}
	if !strings.Contains(out, "Ported conversation resume") {
		t.Fatalf("expected resume section, got: %s", out)
	}
	// One-shot: second SessionStart should not re-inject.
	r2, w2, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w2
	maybeInjectMemory("codex", "SessionStart", "sess-2", root)
	_ = w2.Close()
	os.Stdout = old
	var buf2 bytes.Buffer
	_, _ = io.Copy(&buf2, r2)
	_ = r2.Close()
	if strings.Contains(buf2.String(), "continue the ported work") {
		t.Fatalf("expected one-shot consume, second inject still had resume: %s", buf2.String())
	}
}
