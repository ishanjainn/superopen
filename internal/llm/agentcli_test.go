package llm

import (
	"testing"

	"github.com/ishanjainn/superopen/internal/config"
)

func TestAutoDoesNotUseAmbientAPIKeyWithoutProjectOptIn(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("SUPEROPEN_LLM_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", "")

	cfg := config.Default()
	if got := NewCompleterForBackend(cfg, "auto"); got != nil {
		t.Fatalf("auto selected unconfigured API backend: %T", got)
	}
	if got := NewCompleterForBackend(cfg, "llm_api"); got == nil {
		t.Fatal("explicit llm_api backend should use the ambient key")
	}
	cfg.LLM.Provider = "openai"
	if got := NewCompleterForBackend(cfg, "auto"); got == nil {
		t.Fatal("explicit llm config should enable API fallback")
	}
}
