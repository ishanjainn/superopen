package forward

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/paths"
)

func TestRunCopiesNewLinesAndRemembersCursor(t *testing.T) {
	root := t.TempDir()
	layout := paths.Resolve(root)
	if err := layout.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	sess := filepath.Join(layout.SessionsDir, "s1")
	if err := os.MkdirAll(sess, 0o755); err != nil {
		t.Fatal(err)
	}
	line := []byte("{\"name\":\"coding_agent.tool.call\"}\n")
	if err := os.WriteFile(filepath.Join(sess, "events.jsonl"), line, 0o644); err != nil {
		t.Fatal(err)
	}
	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got = append(got, body...)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	cfgPath := filepath.Join(layout.Root, "forward", "config.json")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(`{"url":"`+srv.URL+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := Run(root)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || string(got) != string(line) {
		t.Fatalf("copied %d body %q", n, got)
	}
	if _, err := os.Stat(filepath.Join(sess, "cursor.json")); !os.IsNotExist(err) {
		t.Fatal("cursor must not be written inside the session directory")
	}
	n, err = Run(root)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("second run copied %d", n)
	}
}
