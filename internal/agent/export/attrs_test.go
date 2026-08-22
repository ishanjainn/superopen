package export

import (
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/agent/normalize"
	"github.com/ishanjainn/superopen/internal/redact"
)

func TestInferProvider(t *testing.T) {
	tests := []struct {
		model, vendor, want string
	}{
		{"claude-opus-4-8", "cursor", "anthropic"},
		{"claude-opus-4-8-thinking-high", "cursor", "anthropic"},
		{"gpt-5.5", "codex", "openai"},
		{"gpt-5.6-sol-medium", "cursor", "openai"},
		{"cursor-grok-4.5-high", "cursor", "xai"},
		{"grok-4.5", "cursor", "xai"},
		{"composer-2.5", "cursor", "cursor"},
		{"composer-2-5", "cursor", "cursor"},
		{"auto", "cursor", "cursor"},
		{"gemini-3-pro", "", "google"},
		{"kimi-k2.7-code", "cursor", "moonshot"},
		{"", "cursor", "cursor"},
		{"", "codex", "openai"},
		{"", "claude-code", "anthropic"},
		{"totally-unknown", "", ""},
	}
	for _, tt := range tests {
		got := inferProvider(tt.model, tt.vendor)
		if got != tt.want {
			t.Errorf("inferProvider(%q, %q) = %q, want %q", tt.model, tt.vendor, got, tt.want)
		}
	}
}

func TestToolCallFilePathJSONOnlyNotShellCommand(t *testing.T) {
	got := toolCallFilePath(normalize.ToolCall{
		ToolName: "Read",
		Args:     `{"file_path":"src/app.ts"}`,
	})
	if got != "src/app.ts" {
		t.Fatalf("read json: %q", got)
	}
	got = toolCallFilePath(normalize.ToolCall{ToolName: "Read", FilePath: "Makefile"})
	if got != "Makefile" {
		t.Fatalf("makefile: %q", got)
	}
	cmd := `so graph query "who wraps app"`
	got = toolCallFilePath(normalize.ToolCall{ToolName: "shell", Args: cmd})
	if got != "" {
		t.Fatalf("cursor shell args: %q", got)
	}
	got = toolCallFilePath(normalize.ToolCall{ToolName: "Bash", Args: cmd})
	if got != "" {
		t.Fatalf("generic bash args: %q", got)
	}
}

func TestVCSRevisionSurvivesStringFullScrub(t *testing.T) {
	sha := strings.Repeat("a", 40)
	if redact.StringFull(sha) == sha {
		t.Fatal("StringFull should redact a 40-hex SHA")
	}
	if !redact.UnredactedAttr("vcs.ref.head.revision") {
		t.Fatal("VCS revision must skip scrub")
	}
	if redact.UnredactedAttr("gen_ai.prompt") {
		t.Fatal("prompts must still scrub")
	}
}
