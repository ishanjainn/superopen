package sdk

import (
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Config holds OTLP bootstrap settings for the coding-agent hook process.
type Config struct {
	// OtlpEndpoint is the OTLP/HTTP base URL (default http://127.0.0.1:4318).
	OtlpEndpoint string
	OtlpHeaders  map[string]string

	Environment     string
	ApplicationName string
	TracerName      string
	ServiceVersion  string

	DisableTracing bool
	DisableMetrics bool
	DisableBatch   bool

	TraceExporterTimeout  time.Duration
	MetricExporterTimeout time.Duration
	MetricExportInterval  time.Duration

	// DisableCaptureMessageContent: coding hooks set true and manage capture themselves.
	DisableCaptureMessageContent bool
	DetailedTracing              bool

	IDGenerator             sdktrace.IDGenerator
	Sampler                 sdktrace.Sampler
	ExtraResourceAttributes map[string]string
}

func (c *Config) setDefaults() {
	if c.OtlpEndpoint == "" {
		c.OtlpEndpoint = "http://127.0.0.1:4318"
	}
	if c.Environment == "" {
		c.Environment = "default"
	}
	if c.ApplicationName == "" {
		c.ApplicationName = "default"
	}
	if c.TracerName == "" {
		c.TracerName = "superopen"
	}
	if c.TraceExporterTimeout == 0 {
		c.TraceExporterTimeout = 10 * time.Second
	}
	if c.MetricExporterTimeout == 0 {
		c.MetricExporterTimeout = 10 * time.Second
	}
	if c.MetricExportInterval == 0 {
		c.MetricExportInterval = 30 * time.Second
	}
	if c.OtlpHeaders == nil {
		c.OtlpHeaders = make(map[string]string)
	}
}
