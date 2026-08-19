package watch

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A recorded signature is what lets a freshly spawned MCP server skip a full
// rebuild of a graph that is already current.
func TestSignatureRoundTrip(t *testing.T) {
	root := initGitRepo(t)

	if got := LoadSignature(root); got != "" {
		t.Fatalf("expected no signature before any build, got %q", got)
	}

	RecordSignature(root)
	recorded := LoadSignature(root)
	if recorded == "" {
		t.Fatal("signature was not recorded")
	}
	if recorded != gitSignature(root) {
		t.Fatal("recorded signature does not match the working tree")
	}
}

func TestSignatureChangesWithWorkingTree(t *testing.T) {
	root := initGitRepo(t)
	RecordSignature(root)
	before := LoadSignature(root)

	if err := os.WriteFile(filepath.Join(root, "new.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}

	if gitSignature(root) == before {
		t.Fatal("a dirty working tree must produce a different signature")
	}
}

// Superopen writes into .so constantly. If that churn moved the signature,
// every poll would rebuild the graph it just built.
func TestSignatureIgnoresSuperopenStateDir(t *testing.T) {
	root := initGitRepo(t)
	before := gitSignature(root)

	if err := os.WriteFile(filepath.Join(root, ".so", "some-state"), []byte("churn"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".so", "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".so", "db", "so.db"), []byte("db"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := gitSignature(root); got != before {
		t.Fatalf("state-dir churn changed the signature:\nbefore %q\nafter  %q", before, got)
	}
}

func TestSignatureHelpersIgnoreEmptyRoot(t *testing.T) {
	RecordSignature("")
	if got := LoadSignature(""); got != "" {
		t.Fatalf("expected empty result, got %q", got)
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".so"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "init")
	return root
}
