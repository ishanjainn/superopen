package export

import (
	"context"
	"strings"

	"github.com/ishanjainn/superopen/internal/paths"
	"github.com/ishanjainn/superopen/internal/session"
	"github.com/ishanjainn/superopen/internal/session/trace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// localSessionExporter is the primary coding-hook exporter. It writes spans
// straight into the repository's unified session stream; no receiver or UI
// process is required for capture.
type localSessionExporter struct {
	traces   *trace.LocalJSONL
	sessions *session.Store
}

func newLocalSessionExporter(cwd string) sdktrace.SpanExporter {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return nil
	}
	root, err := paths.FindRoot(cwd)
	if err != nil {
		return nil
	}
	paths := paths.Resolve(root)
	if !paths.Exists() {
		return nil
	}
	return &localSessionExporter{
		traces:   trace.NewLocalJSONL(paths.SessionsDir),
		sessions: session.NewStore(paths),
	}
}

func (e *localSessionExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	if e == nil || len(spans) == 0 {
		return nil
	}
	rows := make([]trace.Span, 0, len(spans))
	for _, span := range spans {
		attrs := make(map[string]string)
		if res := span.Resource(); res != nil {
			for _, kv := range res.Attributes() {
				attrs[string(kv.Key)] = kv.Value.Emit()
			}
		}
		for _, kv := range span.Attributes() {
			attrs[string(kv.Key)] = kv.Value.Emit()
		}
		sid := localSessionID(attrs)
		rows = append(rows, trace.Span{
			TraceID: span.SpanContext().TraceID().String(), SpanID: span.SpanContext().SpanID().String(),
			ParentSpanID: span.Parent().SpanID().String(), Name: span.Name(),
			StartTimeUnixN: span.StartTime().UnixNano(), EndTimeUnixN: span.EndTime().UnixNano(),
			Attributes: attrs, Status: span.Status().Code.String(), SessionID: sid,
		})
	}
	if err := e.traces.Write(rows); err != nil {
		return err
	}
	e.sessions.UpsertActiveFromSpans(rows)
	return nil
}

func (e *localSessionExporter) Shutdown(context.Context) error { return nil }

func localSessionID(attrs map[string]string) string {
	for _, key := range []string{"gen_ai.conversation.id", "coding_agent.session.id", "coding_agent.session_id", "session.id", "session_id"} {
		if id := strings.TrimSpace(attrs[key]); id != "" && id != "unknown" {
			return id
		}
	}
	return ""
}
