// Package otlpremote implements paid OTLP export + session query/pull.
package otlpremote

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/entitlement"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/redact"
	"github.com/ishanjainn/superopen/internal/session"
	"github.com/ishanjainn/superopen/internal/tracestore"
)

// Exporter sends spans to a remote OTLP HTTP endpoint when entitled.
type Exporter struct {
	Endpoint string
	Token    string
	Client   *http.Client
	MetaOnly bool // when true, strip prompt/completion bodies
}

// NewExporterFromEntitlement returns an exporter if paid cloud OTLP is enabled.
func NewExporterFromEntitlement() (*Exporter, bool) {
	st, err := entitlement.Load()
	if err != nil || !st.Authenticated || !st.Paid || st.OTLPEndpoint == "" {
		return nil, false
	}
	return &Exporter{
		Endpoint: strings.TrimRight(st.OTLPEndpoint, "/"),
		Token:    st.Token,
		Client:   &http.Client{Timeout: 15 * time.Second},
		MetaOnly: true,
	}, true
}

// Write implements tracestore.Store-like write of spans to remote OTLP/HTTP JSON.
func (e *Exporter) Write(spans []tracestore.Span) error {
	if e == nil || e.Endpoint == "" {
		return nil
	}
	payload := map[string]any{
		"resourceSpans": []map[string]any{
			{
				"scopeSpans": []map[string]any{
					{"spans": encodeSpans(spans, e.MetaOnly)},
				},
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint := e.Endpoint
	if !strings.HasSuffix(endpoint, "/v1/traces") {
		endpoint = endpoint + "/v1/traces"
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.Token != "" {
		req.Header.Set("Authorization", "Bearer "+e.Token)
	}
	client := e.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("otlp export: %s: %s", resp.Status, string(b))
	}
	return nil
}

func encodeSpans(spans []tracestore.Span, metaOnly bool) []map[string]any {
	out := make([]map[string]any, 0, len(spans))
	for _, sp := range spans {
		attrs := map[string]any{}
		for k, v := range sp.Attributes {
			if metaOnly && isBodyAttr(k) {
				continue
			}
			attrs[k] = redact.String(v)
		}
		out = append(out, map[string]any{
			"traceId":           sp.TraceID,
			"spanId":            sp.SpanID,
			"name":              sp.Name,
			"startTimeUnixNano": fmt.Sprintf("%d", sp.StartTimeUnixN),
			"endTimeUnixNano":   fmt.Sprintf("%d", sp.EndTimeUnixN),
			"attributes":        attrs,
		})
	}
	return out
}

func isBodyAttr(k string) bool {
	k = strings.ToLower(k)
	return strings.Contains(k, "prompt") || strings.Contains(k, "completion") ||
		strings.Contains(k, "messages") || strings.Contains(k, "thought") ||
		strings.Contains(k, "shell.command") || strings.Contains(k, "tool.arguments")
}

// RemoteBackend queries a paid session API (query endpoint).
type RemoteBackend struct {
	Endpoint string
	Token    string
	Client   *http.Client
}

func NewRemoteBackendFromEntitlement() (*RemoteBackend, bool) {
	st, err := entitlement.Load()
	if err != nil || !st.Authenticated || !st.Paid || st.QueryEndpoint == "" {
		return nil, false
	}
	return &RemoteBackend{
		Endpoint: strings.TrimRight(st.QueryEndpoint, "/"),
		Token:    st.Token,
		Client:   &http.Client{Timeout: 20 * time.Second},
	}, true
}

func (r *RemoteBackend) List(ctx context.Context, filter session.Filter) ([]session.ListItem, error) {
	u, err := url.Parse(r.Endpoint + "/api/sessions")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	if filter.Query != "" {
		q.Set("q", filter.Query)
	}
	if filter.ProjectID != "" {
		q.Set("project", filter.ProjectID)
	}
	if filter.Commit != "" {
		q.Set("commit", filter.Commit)
	}
	if filter.PR != "" {
		q.Set("pr", filter.PR)
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if r.Token != "" {
		req.Header.Set("Authorization", "Bearer "+r.Token)
	}
	client := r.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("remote list: %s", resp.Status)
	}
	var items []session.ListItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *RemoteBackend) Get(ctx context.Context, projectID, sessionID string) (session.Meta, error) {
	u := fmt.Sprintf("%s/api/sessions/%s", r.Endpoint, url.PathEscape(sessionID))
	if projectID != "" {
		u += "?project=" + url.QueryEscape(projectID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return session.Meta{}, err
	}
	if r.Token != "" {
		req.Header.Set("Authorization", "Bearer "+r.Token)
	}
	client := r.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return session.Meta{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return session.Meta{}, fmt.Errorf("remote get: %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return session.Meta{}, err
	}
	var wrap struct {
		Meta session.Meta `json:"meta"`
	}
	if err := json.Unmarshal(body, &wrap); err == nil && wrap.Meta.ID != "" {
		return wrap.Meta, nil
	}
	var m session.Meta
	if err := json.Unmarshal(body, &m); err != nil {
		return session.Meta{}, err
	}
	return m, nil
}

func (r *RemoteBackend) StoreFor(projectID string) (*session.Store, harness.Paths, error) {
	return nil, harness.Paths{}, fmt.Errorf("remote backend has no local store")
}

// FanoutLocalRemote writes to local JSONL and optionally remote when entitled.
func FanoutLocalRemote(local tracestore.Store) tracestore.Store {
	exp, ok := NewExporterFromEntitlement()
	if !ok {
		return local
	}
	return &tracestore.Fanout{Stores: []tracestore.Store{local, spanWriter{exp}}}
}

type spanWriter struct{ e *Exporter }

func (s spanWriter) Write(spans []tracestore.Span) error { return s.e.Write(spans) }
func (s spanWriter) Query(tracestore.QueryFilter) ([]tracestore.Span, error) {
	return nil, fmt.Errorf("remote exporter is write-only")
}
func (s spanWriter) SessionCost(string) (int64, float64, error) {
	return 0, 0, fmt.Errorf("remote exporter is write-only")
}
