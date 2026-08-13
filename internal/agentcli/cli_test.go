package agentcli

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestParseClaudeEnvelopePicksMainModel(t *testing.T) {
	raw := `{"type":"result","result":"{\"ok\":true}","modelUsage":{
		"claude-haiku-4-5":{"inputTokens":523,"outputTokens":13},
		"claude-sonnet-5":{"inputTokens":1,"cacheReadInputTokens":3289,"cacheCreationInputTokens":5563}}}`
	got := parseClaudeEnvelope(raw)
	if got.Text != `{"ok":true}` {
		t.Fatalf("text = %q", got.Text)
	}
	if got.Model != "claude-sonnet-5" {
		t.Fatalf("model = %q, want claude-sonnet-5", got.Model)
	}
}

func TestParseClaudeEnvelopeFallsBackToRawText(t *testing.T) {
	raw := `plain text, not an envelope {"ok":true}`
	got := parseClaudeEnvelope(raw)
	if got.Text != raw || got.Model != "" {
		t.Fatalf("fallback = %#v", got)
	}
}

func TestCodexModelReadsPreamble(t *testing.T) {
	raw := "OpenAI Codex v0.143.0\n--------\nworkdir: /tmp\nmodel: gpt-5.6-sol\nprovider: openai\n--------\n{\"ok\":true}\n"
	if got := codexModel(raw); got != "gpt-5.6-sol" {
		t.Fatalf("model = %q", got)
	}
	if got := codexModel("no preamble at all"); got != "" {
		t.Fatalf("expected empty model, got %q", got)
	}
}

func TestCodexExecArgsExcludeRemovedFeatures(t *testing.T) {
	args := codexExecArgs("/tmp/judge")
	if slices.Contains(args, "browser_use_full_cdp_access") {
		t.Fatal("codex args include removed browser_use_full_cdp_access feature")
	}
}

func TestSupportedIncludesOpenCodeAndPi(t *testing.T) {
	if !slices.Equal(Supported, []string{"claude", "codex", "opencode", "pi"}) {
		t.Fatalf("Supported = %v", Supported)
	}
}

func TestOpenCodeRunArgsAreSealed(t *testing.T) {
	args := opencodeRunArgs("/tmp/judge")
	if slices.Contains(args, "--auto") || slices.Contains(args, "--dangerously-skip-permissions") {
		t.Fatalf("opencode args must not auto-approve tools: %v", args)
	}
	if !slices.Contains(args, "--pure") || !slices.Contains(args, "json") {
		t.Fatalf("opencode args missing sealed flags: %v", args)
	}
	if !slices.Contains(args, "/tmp/judge") {
		t.Fatalf("opencode args missing sealed workdir: %v", args)
	}
	env := strings.Join(opencodeSealedEnv(), "\n")
	if !strings.Contains(env, `OPENCODE_PERMISSION={"*":"deny"}`) {
		t.Fatalf("opencode env missing deny-all permissions: %s", env)
	}
}

func TestPiPrintArgsDisableTools(t *testing.T) {
	args := piPrintArgs()
	for _, want := range []string{"--print", "--no-tools", "--no-session", "--no-extensions", "--no-skills", "--no-context-files", "--no-approve"} {
		if !slices.Contains(args, want) {
			t.Fatalf("pi args missing %s: %v", want, args)
		}
	}
}

func TestParseOpenCodeJSONConcatenatesText(t *testing.T) {
	raw := `{"type":"step_start","part":{"modelID":"anthropic/claude-sonnet-4"}}
{"type":"text","part":{"text":"{\"ok\":true}"}}
not json
{"type":"step_finish"}
`
	got := parseOpenCodeJSON(raw)
	if got.Text != `{"ok":true}` {
		t.Fatalf("text = %q", got.Text)
	}
	if got.Model != "anthropic/claude-sonnet-4" {
		t.Fatalf("model = %q", got.Model)
	}
}

func TestParseOpenCodeJSONFallsBackToRawText(t *testing.T) {
	raw := "plain assistant reply"
	got := parseOpenCodeJSON(raw)
	if got.Text != raw || got.Model != "" {
		t.Fatalf("fallback = %#v", got)
	}
}

func TestTruncateRunesPreservesUTF8(t *testing.T) {
	detail := strings.Repeat("a", 499) + "界tail"
	got := truncateRunes(detail, 500)

	if !utf8.ValidString(got) {
		t.Fatalf("truncated detail is not valid UTF-8: %q", got)
	}
	want := strings.Repeat("a", 499) + "界"
	if got != want {
		t.Fatalf("truncated detail = %q, want %q", got, want)
	}
}

func TestTruncateRunesRepairsInvalidUTF8(t *testing.T) {
	got := truncateRunes(string([]byte{'o', 'k', 0xff}), 500)
	if !utf8.ValidString(got) {
		t.Fatalf("failure detail contains invalid UTF-8: %q", got)
	}
}
