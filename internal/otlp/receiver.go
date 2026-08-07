package otlp

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/ishanjainn/superopen/internal/tracestore"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/protobuf/proto"
)

// Receiver is an OTLP HTTP receiver (JSON + protobuf) that fans spans into a TraceStore.
type Receiver struct {
	Store  tracestore.Store
	Addr   string // host:port, e.g. 127.0.0.1:4318
	// AfterWrite is invoked after spans are persisted (e.g. upsert live UI sessions).
	AfterWrite func(spans []tracestore.Span)
	server     *http.Server
}

func NewReceiver(listenURL string, store tracestore.Store) (*Receiver, error) {
	u, err := url.Parse(listenURL)
	if err != nil {
		return nil, err
	}
	host := u.Host
	if host == "" {
		host = "127.0.0.1:4318"
	}
	return &Receiver{Store: store, Addr: host}, nil
}

func (r *Receiver) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/traces", r.handleTraces)
	// Coding SDK also exports metrics; accept and ack so hook Shutdown succeeds.
	mux.HandleFunc("/v1/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	r.server = &http.Server{Addr: r.Addr, Handler: mux}
	ln, err := net.Listen("tcp", r.Addr)
	if err != nil {
		return err
	}
	go func() { _ = r.server.Serve(ln) }()
	return nil
}

func (r *Receiver) Stop() error {
	if r.server == nil {
		return nil
	}
	return r.server.Close()
}

// Simplified OTLP JSON shapes (subset).
type otlpExport struct {
	ResourceSpans []resourceSpans `json:"resourceSpans"`
}

type resourceSpans struct {
	Resource   resource     `json:"resource"`
	ScopeSpans []scopeSpans `json:"scopeSpans"`
}

type resource struct {
	Attributes []kv `json:"attributes"`
}

type scopeSpans struct {
	Spans []otlpSpan `json:"spans"`
}

type otlpSpan struct {
	TraceID           string `json:"traceId"`
	SpanID            string `json:"spanId"`
	ParentSpanID      string `json:"parentSpanId"`
	Name              string `json:"name"`
	StartTimeUnixNano string `json:"startTimeUnixNano"`
	EndTimeUnixNano   string `json:"endTimeUnixNano"`
	Attributes        []kv   `json:"attributes"`
	Status            *struct {
		Code string `json:"code"`
	} `json:"status"`
}

type kv struct {
	Key   string `json:"key"`
	Value struct {
		StringValue string  `json:"stringValue"`
		IntValue    string  `json:"intValue"`
		DoubleValue float64 `json:"doubleValue"`
		BoolValue   bool    `json:"boolValue"`
	} `json:"value"`
}

