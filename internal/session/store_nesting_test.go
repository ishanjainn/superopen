package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/session/agentlinks"
	"github.com/ishanjainn/superopen/internal/tracestore"
)

func TestUpsertActiveFromSpansDoesNotPoisonParentWithSubagentType(t *testing.T) {
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
	now := time.Now().UnixNano()

	// Parent chat emits a subagent span ON ITS OWN session id (Cursor Task marker).
	// That must not mark the parent as is_subagent / hide it from the list.
	store.UpsertActiveFromSpans([]tracestore.Span{
		{
			Name:           "coding_agent.session",
			StartTimeUnixN: now,
			Attributes: map[string]string{
				"gen_ai.conversation.id":  "chat-poison",
				"coding_agent.session.id": "proc-parent",
				"coding_agent.client":     "cursor",
				"gen_ai.prompt":           "parent work",
				"gen_ai.user.name":        "dev@example.com",
			},
		},
		{
			Name:           "coding_agent.subagent",
			StartTimeUnixN: now + 1,
			Attributes: map[string]string{
				"gen_ai.conversation.id":    "chat-poison",
				"coding_agent.session.id":   "proc-parent",
				"coding_agent.agent.type":   "subagent",
				"coding_agent.client":       "cursor",
				"gen_ai.prompt":             "spawned work",
			},
		},
	})

	parent, err := store.Get("chat-poison")
	if err != nil {
		t.Fatalf("parent: %v", err)
	}
	if parent.IsSubagent || parent.ParentID != "" {
		t.Fatalf("parent must stay top-level, got %+v", parent)
	}
	if parent.User != "dev@example.com" {
		t.Fatalf("expected user stamp, got %+v", parent)
	}

	list, err := store.ListDetailed()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "chat-poison" {
		t.Fatalf("parent must list, got %+v", list)
	}
}

func TestResolveNestedParentClearsOrphanSubagentFlag(t *testing.T) {
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
	if err := store.Start(Meta{
		ID:            "orphan",
		Vendor:        "cursor",
		Status:        StatusActive,
		IsSubagent:    true,
		PromptPreview: "orphan work",
		StartedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	list, err := store.ListDetailed()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "orphan" {
		t.Fatalf("orphan is_subagent must list after repair, got %+v", list)
	}
	meta, err := store.Get("orphan")
	if err != nil {
		t.Fatal(err)
	}
	if meta.IsSubagent {
		t.Fatalf("orphan flag should be cleared: %+v", meta)
	}
}

func TestUpsertActiveFromSpansNestsSubagents(t *testing.T) {
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

	now := time.Now().UnixNano()
	store.UpsertActiveFromSpans([]tracestore.Span{
		{
			Name:           "coding_agent.session",
			StartTimeUnixN: now,
			Attributes: map[string]string{
				"gen_ai.conversation.id":  "chat-1",
				"coding_agent.session.id": "proc-a",
				"coding_agent.client":     "cursor",
				"gen_ai.prompt":           "hello main",
			},
		},
		{
			Name:           "coding_agent.tool.call",
			StartTimeUnixN: now + 1,
			Attributes: map[string]string{
				"gen_ai.conversation.id":       "sub-1",
				"coding_agent.session.id":      "proc-b",
				"coding_agent.agent.parent_id": "chat-1",
				"coding_agent.session.is_subagent": "true",
				"coding_agent.client":          "cursor",
				"gen_ai.prompt":                "subagent task",
			},
		},
	})

	parent, err := store.Get("chat-1")
	if err != nil {
		t.Fatalf("parent: %v", err)
	}
	if parent.ParentID != "" || parent.IsSubagent {
		t.Fatalf("parent should be top-level: %+v", parent)
	}

	child, err := store.Get("sub-1")
	if err != nil {
		t.Fatalf("child: %v", err)
	}
	if child.ParentID != "chat-1" || !child.IsSubagent {
		t.Fatalf("child linkage: %+v", child)
	}

	list, err := store.ListDetailed()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "chat-1" {
		t.Fatalf("list should be one root session, got %+v", list)
	}

	kids, err := store.Children("chat-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(kids) != 1 || kids[0].ID != "sub-1" {
		t.Fatalf("children: %+v", kids)
	}
}

func TestUpsertActiveFromSpansSameThreadOneSession(t *testing.T) {
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
	now := time.Now().UnixNano()

	// Two ephemeral session ids, same conversation - one chat row.
	store.UpsertActiveFromSpans([]tracestore.Span{
		{
			StartTimeUnixN: now,
			Attributes: map[string]string{
				"gen_ai.conversation.id":  "chat-stable",
				"coding_agent.session.id": "ephemeral-1",
				"coding_agent.client":     "cursor",
				"gen_ai.prompt":           "turn one",
			},
		},
		{
			StartTimeUnixN: now + 2,
			Attributes: map[string]string{
				"gen_ai.conversation.id":  "chat-stable",
				"coding_agent.session.id": "ephemeral-2",
				"coding_agent.client":     "cursor",
				"gen_ai.prompt":           "turn two",
			},
		},
	})

	list, err := store.ListDetailed()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "chat-stable" {
		t.Fatalf("want one stable chat row, got %+v", list)
	}
	if _, err := store.Get("ephemeral-1"); err == nil {
		t.Fatal("ephemeral session id should not be a session row")
	}
}

func TestUpsertActiveFromSpansUsesAgentLinks(t *testing.T) {
	root := t.TempDir()
	paths := harness.Paths{
		Root:          filepath.Join(root, ".so"),
		SessionsDir:   filepath.Join(root, ".so", "sessions"),
		SessionsIndex: filepath.Join(root, ".so", "sessions", "index.json"),
	}
	if err := os.MkdirAll(paths.SessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	child := "9e24ef36-3521-462c-b43e-e552bbf0f807"
	parent := "f0c187a2-93b7-4d06-ac3b-7ca9f6539b1a"
	if err := agentlinks.Register(paths.SessionsDir, child, parent, "cursor", "test"); err != nil {
		t.Fatal(err)
	}
	store := NewStore(paths)
	now := time.Now().UnixNano()
	store.UpsertActiveFromSpans([]tracestore.Span{
		{
			Name:           "coding_agent.llm.turn",
			StartTimeUnixN: now,
			Attributes: map[string]string{
				"gen_ai.conversation.id":  child,
				"coding_agent.session.id": child,
				"coding_agent.client":     "cursor",
				"gen_ai.prompt":           "subagent turn",
			},
		},
	})
	meta, err := store.Get(child)
	if err != nil {
		t.Fatal(err)
	}
	if meta.ParentID != parent || !meta.IsSubagent {
		t.Fatalf("expected nested via agent-links, got %+v", meta)
	}
	list, err := store.ListDetailed()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range list {
		if item.ID == child {
			t.Fatalf("child must not appear in top-level list: %+v", list)
		}
	}
}
