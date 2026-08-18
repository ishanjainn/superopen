package export

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ishanjainn/superopen/internal/agent/config"
	"github.com/ishanjainn/superopen/internal/agent/normalize"
	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/session"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestLocalSessionExporterMakesHookTraceVisibleWithoutReceiver(t *testing.T) {
	repo := t.TempDir()
	paths := paths.Resolve(repo)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	if err := session.NewStore(paths).Ensure(); err != nil {
		t.Fatal(err)
	}
	exporter := newLocalSessionExporter(repo)
	if exporter == nil {
		t.Fatal("expected local exporter for initialized repository")
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(resource.NewSchemaless(
			attribute.String("code.cwd", repo),
			attribute.String("coding_agent.client", "codex"),
		)),
		sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(exporter)),
	)
	_, span := tp.Tracer("test").Start(context.Background(), "coding_agent.user_prompt.submit")
	span.SetAttributes(
		attribute.String("coding_agent.session.id", "session-file-first"),
		attribute.String("gen_ai.request.model", "test-model"),
	)
	span.End()
	if err := tp.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	events := filepath.Join(paths.SessionDir("session-file-first"), "events.jsonl")
	if _, err := os.Stat(events); err != nil {
		t.Fatalf("events stream missing: %v", err)
	}
	items, err := session.NewStore(paths).ListDetailed()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "session-file-first" || items[0].Turns != 1 {
		t.Fatalf("visible sessions = %#v", items)
	}
}

func TestEmitterAlwaysPersistsLocally(t *testing.T) {
	repo := t.TempDir()
	paths := paths.Resolve(repo)
	if err := paths.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	if err := session.NewStore(paths).Ensure(); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Resolved{
		ApplicationName: "codex",
		Environment:     "test",
		Source:          map[string]string{},
	}
	emitter, err := NewEmitter(context.Background(), cfg, "codex", map[string]string{
		"code.cwd": repo, "coding_agent.session.id": "no-receiver-session",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := emitter.EmitEvent(normalize.EventEmission{
		SessionID: "no-receiver-session", Name: "coding_agent.user_prompt.submit",
	}); err != nil {
		t.Fatal(err)
	}
	if err := emitter.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(paths.SessionDir("no-receiver-session"), "events.jsonl")); err != nil {
		t.Fatalf("local event stream missing: %v", err)
	}
}