func (r *Receiver) handleTraces(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ct := strings.ToLower(req.Header.Get("Content-Type"))
	spans, err := parseOTLPBody(body, ct)
	if err != nil {
		http.Error(w, fmt.Sprintf("parse: %v", err), http.StatusBadRequest)
		return
	}
	if err := r.Store.Write(spans); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if r.AfterWrite != nil && len(spans) > 0 {
		r.AfterWrite(spans)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{}`))
}

func parseOTLPBody(body []byte, contentType string) ([]tracestore.Span, error) {
	wantProto := strings.Contains(contentType, "protobuf") ||
		strings.Contains(contentType, "application/x-protobuf") ||
		strings.Contains(contentType, "application/protobuf")

	if wantProto {
		if spans, err := parseOTLPProtobuf(body); err == nil {
			return spans, nil
		}
	}

	if spans, err := parseOTLPJSON(body); err == nil {
		return spans, nil
	} else {
		jsonErr := err
		// Try protobuf even without content-type (SDK default).
		if spans, err := parseOTLPProtobuf(body); err == nil {
			return spans, nil
		}
		var direct []tracestore.Span
		if json.Unmarshal(body, &direct) == nil && len(direct) > 0 {
			return direct, nil
		}
		return nil, jsonErr
	}
}

func parseOTLPProtobuf(body []byte) ([]tracestore.Span, error) {
	var req coltracepb.ExportTraceServiceRequest
	if err := proto.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	var out []tracestore.Span
	for _, rs := range req.ResourceSpans {
		resAttrs := map[string]string{}
		if rs.Resource != nil {
			resAttrs = protoAttrs(rs.Resource.Attributes)
		}
		for _, ss := range rs.ScopeSpans {
			for _, sp := range ss.Spans {
				attrs := protoAttrs(sp.Attributes)
				for k, v := range resAttrs {
					if _, ok := attrs[k]; !ok {
						attrs[k] = v
					}
				}
				traceID := hex.EncodeToString(sp.TraceId)
				sessionID := ResolveSessionID(attrs, traceID)
				status := ""
				if sp.Status != nil {
					status = sp.Status.Code.String()
				}
				out = append(out, tracestore.Span{
					TraceID:        traceID,
					SpanID:         hex.EncodeToString(sp.SpanId),
					ParentSpanID:   hex.EncodeToString(sp.ParentSpanId),
					Name:           sp.Name,
					StartTimeUnixN: int64(sp.StartTimeUnixNano),
					EndTimeUnixN:   int64(sp.EndTimeUnixNano),
					Attributes:     attrs,
					Status:         status,
					SessionID:      sessionID,
				})
			}
		}
	}
	return out, nil
}

func protoAttrs(attrs []*commonpb.KeyValue) map[string]string {
	m := map[string]string{}
	for _, a := range attrs {
		if a == nil || a.Key == "" || a.Value == nil {
			continue
		}
		m[a.Key] = anyValueString(a.Value)
	}
	return m
}

func anyValueString(v *commonpb.AnyValue) string {
	switch x := v.Value.(type) {
	case *commonpb.AnyValue_StringValue:
		return x.StringValue
	case *commonpb.AnyValue_IntValue:
		return fmt.Sprintf("%d", x.IntValue)
	case *commonpb.AnyValue_DoubleValue:
		return fmt.Sprintf("%g", x.DoubleValue)
	case *commonpb.AnyValue_BoolValue:
		return fmt.Sprintf("%t", x.BoolValue)
	case *commonpb.AnyValue_BytesValue:
		return hex.EncodeToString(x.BytesValue)
	default:
		return v.String()
	}
}

func parseOTLPJSON(body []byte) ([]tracestore.Span, error) {
	var exp otlpExport
	if err := json.Unmarshal(body, &exp); err != nil {
		return nil, err
	}
	var out []tracestore.Span
	for _, rs := range exp.ResourceSpans {
		resAttrs := kvMap(rs.Resource.Attributes)
		for _, ss := range rs.ScopeSpans {
			for _, sp := range ss.Spans {
				attrs := kvMap(sp.Attributes)
				for k, v := range resAttrs {
					if _, ok := attrs[k]; !ok {
						attrs[k] = v
					}
				}
				start, _ := parseNano(sp.StartTimeUnixNano)
				end, _ := parseNano(sp.EndTimeUnixNano)
				status := ""
				if sp.Status != nil {
					status = sp.Status.Code
				}
				sessionID := ResolveSessionID(attrs, sp.TraceID)
				out = append(out, tracestore.Span{
					TraceID:        sp.TraceID,
					SpanID:         sp.SpanID,
					ParentSpanID:   sp.ParentSpanID,
					Name:           sp.Name,
					StartTimeUnixN: start,
					EndTimeUnixN:   end,
					Attributes:     attrs,
					Status:         status,
					SessionID:      sessionID,
				})
			}
		}
	}
	return out, nil
}

func kvMap(attrs []kv) map[string]string {
	m := map[string]string{}
	for _, a := range attrs {
		switch {
		case a.Value.StringValue != "":
			m[a.Key] = a.Value.StringValue
		case a.Value.IntValue != "":
			m[a.Key] = a.Value.IntValue
		case a.Value.DoubleValue != 0:
			m[a.Key] = fmt.Sprintf("%g", a.Value.DoubleValue)
		case a.Value.BoolValue:
			m[a.Key] = "true"
		}
	}
	return m
}

func parseNano(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	var n int64
	_, err := fmt.Sscan(s, &n)
	return n, err
}


// ResolveSessionID picks the chat-thread id coding agents stamp on spans.
// Prefer gen_ai.conversation.id (stable for the life of a chat) over
// coding_agent.session.id (can be per-process / per-invocation on Cursor).
// Older paths used coding_agent.session_id.
func ResolveSessionID(attrs map[string]string, fallback string) string {
	for _, k := range []string{
		"gen_ai.conversation.id",
		"coding_agent.session.id",
		"coding_agent.session_id",
		"session.id",
		"session_id",
	} {
		if v := strings.TrimSpace(attrs[k]); v != "" && !IsPlaceholderSessionID(v) {
			return v
		}
	}
	if strings.TrimSpace(fallback) != "" && !IsPlaceholderSessionID(fallback) {
		return fallback
	}
	return ""
}

// IsPlaceholderSessionID reports ids that must never become Sessions UI rows
// (OpenCode/Pi hooks historically fell back to "unknown" when sessionID was missing).
func IsPlaceholderSessionID(id string) bool {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case "", "unknown", "null", "undefined", "nil", "none":
		return true
	default:
		return false
	}
}

// ResolveParentID returns the parent chat-thread id when this span belongs
// to a subagent / nested agent. Empty when missing or a self-parent echo.
func ResolveParentID(attrs map[string]string) string {
	if attrs == nil {
		return ""
	}
	parent := ""
	for _, k := range []string{
		"coding_agent.agent.parent_id",
		"coding_agent.parent_conversation.id",
		"gen_ai.conversation.parent_id",
	} {
		if v := attrs[k]; v != "" {
			parent = v
			break
		}
	}
	if parent == "" {
		return ""
	}
	self := ResolveSessionID(attrs, "")
	if parent == self {
		return ""
	}
	return parent
}

// IsSubagentAttrs reports whether span attributes mark a nested/subagent session.
func IsSubagentAttrs(attrs map[string]string) bool {
	if attrs == nil {
		return false
	}
	if v := attrs["coding_agent.session.is_subagent"]; v == "true" || v == "1" {
		return true
	}
	if v := attrs["coding_agent.agent.type"]; v == "subagent" {
		return true
	}
	return ResolveParentID(attrs) != ""
}
