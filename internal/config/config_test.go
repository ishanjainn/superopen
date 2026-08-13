package config_test

import (
	"os"
	"path/filepath"
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

func TestDefaultConfigDoesNotAdvertiseAPIProvider(t *testing.T) {
	cfg := config.Default()
	if cfg.Graph.Semantic {
		t.Fatal("fresh config must not enable network-backed semantic extraction")
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "semantic: false") {
		t.Fatalf("fresh config does not record the code-only default:\n%s", text)
	}
	if strings.Contains(text, "\nllm:") || strings.Contains(text, "provider: openai") || strings.Contains(text, "SUPEROPEN_LLM_API_KEY") {
		t.Fatalf("fresh config contains unused API configuration:\n%s", text)
	}
}

func TestLoadRequiresV2Layout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	legacy := "llm:\n  provider: openai\n"
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(path)
	if err == nil || !strings.Contains(err.Error(), "layout_version: 2") {
		t.Fatalf("expected unsupported-layout error, got %v", err)
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
