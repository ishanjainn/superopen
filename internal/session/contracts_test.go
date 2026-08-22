package session

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/artifact"
	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/session/trace"
)

func testSessionStore(t *testing.T) (*Store, string) {
	t.Helper()
	root := t.TempDir()
	p := paths.Paths{
		Root:          filepath.Join(root, ".so"),
		SessionsDir:   filepath.Join(root, ".so", "sessions"),
		SessionsIndex: filepath.Join(root, ".so", "sessions", "index.json"),
	}
	if err := os.MkdirAll(p.SessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return NewStore(p), root
}

func TestMaterializeFootprintFromToolCallArguments(t *testing.T) {
	store, _ := testSessionStore(t)
	id := "sess-foot"
	if err := store.Start(Meta{ID: id, Vendor: "cursor", StartedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	spans := []trace.Span{{
		Name: "coding_agent.tool.call", SpanID: "t1", SessionID: id,
		StartTimeUnixN: time.Now().UnixNano(),
		Attributes: map[string]string{
			"gen_ai.tool.name":            "Read",
			"gen_ai.tool.call.arguments":  `{"file_path":"src/app.ts"}`,
		},
	}}
	meta, err := store.MaterializeFromSpans(id, spans, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	_ = meta
	fp, err := store.GetFootprint(id)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range fp.Files {
		if f.Path == "src/app.ts" {
			found = true
			if f.State != "read" {
				t.Fatalf("state=%q", f.State)
			}
		}
	}
	if !found {
		t.Fatalf("expected src/app.ts in footprint: %+v", fp.Files)
	}
}

func TestMaterializeIgnoresShellCommandPath(t *testing.T) {
	store, _ := testSessionStore(t)
	id := "sess-shell"
	if err := store.Start(Meta{ID: id, Vendor: "cursor", StartedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	spans := []trace.Span{{
		Name: "coding_agent.tool.call", SpanID: "t1", SessionID: id,
		StartTimeUnixN: time.Now().UnixNano(),
		Attributes: map[string]string{
			"gen_ai.tool.name":           "shell",
			"coding_agent.file_path":     `so graph query "who wraps app"`,
			"gen_ai.tool.call.arguments": `so graph query "who wraps app"`,
		},
	}, {
		Name: "coding_agent.tool.call", SpanID: "t2", SessionID: id,
		StartTimeUnixN: time.Now().UnixNano(),
		Attributes: map[string]string{
			"gen_ai.tool.name":           "Read",
			"gen_ai.tool.call.arguments": `{"file_path":"Makefile"}`,
		},
	}}
	if _, err := store.MaterializeFromSpans(id, spans, 0, 0); err != nil {
		t.Fatal(err)
	}
	fp, err := store.GetFootprint(id)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range fp.Files {
		if strings.Contains(f.Path, "so graph") || strings.Contains(f.Path, "query") {
			t.Fatalf("shell command in footprint: %+v", fp.Files)
		}
	}
	found := false
	for _, f := range fp.Files {
		if f.Path == "Makefile" && f.State == "read" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected Makefile read, got %+v", fp.Files)
	}
}

func TestSessionCostCountsUsageOnce(t *testing.T) {
	dir := t.TempDir()
	s := trace.NewLocalJSONL(dir)
	id := "sess-cost"
	now := time.Now().UnixNano()
	if err := s.Write([]trace.Span{
		{Name: "coding_agent.session", SpanID: "root", SessionID: id, StartTimeUnixN: now,
			Attributes: map[string]string{"gen_ai.usage.total_tokens": "1000"}},
		{Name: "coding_agent.session.loop.stop", SpanID: "stop", SessionID: id, StartTimeUnixN: now + 1,
			Attributes: map[string]string{"gen_ai.usage.total_tokens": "1000"}},
	}); err != nil {
		t.Fatal(err)
	}
	tokens, _, err := s.SessionCost(id)
	if err != nil {
		t.Fatal(err)
	}
	if tokens != 1000 {
		t.Fatalf("tokens=%d want 1000 (counted once)", tokens)
	}
}

func TestMaterializeKeepsVCSRevisionAndRedactsPromptSecrets(t *testing.T) {
	store, _ := testSessionStore(t)
	id := "sess-vcs"
	if err := store.Start(Meta{ID: id, Vendor: "cursor", StartedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	sha := strings.Repeat("a", 40)
	secret := "ghp_" + strings.Repeat("a", 36)
	spans := []trace.Span{
		{Name: "coding_agent.session", SpanID: "root", SessionID: id, StartTimeUnixN: time.Now().UnixNano(),
			Attributes: map[string]string{"vcs.ref.head.revision": sha}},
		{Name: "coding_agent.llm.turn", SpanID: "p", SessionID: id, StartTimeUnixN: time.Now().UnixNano(),
			Attributes: map[string]string{"gen_ai.prompt": "rotate " + secret}},
	}
	meta, err := store.MaterializeFromSpans(id, spans, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if meta.HeadSHA != sha {
		t.Fatalf("head sha=%q want %q", meta.HeadSHA, sha)
	}
	events := filepath.Join(store.Paths.SessionDir(id), "events.jsonl")
	body, err := os.ReadFile(events)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), sha) {
		t.Fatalf("events.jsonl lost SHA: %s", body)
	}
	if strings.Contains(string(body), secret) {
		t.Fatalf("prompt secret survived redact: %s", body)
	}
}

func TestMaterializeDoesNotWipeLongerEventsFile(t *testing.T) {
	store, _ := testSessionStore(t)
	id := "sess-keep"
	if err := store.Start(Meta{ID: id, Vendor: "cursor", StartedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	events := filepath.Join(store.Paths.SessionDir(id), "events.jsonl")
	if err := artifact.EnsureJSONL(events, artifact.About{Purpose: "test", Authority: "test", UpdatedBy: "test"}); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(events, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	enc := json.NewEncoder(f)
	now := time.Now().UnixNano()
	for i := 0; i < 50; i++ {
		if err := enc.Encode(trace.Span{Name: "coding_agent.llm.turn", SpanID: "e" + string(rune('a'+i%26)), SessionID: id, StartTimeUnixN: now + int64(i)}); err != nil {
			t.Fatal(err)
		}
	}
	_ = f.Close()
	query := make([]trace.Span, 5)
	for i := range query {
		query[i] = trace.Span{Name: "coding_agent.llm.turn", SpanID: "q", SessionID: id, StartTimeUnixN: now}
	}
	if _, err := store.MaterializeFromSpans(id, query, 0, 0); err != nil {
		t.Fatal(err)
	}
	n := 0
	in, err := os.Open(events)
	if err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(in)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var raw map[string]any
		if json.Unmarshal([]byte(line), &raw) != nil {
			continue
		}
		if name, _ := raw["name"].(string); name != "" {
			n++
		}
	}
	_ = in.Close()
	if n < 50 {
		t.Fatalf("finalize wiped events: got %d want >= 50", n)
	}
}
