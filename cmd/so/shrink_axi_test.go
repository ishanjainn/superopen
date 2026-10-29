package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/compact"
)

func TestShrinkAndRetrieveAXI(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".so"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cliFlags.Root = ""; cliFlags.JSON = false; cliFlags.Full = false })
	run := func(args ...string) string {
		t.Helper()
		cliFlags.Root, cliFlags.JSON, cliFlags.Full = "", false, false
		return captureStdout(t, func() {
			cmd := newRootCommand()
			cmd.SetArgs(append([]string{"--root", root}, args...))
			cmd.SetOut(os.Stdout)
			cmd.SetErr(io.Discard)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
		})
	}
	if got := run("shrink", "--", "printf", "hello"); !strings.Contains(got, "bytes_before: 5\ncontent: hello") {
		t.Fatalf("structured shrink: %q", got)
	}
	if got := run("shrink", "--raw", "--", "printf", "hello"); got != "hello" {
		t.Fatalf("raw shrink: %q", got)
	}
	plain := strings.Repeat("--- PASS: TestWorking\n", 100) + "--- FAIL: TestHidden\nexpected 12 nodes, got 9\n"
	plainPath := filepath.Join(root, "plain-test.txt")
	if err := os.WriteFile(plainPath, []byte(plain), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := run("shrink", "--", "cat", plainPath); !strings.Contains(got, "TestHidden") || strings.Contains(got, "use --full") {
		t.Fatalf("pass-through output hid the failure: %q", got)
	}
	if got := run("blocks", "run", "test-summary", plainPath); !strings.Contains(got, "TestHidden") || strings.Contains(got, "use --full") {
		t.Fatalf("unrecognized test report hid the failure: %q", got)
	}
	junit := "<testsuite tests=\"102\" failures=\"2\">\n" + strings.Repeat("<testcase name=\"pass\"/>\n", 100) +
		"<testcase name=\"FirstFailure\"><failure message=\"first\">first detail</failure></testcase>\n" +
		"<testcase name=\"SecondFailure\"><failure message=\"second\">second detail</failure></testcase>\n</testsuite>\n"
	junitPath := filepath.Join(root, "junit.xml")
	if err := os.WriteFile(junitPath, []byte(junit), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"shrink", "--", "cat", junitPath}, {"blocks", "run", "test-summary", junitPath}} {
		if got := run(args...); !strings.Contains(got, "FirstFailure") || !strings.Contains(got, "SecondFailure") || strings.Contains(got, "use --full") {
			t.Fatalf("compressed report hid a failure: %q", got)
		}
	}
	var encoded map[string]any
	if got := run("--json", "shrink", "--", "printf", "hello"); json.Unmarshal([]byte(got), &encoded) != nil || encoded["kind"] != "shrink" {
		t.Fatalf("json shrink: %q", got)
	}
	if _, ok := encoded["next"].([]any); !ok {
		t.Fatalf("next must be a list: %+v", encoded)
	}
	rows := make([]string, 4000)
	for i := range rows {
		rows[i] = "repeated long value"
	}
	input, _ := json.Marshal(rows)
	result := compact.CompressAny(root, input, "json")
	if result.Handle == "" {
		t.Fatal("fixture did not compress")
	}
	if got := run("retrieve", result.Handle); !strings.Contains(got, "content:") || !strings.Contains(got, "use --full") {
		t.Fatalf("bounded retrieve: %q", got)
	}
	if got := run("retrieve", "--raw", result.Handle); got != string(input) {
		t.Fatal("raw retrieve changed original bytes")
	}
}
