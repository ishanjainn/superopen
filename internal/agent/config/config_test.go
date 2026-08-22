package config

import (
	"testing"
	"time"
)

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

func TestLoadRetentionHoursDefaultAndZero(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(EnvSessionRetentionHours, "")
	t.Setenv(EnvMemoryRetentionHours, "")
	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SessionRetentionHours != DefaultRetentionHours || cfg.MemoryRetentionHours != DefaultRetentionHours {
		t.Fatalf("defaults: sessions=%d memory=%d", cfg.SessionRetentionHours, cfg.MemoryRetentionHours)
	}

	t.Setenv(EnvSessionRetentionHours, "0")
	t.Setenv(EnvMemoryRetentionHours, "48")
	cfg, err = Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SessionRetentionHours != 0 || cfg.MemoryRetentionHours != 48 {
		t.Fatalf("parsed: sessions=%d memory=%d", cfg.SessionRetentionHours, cfg.MemoryRetentionHours)
	}
}

func TestHoursDuration(t *testing.T) {
	if HoursDuration(0) != 0 {
		t.Fatal("0 hours must mean keep forever")
	}
	if HoursDuration(168) != 168*time.Hour {
		t.Fatal("168 hours is 7 days")
	}
}
