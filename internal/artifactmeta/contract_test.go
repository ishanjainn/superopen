package artifactmeta_test

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/artifactmeta"
	"github.com/ishanjainn/superopen/internal/audit"
	"github.com/ishanjainn/superopen/internal/checkpoint"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/session"
	"github.com/ishanjainn/superopen/internal/tracestore"
)

func TestV2ArtifactsAreSelfDescribing(t *testing.T) {
	repo := t.TempDir()
	paths := harness.Resolve(repo)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	store := session.NewStore(paths)
	if err := store.Start(session.Meta{ID: "s1", Vendor: "codex", StartedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	traces := tracestore.NewLocalJSONL(paths.SessionsDir)
	if err := traces.Write([]tracestore.Span{{SessionID: "s1", Name: "prompt"}, {Name: "unresolved"}}); err != nil {
		t.Fatal(err)
	}
	if err := audit.Append(paths, audit.Event{Action: "sync"}); err != nil {
		t.Fatal(err)
	}
	if err := audit.Append(paths, audit.Event{Action: "allow", Session: "audit-session", Vendor: "codex"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.NewStore(paths).ReadDocument("audit-session"); err != nil {
		t.Fatalf("session-associated audit event must materialize session.json: %v", err)
	}
	mem := memory.NewStore(paths)
	if err := mem.Ensure(); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.RefreshActive(""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "exact.txt"), []byte("exact bytes\x00\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := checkpoint.NewStore(paths).Create("s1", repo, "contract", []string{"exact.txt"}, 0); err != nil {
		t.Fatal(err)
	}

	err := filepath.WalkDir(paths.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if strings.Contains(filepath.ToSlash(path), "/checkpoints/1/files/") {
			return nil // exact checkpoint payloads are documented by manifest.json
		}
		if err := artifactmeta.Validate(path); err != nil {
			t.Errorf("%s: %v", path, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	assertFirstLine(t, filepath.Join(paths.SessionDir("s1"), "events.jsonl"), `{"type":"superopen.file_manifest","purpose":"Normalized prompts, responses, tool calls, file activity, usage, lifecycle, and audit events for this session.","authority":"authoritative session event stream","updated_by":"vendor telemetry adapter"}`)
	assertFirstLine(t, filepath.Join(paths.SessionsDir, "inbox.jsonl"), `{"type":"superopen.file_manifest","purpose":"Temporary event spool for telemetry whose session ID has not been resolved.","authority":"temporary","updated_by":"telemetry ingestion"}`)
	assertFirstLine(t, filepath.Join(paths.AuditDir, "events.jsonl"), `{"type":"superopen.file_manifest","purpose":"Audit events that are not associated with a coding session.","authority":"append-only runtime history","updated_by":"Superopen CLI and hooks"}`)
	payload, err := os.ReadFile(filepath.Join(paths.SessionDir("s1"), "checkpoints", "1", "files", "exact.txt"))
	if err != nil || string(payload) != "exact bytes\x00\n" {
		t.Fatalf("checkpoint bytes changed: %q (%v)", payload, err)
	}
}

func assertFirstLine(t *testing.T, path, want string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	if !s.Scan() || s.Text() != want {
		t.Fatalf("%s first record:\n got %s\nwant %s", path, s.Text(), want)
	}
}
