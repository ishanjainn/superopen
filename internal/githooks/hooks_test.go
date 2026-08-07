package githooks_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/githooks"
)

func TestAppendAndParseTrailers(t *testing.T) {
	dir := t.TempDir()
	msg := filepath.Join(dir, "COMMIT_EDITMSG")
	if err := os.WriteFile(msg, []byte("hello world\n\n# comment\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := githooks.AppendTrailer(msg, githooks.TrailerSession, "sess-1"); err != nil {
		t.Fatal(err)
	}
	if err := githooks.AppendTrailer(msg, githooks.TrailerAttribution, "80% agent"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(msg)
	s := string(data)
	if !strings.Contains(s, "SO-Session: sess-1") {
		t.Fatalf("missing session trailer: %s", s)
	}
	if !strings.Contains(s, "SO-Attribution: 80% agent") {
		t.Fatalf("missing attribution: %s", s)
	}
	sid, attr := githooks.ParseTrailers(s)
	if sid != "sess-1" || attr != "80% agent" {
		t.Fatalf("parse got %q %q", sid, attr)
	}
	// idempotent
	_ = githooks.AppendTrailer(msg, githooks.TrailerSession, "other")
	data, _ = os.ReadFile(msg)
	if strings.Count(string(data), "SO-Session:") != 1 {
		t.Fatalf("duplicate trailer: %s", data)
	}
}

func TestInstallRemovesSuperopenHooks(t *testing.T) {
	dir := t.TempDir()
	hooks := filepath.Join(dir, ".git", "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	// fake git repo for rev-parse --git-path hooks
	if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prePush := filepath.Join(hooks, "pre-push")
	body := "#!/bin/sh\n# Superopen pre-push\nexec so githook pre-push \"$@\"\n"
	if err := os.WriteFile(prePush, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(hooks, "pre-commit")
	if err := os.WriteFile(foreign, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := githooks.Install(dir, "so"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(prePush); !os.IsNotExist(err) {
		t.Fatalf("expected Superopen pre-push removed, err=%v", err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatalf("foreign hook should remain: %v", err)
	}
}
