package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHumanizePromptPreviewCursorMessages(t *testing.T) {
	raw := `[{"parts":[{"content":"I have this logo in text.png in downalods, use that","type":"text"}],"role":"user"}]`
	got := humanizePromptPreview(raw)
	want := "I have this logo in text.png in downalods, use that"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestHumanizePromptPreviewPlainText(t *testing.T) {
	if got := humanizePromptPreview("Add health check"); got != "Add health check" {
		t.Fatalf("got %q", got)
	}
}

func TestDisplayNameUsesHumanizedPreview(t *testing.T) {
	m := Meta{
		ID:            "abc",
		PromptPreview: `[{"parts":[{"content":"Fix the sessions UI","type":"text"}],"role":"user"}]`,
	}
	if got := DisplayName(m); got != "Fix the sessions UI" {
		t.Fatalf("got %q", got)
	}
}

func TestLookupPiSessionName(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PI_CODING_AGENT_DIR", root)
	dir := filepath.Join(root, "sessions", "proj")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	id := "019fc93a-5dd7-7731-8f98-48634d94ffe7"
	path := filepath.Join(dir, "2026-08-03T20-04-33-367Z_"+id+".jsonl")
	body := `{"type":"session","version":3,"id":"` + id + `","cwd":"/tmp/proj"}
{"type":"message","message":{"role":"user","content":[{"type":"text","text":"hi"}]}}
{"type":"session_info","name":"Wire OTLP exporter"}
{"type":"session_info","name":"Fix Pi session titles"}
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := lookupPiSessionName(id); got != "Fix Pi session titles" {
		t.Fatalf("got %q", got)
	}
	meta := &Meta{ID: id, Vendor: "pi", PromptPreview: "hi"}
	EnsureTitle(meta, nil)
	if meta.Title != "Fix Pi session titles" {
		t.Fatalf("EnsureTitle title=%q", meta.Title)
	}
}

func TestLookupPiSessionNameMissingInfoFallsThrough(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PI_CODING_AGENT_DIR", root)
	dir := filepath.Join(root, "sessions", "proj")
	_ = os.MkdirAll(dir, 0o755)
	id := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	path := filepath.Join(dir, "x_"+id+".jsonl")
	_ = os.WriteFile(path, []byte(`{"type":"session","id":"`+id+`"}
{"type":"message","message":{"role":"user","content":"only prompt"}}
`), 0o644)
	if got := lookupPiSessionName(id); got != "" {
		t.Fatalf("expected empty without session_info, got %q", got)
	}
}

func TestLookupOpenCodeTitleFromJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// UserHomeDir reads HOME on Unix.
	dir := filepath.Join(home, ".opencode", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	id := "ses_testtitle123"
	doc := `{"info":{"id":"` + id + `","title":"GCP Secret Manager cost alternatives","directory":"/tmp"}}`
	if err := os.WriteFile(filepath.Join(dir, id+".json"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := lookupOpenCodeTitle(id); got != "GCP Secret Manager cost alternatives" {
		t.Fatalf("got %q", got)
	}
	meta := &Meta{ID: id, Vendor: "opencode", PromptPreview: "what about costs"}
	EnsureTitle(meta, nil)
	if meta.Title != "GCP Secret Manager cost alternatives" {
		t.Fatalf("EnsureTitle title=%q", meta.Title)
	}
}

func TestLookupOpenCodeTitleIgnoresStubID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".opencode", "sessions")
	_ = os.MkdirAll(dir, 0o755)
	id := "ses_stub"
	_ = os.WriteFile(filepath.Join(dir, id+".json"), []byte(`{"info":{"id":"`+id+`","title":"`+id+`"}}`), 0o644)
	if got := lookupOpenCodeTitle(id); got != "" {
		t.Fatalf("stub title should be ignored, got %q", got)
	}
}

func TestLookupOpenCodeTitleIgnoresNewSessionStub(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".opencode", "sessions")
	_ = os.MkdirAll(dir, 0o755)
	id := "ses_newstub"
	doc := `{"info":{"id":"` + id + `","title":"New session - 2026-08-07T16:57:04.810Z"}}`
	_ = os.WriteFile(filepath.Join(dir, id+".json"), []byte(doc), 0o644)
	if got := lookupOpenCodeTitle(id); got != "" {
		t.Fatalf("New session stub should be ignored, got %q", got)
	}
}

func TestEnsureTitleRefreshesOpenCodePlaceholder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".opencode", "sessions")
	_ = os.MkdirAll(dir, 0o755)
	id := "ses_refresh"
	doc := `{"info":{"id":"` + id + `","title":"Repo scan request"}}`
	_ = os.WriteFile(filepath.Join(dir, id+".json"), []byte(doc), 0o644)
	meta := &Meta{
		ID:    id,
		Vendor: "opencode",
		Title: "New session - 2026-08-07T16:57:04.810Z",
	}
	EnsureTitle(meta, nil)
	if meta.Title != "Repo scan request" {
		t.Fatalf("EnsureTitle title=%q", meta.Title)
	}
}

func TestIsPlaceholderTitle(t *testing.T) {
	if !IsPlaceholderTitle("New session - 2026-08-07T16:57:04.810Z", "ses_x") {
		t.Fatal("expected New session stub")
	}
	if IsPlaceholderTitle("Repo scan", "ses_x") {
		t.Fatal("real title should not be placeholder")
	}
}
