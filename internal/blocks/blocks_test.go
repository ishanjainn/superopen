package blocks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHintOneLineNeverGraph(t *testing.T) {
	if got := Hint(`go test ./... | tail -20`); got != "use so blocks run test-summary" {
		t.Fatalf("test tail: %q", got)
	}
	if got := Hint(`so graph query "where is main"`); got != "" {
		t.Fatalf("graph command must stay quiet, got %q", got)
	}
	if got := Hint(`grep -n "func " *.go`); got != "" {
		t.Fatalf("plain grep must stay quiet, got %q", got)
	}
	if got := Hint(`curl https://example.com/health`); got != "" {
		t.Fatalf("plain curl must stay quiet, got %q", got)
	}
	if strings.Contains(Hint(`python -c 'import json; json.loads(open("a").read())'`), "\n") {
		t.Fatal("hint must be one line")
	}
}

func TestJSONPeekAndStats(t *testing.T) {
	peek, err := JSONPeek(strings.NewReader(`{"name":"a","n":1,"tags":["x"]}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(peek, "name string") || !strings.Contains(peek, "object keys=3") {
		t.Fatalf("peek: %s", peek)
	}
	stats, err := JSONLStats(strings.NewReader("{\"a\":1}\n{\"a\":2,\"b\":3}\n\nnot-json\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stats, "lines=3") || !strings.Contains(stats, "bad=1") || !strings.Contains(stats, "a 2") {
		t.Fatalf("stats: %s", stats)
	}
}

func TestFirstErrorAndReplace(t *testing.T) {
	got := FirstError("ok\nERROR boom\nnext\nmore\n")
	if !strings.HasPrefix(got, "ERROR boom\nnext\nmore\n") {
		t.Fatalf("first error: %q", got)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("alpha beta"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceInFile(path, "beta", "gamma"); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(path)
	if string(body) != "alpha gamma" {
		t.Fatalf("replaced: %q", body)
	}
}

func TestInitDoesNotWriteAgents(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatal("init wrote AGENTS.md")
	}
	if err := Apply(root); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), GrepDefsNote) || !strings.Contains(string(body), markerBegin) {
		t.Fatalf("applied index missing note: %s", body)
	}
}

func TestScanRanksLocalCommands(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".so", "sessions", "s1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"attributes":{"coding_agent.tool.command":"python -c 'print(1)' | head"}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(line+line), 0o644); err != nil {
		t.Fatal(err)
	}
	hits, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Count != 2 {
		t.Fatalf("hits: %+v", hits)
	}
}

func TestWaitFor(t *testing.T) {
	n := 0
	err := WaitFor(time.Second, func() (bool, error) {
		n++
		return n == 2, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
