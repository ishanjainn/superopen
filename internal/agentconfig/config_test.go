package agentconfig

import "testing"

func TestLoadIgnoresNetworkTelemetryEnvironment(t *testing.T) {
	t.Setenv("SUPEROPEN_OTLP_ENDPOINT", "https://example.invalid")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "https://example.invalid")
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "authorization=secret")
	t.Setenv("SUPEROPEN_API_KEY", "secret")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Source["otlp_endpoint"]; ok {
		t.Fatalf("network telemetry environment affected config: %#v", cfg.Source)
	}
}

func TestLoadIgnoresRemovedContentCaptureSetting(t *testing.T) {
	t.Setenv("SUPEROPEN_CODING_CONTENT_CAPTURE", "minimal")
	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Source["coding_content_capture"]; ok {
		t.Fatalf("removed capture setting affected config: %#v", cfg.Source)
	}
}
