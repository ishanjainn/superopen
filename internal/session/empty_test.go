package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/tracestore"
)

func TestSpansHaveActivity(t *testing.T) {
	if SpansHaveActivity(nil) {
		t.Fatal("empty spans")
	}
	if SpansHaveActivity([]tracestore.Span{{
		Name: "coding_agent.session",
		Attributes: map[string]string{
			"coding_agent.client":  "cursor",
			"gen_ai.request.model": "gpt",
			"gen_ai.user.name":     "a@b.com",
			"vcs.ref.head.name":    "main",
		},
	}}) {
		t.Fatal("identity-only spans must not count as activity")
	}
	if !SpansHaveActivity([]tracestore.Span{{
		Name: "coding_agent.prompt",
		Attributes: map[string]string{
			"gen_ai.prompt": "hello",
		},
	}}) {
		t.Fatal("prompt must count")
	}
	if !SpansHaveActivity([]tracestore.Span{{
		Name: "coding_agent.read",
		Attributes: map[string]string{
			"coding_agent.file_path": "main.go",
		},
	}}) {
		t.Fatal("file tool must count")
	}
}

func TestIsEmptyListItem(t *testing.T) {
	empty := ListItem{Meta: Meta{ID: "x", Status: StatusActive, StartedAt: time.Now()}}
	if !IsEmptyListItem(empty) {
		t.Fatal("expected empty")
	}
	withPrompt := empty
	withPrompt.PromptPreview = "hi"
	if IsEmptyListItem(withPrompt) {
		t.Fatal("prompt means not empty")
	}
	withTurns := empty
	withTurns.Turns = 1
	if IsEmptyListItem(withTurns) {
		t.Fatal("turns means not empty")
	}
}

func TestListDetailedKeepsToolOnlyLiveSession(t *testing.T) {
	root := t.TempDir()
	paths := harness.Resolve(root)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	store := NewStore(paths)
	if err := store.Start(Meta{ID: "tool-session", Vendor: "claude-code", Status: StatusActive, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	events := filepath.Join(paths.SessionDir("tool-session"), "events.jsonl")
	body := `{"name":"coding_agent.tool.call","attributes":{"gen_ai.tool.name":"Write","code.file.path":"main.go"}}` + "\n"
	if err := os.WriteFile(events, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListDetailed()
	if err != nil || len(items) != 1 || items[0].ID != "tool-session" {
		t.Fatalf("items = %#v, %v", items, err)
	}
}

func TestUpsertSkipsIdentityOnly(t *testing.T) {
	root := t.TempDir()
	paths := harness.Paths{
		Root:          filepath.Join(root, ".so"),
		SessionsDir:   filepath.Join(root, ".so", "sessions"),
		SessionsIndex: filepath.Join(root, ".so", "sessions", "index.json"),
	}
	if err := os.MkdirAll(paths.SessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	store := NewStore(paths)
	store.UpsertActiveFromSpans([]tracestore.Span{{
		Name:           "coding_agent.session",
		StartTimeUnixN: time.Now().UnixNano(),
		Attributes: map[string]string{
			"gen_ai.conversation.id": "empty-chat",
			"coding_agent.client":    "cursor",
			"gen_ai.request.model":   "gpt",
			"gen_ai.user.name":       "a@b.com",
		},
	}})
	if _, err := store.Get("empty-chat"); err == nil {
		t.Fatal("identity-only upsert must not create a session")
	}
	store.UpsertActiveFromSpans([]tracestore.Span{{
		Name:           "coding_agent.prompt",
		StartTimeUnixN: time.Now().UnixNano(),
		Attributes: map[string]string{
			"gen_ai.conversation.id": "real-chat",
			"coding_agent.client":    "cursor",
			"gen_ai.prompt":          "do something",
		},
	}})
	if _, err := store.Get("real-chat"); err != nil {
		t.Fatalf("prompt upsert should create session: %v", err)
	}
	list, err := store.ListDetailed()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "real-chat" {
		t.Fatalf("list=%+v", list)
	}
}
