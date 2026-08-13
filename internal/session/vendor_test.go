package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/tracestore"
)

func TestVendorFromAttrsPrefersClient(t *testing.T) {
	got := VendorFromAttrs(map[string]string{
		"coding_agent.client": "claude-code",
		"gen_ai.agent.name":   "claude-code",
		// coding_agent.vendor intentionally absent — live Claude Code spans
	})
	if got != "claude-code" {
		t.Fatalf("got %q", got)
	}
}

func TestStartDoesNotClearDetectedVendor(t *testing.T) {
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
	id := "ses_sticky_vendor"
	if err := store.Start(Meta{
		ID:        id,
		Vendor:    "claude-code",
		Model:     "claude-sonnet-5",
		StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	// Finalize-style Start that only looked at missing coding_agent.vendor.
	if err := store.Start(Meta{
		ID:        id,
		Vendor:    "",
		StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	meta, err := store.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Vendor != "claude-code" {
		t.Fatalf("vendor cleared to %q", meta.Vendor)
	}
	if meta.Model != "claude-sonnet-5" {
		t.Fatalf("model cleared to %q", meta.Model)
	}
}

func TestMaterializeFillsVendorFromClientWhenVendorKeyMissing(t *testing.T) {
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
	id := "ses_materialize_vendor"
	if err := store.Start(Meta{ID: id, Vendor: "", StartedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	spans := []tracestore.Span{{
		Name:           "coding_agent.llm.turn",
		StartTimeUnixN: time.Now().UnixNano(),
		Attributes: map[string]string{
			"coding_agent.client": "claude-code",
			"gen_ai.agent.name":   "claude-code",
			"gen_ai.prompt":       "scan repo",
		},
	}}
	meta, err := store.MaterializeFromSpans(id, spans, 10, 0.01)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Vendor != "claude-code" {
		t.Fatalf("vendor=%q want claude-code", meta.Vendor)
	}
	raw, err := store.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if raw.Vendor != "claude-code" {
		t.Fatalf("persisted vendor=%q", raw.Vendor)
	}
}

func TestMaterializeRepairsEpochSessionStart(t *testing.T) {
	root := t.TempDir()
	paths := harness.Resolve(root)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	store := NewStore(paths)
	if err := store.Start(Meta{ID: "epoch", Vendor: "codex", StartedAt: time.Unix(0, 0).UTC()}); err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC().Add(-time.Minute)
	meta, err := store.MaterializeFromSpans("epoch", []tracestore.Span{{
		Name: "coding_agent.tool.call", StartTimeUnixN: started.UnixNano(),
		Attributes: map[string]string{"coding_agent.vendor": "codex", "gen_ai.tool.name": "Read"},
	}}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if meta.StartedAt.Before(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("epoch timestamp was retained: %s", meta.StartedAt)
	}
	if meta.DurationMs < 0 || meta.DurationMs > int64((5*time.Minute)/time.Millisecond) {
		t.Fatalf("implausible duration after repair: %dms", meta.DurationMs)
	}
}
