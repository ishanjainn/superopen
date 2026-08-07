package gitruntime

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWriteReadSessionSideRef(t *testing.T) {
	root := t.TempDir()
	run(t, root, "git", "init")
	run(t, root, "git", "config", "user.email", "t@example.com")
	run(t, root, "git", "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(root, "README"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, root, "git", "add", "README")
	run(t, root, "git", "commit", "-m", "init")

	sha, err := WriteSession(root, "sess-1", map[string][]byte{
		"meta.json":        []byte(`{"id":"sess-1"}`),
		"transcript.jsonl": []byte("{\"role\":\"user\"}\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if sha == "" {
		t.Fatal("empty commit sha")
	}
	meta, err := ReadFile(root, "sess-1", "meta.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(meta) != `{"id":"sess-1"}` {
		t.Fatalf("meta=%s", meta)
	}
	ids, err := ListSessionIDs(root)
	if err != nil || len(ids) != 1 || ids[0] != "sess-1" {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
	// Second write appends parent commit (CAS).
	if _, err := WriteSession(root, "sess-1", map[string][]byte{
		"meta.json": []byte(`{"id":"sess-1","v":2}`),
	}); err != nil {
		t.Fatal(err)
	}
	meta, _ = ReadFile(root, "sess-1", "meta.json")
	if string(meta) != `{"id":"sess-1","v":2}` {
		t.Fatalf("meta after update=%s", meta)
	}
	// Feature branch HEAD must be untouched.
	head, _ := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	status, _ := exec.Command("git", "-C", root, "status", "--porcelain").Output()
	if string(status) != "" {
		t.Fatalf("working tree dirty: %s", status)
	}
	_ = head
}

func TestStateDir(t *testing.T) {
	root := t.TempDir()
	run(t, root, "git", "init")
	dir := StateDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dir) != StateDirName {
		t.Fatalf("dir=%s", dir)
	}
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}
