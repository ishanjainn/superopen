// Package sdk provides OpenTelemetry bootstrap used by `so coding hook`
// (coding-agent session telemetry). It is not a general LLM provider SDK.
package sdk

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/ishanjainn/superopen/sdk/go/helpers"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

var (
	globalConfig   *Config
	globalShutdown func(context.Context) error
	initMutex      sync.Mutex
	isInitialized  bool
	tracerProvider *trace.TracerProvider
)

// Init initializes local tracing for the coding-agent hook process.
// Call once per process; pair with Shutdown before exit.
func Init(cfg Config) error {
	initMutex.Lock()
	defer initMutex.Unlock()

	if isInitialized {
		return fmt.Errorf("SDK is already initialized")
	}

	cfg.setDefaults()
	globalConfig = &cfg

	helpers.SetCaptureMessageContent(!cfg.DisableCaptureMessageContent)

	res, err := newResource(cfg)
	if err != nil {
		return fmt.Errorf("failed to create resource: %w", err)
	}

	if !cfg.DisableTracing {
		tp, err := newTracerProvider(res, cfg)
		if err != nil {
			return fmt.Errorf("failed to create tracer provider: %w", err)
		}
		tracerProvider = tp
		otel.SetTracerProvider(tp)
	}

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	globalShutdown = func(ctx context.Context) error {
		var first error
		if tracerProvider != nil {
			if err := tracerProvider.Shutdown(ctx); err != nil && first == nil {
				first = err
			}
		}
		return first
	}

	isInitialized = true
	return nil
}

// Shutdown flushes pending spans/metrics. Call before process exit.
func Shutdown(ctx context.Context) error {
	initMutex.Lock()
	defer initMutex.Unlock()

	if !isInitialized {
		return nil
	}

	if globalShutdown != nil {
		err := globalShutdown(ctx)
		if err != nil {
			return fmt.Errorf("failed to shutdown Superopen: %w", err)
		}
	}

	isInitialized = false
	globalConfig = nil
	tracerProvider = nil
	globalShutdown = nil

	log.Println("Superopen shut down successfully")
	return nil
}

// GetConfig returns the current global configuration.
// Returns nil if Superopen is not initialized.
func GetConfig() *Config {
	initMutex.Lock()
	defer initMutex.Unlock()
	return globalConfig
}

// IsInitialized returns whether Superopen has been initialized.
func IsInitialized() bool {
	initMutex.Lock()
	defer initMutex.Unlock()
	return isInitialized
}

// newResource creates a new resource with the application metadata
func newResource(cfg Config) (*resource.Resource, error) {
	attrs := []resource.Option{
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ApplicationName),
			semconv.DeploymentEnvironmentKey.String(cfg.Environment),
			attribute.String("superopen.sdk.version", Version),
		),
	}

	if cfg.ServiceVersion != "" {
		attrs = append(attrs, resource.WithAttributes(
			semconv.ServiceVersionKey.String(cfg.ServiceVersion),
		))
	}

	// Caller-supplied extras (e.g. `gen_ai.user.name`, `host.name`)
	// attach once at SDK init and then ride along on every span. Empty
	// keys/values are skipped so a missing field doesn't pollute the
	// resource map.
	if len(cfg.ExtraResourceAttributes) > 0 {
		extras := make([]attribute.KeyValue, 0, len(cfg.ExtraResourceAttributes))
		for k, v := range cfg.ExtraResourceAttributes {
			if k == "" || v == "" {
				continue
			}
			extras = append(extras, attribute.String(k, v))
		}
		if len(extras) > 0 {
			attrs = append(attrs, resource.WithAttributes(extras...))
		}
	}

	return resource.New(
		context.Background(),
		append(attrs, resource.WithTelemetrySDK())...,
	)
}

// newTracerProvider creates a tracer provider with caller-supplied local exporters.
func newTracerProvider(res *resource.Resource, cfg Config) (*trace.TracerProvider, error) {
	exporters := append([]trace.SpanExporter(nil), cfg.TraceExporters...)

	sampler := cfg.Sampler
	if sampler == nil {
		// Default beta posture: keep every span. Production
		// callers should set Config.Sampler to a head sampler
		// matching their volume budget. See D8 in the
		// coding-agents plan.
		sampler = trace.AlwaysSample()
	}
	tpOpts := []trace.TracerProviderOption{
		trace.WithResource(res),
		trace.WithSampler(sampler),
	}
	for _, exporter := range exporters {
		if exporter == nil {
			continue
		}
		if cfg.DisableBatch {
			tpOpts = append(tpOpts, trace.WithSpanProcessor(trace.NewSimpleSpanProcessor(exporter)))
		} else {
			tpOpts = append(tpOpts, trace.WithSpanProcessor(trace.NewBatchSpanProcessor(exporter)))
		}
	}
	if cfg.IDGenerator != nil {
		tpOpts = append(tpOpts, trace.WithIDGenerator(cfg.IDGenerator))
	}
	tp := trace.NewTracerProvider(tpOpts...)

	return tp, nil
}
