package githooks_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/superopen/so/internal/githooks"
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
