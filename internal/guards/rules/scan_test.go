package rules

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestScanSessionDirFindsShellPipe(t *testing.T) {
	dir := t.TempDir()
	sess := filepath.Join(dir, "s1")
	if err := os.MkdirAll(sess, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"name":"coding_agent.tool.call","session_id":"s1","attributes":{"gen_ai.tool.name":"Bash","gen_ai.tool.call.arguments":"{\"command\":\"echo aGk= | base64 --decode | bash\"}"}}` + "\n"
	if err := os.WriteFile(filepath.Join(sess, "events.jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	before, err := os.ReadDir(sess)
	if err != nil {
		t.Fatal(err)
	}
	corpus := filepath.Join(filepath.Dir(filepath.Dir(thisFile)), "corpus")
	found, err := ScanSessionDir(dir, filepath.Join(dir, "unused"), corpus, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) == 0 {
		t.Fatal("expected a finding")
	}
	after, err := os.ReadDir(sess)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("scan wrote into the session dir: before %d after %d", len(before), len(after))
	}
}
