package config

import "testing"

func TestNormalizeModelSlug(t *testing.T) {
	if got := NormalizeModelSlug("codex", "luna"); got != "gpt-5.6-luna" {
		t.Fatalf("got %q", got)
	}
	if got := NormalizeModelSlug("claude", "sonnet-5"); got != "claude-sonnet-5" {
		t.Fatalf("got %q", got)
	}
}

func TestAutoApplyTiers(t *testing.T) {
	c := Default()
	if !c.AllowsAutoApplyTier("soft") {
		t.Fatal("default should allow soft")
	}
	if c.AllowsAutoApplyTier("policy") {
		t.Fatal("default should not allow policy")
	}
	c.Recommendations.RequireApproval = false
	if !c.AllowsAutoApplyTier("policy") {
		t.Fatal("require_approval false → all")
	}
}

func TestModelForCLI(t *testing.T) {
	c := Default()
	if c.ModelForCLI("claude") != "claude-sonnet-5" {
		t.Fatal(c.ModelForCLI("claude"))
	}
	if c.ModelForCLI("codex") != "gpt-5.6-luna" {
		t.Fatal(c.ModelForCLI("codex"))
	}
}

func TestNormalizeObservabilityLocalOnly(t *testing.T) {
	c := Default()
	c.Observability.Exporters = []ExporterConfig{
		{Type: "unsupported"},
		{Type: "local_jsonl", Path: ".so/custom-traces"},
	}
	c.normalizeObservability()
	if len(c.Observability.Exporters) != 1 {
		t.Fatalf("want 1 exporter, got %#v", c.Observability.Exporters)
	}
	if c.Observability.Exporters[0].Type != "local_jsonl" || c.Observability.Exporters[0].Path != ".so/sessions" {
		t.Fatalf("got %#v", c.Observability.Exporters[0])
	}
	c.Observability.Exporters = []ExporterConfig{{Type: "unsupported"}}
	c.normalizeObservability()
	if c.Observability.Exporters[0].Type != "local_jsonl" || c.Observability.Exporters[0].Path != ".so/sessions" {
		t.Fatalf("fallback got %#v", c.Observability.Exporters[0])
	}
}
