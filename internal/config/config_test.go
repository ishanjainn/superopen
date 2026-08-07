package config_test

import (
	"strings"
	"testing"

	"github.com/ishanjainn/superopen/internal/config"
)

func TestResolveLLMOpenRouter(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("SUPEROPEN_LLM_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", "")
	cfg := config.Default()
	r := cfg.ResolveLLM()
	if r.Provider != "openrouter" {
		t.Fatalf("provider=%s", r.Provider)
	}
	if r.BaseURL == "" {
		t.Fatal("expected openrouter base url")
	}
	if !cfg.HasLLM() {
		t.Fatal("expected HasLLM")
	}
}

func TestResolveLLMLocal(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("SUPEROPEN_LLM_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", "http://127.0.0.1:11434/v1")
	cfg := config.Default()
	r := cfg.ResolveLLM()
	if r.Provider != "local" {
		t.Fatalf("provider=%s", r.Provider)
	}
	if !cfg.HasLLM() {
		t.Fatal("expected HasLLM for local gateway")
	}
}

func TestLLMSetupGuide(t *testing.T) {
	g := config.LLMSetupGuide()
	for _, needle := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "OPENROUTER_API_KEY", "OPENAI_BASE_URL"} {
		if !strings.Contains(g, needle) {
			t.Fatalf("guide missing %s", needle)
		}
	}
}

func TestGuardrailsEnabledEnv(t *testing.T) {
	cfg := config.Default()
	t.Setenv("SUPEROPEN_GUARDRAILS", "off")
	if cfg.GuardrailsEnabled() {
		t.Fatal("expected SUPEROPEN_GUARDRAILS=off")
	}
	t.Setenv("SUPEROPEN_GUARDRAILS", "on")
	if !cfg.GuardrailsEnabled() {
		t.Fatal("expected SUPEROPEN_GUARDRAILS=on")
	}
}
