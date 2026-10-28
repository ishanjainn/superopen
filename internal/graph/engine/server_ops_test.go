package engine

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func TestServerDispatchesCodeSearchIncrementalAndTraceIngest(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	repo := t.TempDir()
	server := Server{EngineVersion: "test"}
	for operation, params := range map[api.Operation]any{
		api.OpCodeSearch: api.CodeSearchRequest{RepoRoot: repo, Pattern: "func", Limit: 5},
		api.OpIncremental: api.IncrementalRequest{
			BuildRequest: api.BuildRequest{RepoRoot: repo},
			Changes:      api.ChangeSet{RequiresFull: true},
		},
		api.OpTraceIngest: api.TraceIngestRequest{
			RepoRoot: repo,
			Traces: []api.RuntimeTrace{
				{Source: "caller.main", Target: "callee.run", Type: "CALLS"},
			},
		},
	} {
		body, _ := json.Marshal(params)
		response := server.Handle(context.Background(), api.Request{Protocol: api.ProtocolVersion, Operation: operation, Params: body})
		if operation == api.OpIncremental {
			if response.OK || response.Error == nil || response.Error.Code != "incremental" {
				t.Fatalf("incremental without assets should fail: %+v", response.Error)
			}
			continue
		}
		if !response.OK {
			t.Fatalf("%s failed: %+v", operation, response.Error)
		}
	}
}

func TestTraceIngestAcceptsValidTraces(t *testing.T) {
	result, err := TraceIngest(context.Background(), t.TempDir(), api.TraceIngestRequest{
		Traces: []api.RuntimeTrace{
			{Source: "a.run", Target: "b.run"},
			{Source: "", Target: "missing"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Accepted != 1 || result.Rejected != 1 {
		t.Fatalf("result = %+v", result)
	}
}

func TestCodeSearchReturnsEmptyWithoutMatches(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := CodeSearch(context.Background(), root, api.CodeSearchRequest{Pattern: "missing-symbol-xyz"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 0 {
		t.Fatalf("matches = %#v", result.Matches)
	}
}

func TestCodeSearchTreatsDashPatternsAsLiteral(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "options.py")
	if err := os.WriteFile(path, []byte("first line\n-v = True\n--verbose = True\nflag -v here\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "registration.py"), []byte("option = (\"verbose\", {})\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, fallback := range []bool{false, true} {
		if !fallback {
			if _, err := exec.LookPath("rg"); err != nil {
				continue
			}
		}
		t.Run(map[bool]string{false: "ripgrep", true: "fallback"}[fallback], func(t *testing.T) {
			if fallback {
				t.Setenv("PATH", t.TempDir())
			}
			for _, tc := range []struct {
				pattern string
				lines   []int
			}{
				{"-v", []int{2, 3, 4}},
				{"--verbose", []int{3}},
				{" -v", []int{4}},
			} {
				result, err := CodeSearch(context.Background(), root, api.CodeSearchRequest{Pattern: tc.pattern, Limit: 10})
				if err != nil {
					t.Fatalf("pattern %q: %v", tc.pattern, err)
				}
				if len(result.Matches) != len(tc.lines) {
					t.Fatalf("pattern %q: matches = %#v", tc.pattern, result.Matches)
				}
				found := map[int]bool{}
				for _, match := range result.Matches {
					found[match.Location.StartLine] = true
				}
				for _, line := range tc.lines {
					if !found[line] {
						t.Fatalf("pattern %q: missing line %d in %#v", tc.pattern, line, result.Matches)
					}
				}
				if tc.pattern == "--verbose" {
					if len(result.NameMatches) != 1 || result.NameMatches[0].Location.File != "registration.py" {
						t.Fatalf("long flag should expose bare-name registration: %#v", result.NameMatches)
					}
				}
			}
		})
	}
}

func TestCodeSearchFallbackFindsMatchesWithoutRipgrep(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\nfunc HelloWorld() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir()) // ensure LookPath("rg") fails
	result, err := CodeSearch(context.Background(), root, api.CodeSearchRequest{Pattern: "HelloWorld", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 1 {
		t.Fatalf("matches = %#v", result.Matches)
	}
	if result.Matches[0].Location.File != "main.go" {
		t.Fatalf("file = %q", result.Matches[0].Location.File)
	}
}

func TestCodeSearchPathFilterAndLineNumbers(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "core", "mail"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "core", "mail", "utils.py"), []byte("import x\nDNS_NAME = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "other.py"), []byte("DNS_NAME = 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "excluded.py"), []byte("DNS_NAME = 3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, noRG := range []bool{false, true} {
		if noRG {
			t.Setenv("PATH", t.TempDir())
		}
		for _, filter := range []string{"core/mail", "./core/mail/", "core/mail/utils.py"} {
			result, err := CodeSearch(context.Background(), root, api.CodeSearchRequest{Pattern: "DNS_NAME", PathFilter: filter, Limit: 5})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Matches) != 1 || result.Matches[0].Location.File != "core/mail/utils.py" || result.Matches[0].Location.StartLine != 2 {
				t.Fatalf("noRG=%v filter=%q matches=%#v", noRG, filter, result.Matches)
			}
		}
		result, err := CodeSearch(context.Background(), root, api.CodeSearchRequest{Pattern: "DNS_NAME", Paths: []string{"core/mail", "other.py"}, Limit: 5})
		if err != nil {
			t.Fatal(err)
		}
		found := map[string]bool{}
		for _, match := range result.Matches {
			found[match.Location.File] = true
		}
		if len(result.Matches) != 2 || !found["core/mail/utils.py"] || !found["other.py"] {
			t.Fatalf("noRG=%v multi-path matches=%#v", noRG, result.Matches)
		}
	}
	if _, err := CodeSearch(context.Background(), root, api.CodeSearchRequest{Pattern: "DNS_NAME", PathFilter: "../etc"}); err == nil {
		t.Fatal("a path outside the repo must fail")
	}
}
