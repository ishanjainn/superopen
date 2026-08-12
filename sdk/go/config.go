package sdk

import (
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Config holds local telemetry settings for the coding-agent hook process.
type Config struct {
	Environment     string
	ApplicationName string
	TracerName      string
	ServiceVersion  string

	DisableTracing bool
	DisableBatch   bool
	// TraceExporters are local destinations supplied by the caller. The SDK
	// contains no network exporter.
	TraceExporters []sdktrace.SpanExporter

	// DisableCaptureMessageContent: coding hooks set true and manage capture themselves.
	DisableCaptureMessageContent bool
	DetailedTracing              bool

	IDGenerator             sdktrace.IDGenerator
	Sampler                 sdktrace.Sampler
	ExtraResourceAttributes map[string]string
}

func (c *Config) setDefaults() {
	if c.Environment == "" {
		c.Environment = "default"
	}
	if c.ApplicationName == "" {
		c.ApplicationName = "default"
	}
	if c.TracerName == "" {
		c.TracerName = "superopen"
	}
}
