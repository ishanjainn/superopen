package checkpoint_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/superopen/so/internal/checkpoint"
	"github.com/superopen/so/internal/harness"
)

func TestCreateRestore(t *testing.T) {
	repo := t.TempDir()
	soRoot := filepath.Join(repo, ".so")
	_ = os.MkdirAll(soRoot, 0o755)
	paths := harness.Resolve(repo)
	_ = os.MkdirAll(paths.SessionDir("s1"), 0o755)

	src := filepath.Join(repo, "a.txt")
	if err := os.WriteFile(src, []byte("secret sk-abcdefghijklmnopqrstuvwxyz123456"), 0o644); err != nil {
		t.Fatal(err)
	}
	cs := checkpoint.NewStore(paths)
	m, err := cs.Create("s1", repo, "test", []string{"a.txt"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != 1 || len(m.Files) != 1 {
		t.Fatalf("meta=%+v", m)
	}
	snap := filepath.Join(paths.SessionDir("s1"), "checkpoints", "1", "files", "a.txt")
	data, err := os.ReadFile(snap)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "secret sk-abcdefghijklmnopqrstuvwxyz123456" {
		t.Fatal("expected redaction in checkpoint snapshot")
	}
	_ = os.WriteFile(src, []byte("changed"), 0o644)
	if err := cs.Restore("s1", "1", repo); err != nil {
		t.Fatal(err)
	}
	restored, _ := os.ReadFile(src)
	if string(restored) != string(data) {
		t.Fatalf("restore mismatch: %q vs %q", restored, data)
	}
}
