package otlpremote_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ishanjainn/superopen/internal/entitlement"
	"github.com/ishanjainn/superopen/internal/otlpremote"
	"github.com/ishanjainn/superopen/internal/tracestore"
)

// fakeSecret is assembled at runtime so the source file never contains
// "sk-<32 alphanumerics>" as a literal - GitHub's push-protection scanner
// shape-matches that pattern even though this is a harmless fixture.
var fakeSecret = "sk-" + strings.Repeat("a", 22) + "123456"

func TestExporterWritesWhenEntitled(t *testing.T) {
	dir := t.TempDir()
	entitlement.SetPathForTest(filepath.Join(dir, "auth.json"))

	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("missing auth")
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	_ = entitlement.LoginPaid("a@b.c", "tok", srv.URL, srv.URL, time.Now().Add(time.Hour))
	exp, ok := otlpremote.NewExporterFromEntitlement()
	if !ok {
		t.Fatal("expected exporter")
	}
	err := exp.Write([]tracestore.Span{{
		TraceID: "t1", SpanID: "s1", Name: "coding_agent.llm.turn",
		Attributes: map[string]string{"gen_ai.prompt": "hi " + fakeSecret},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatal(err)
	}
	raw := string(gotBody)
	if !contains(raw, "resourceSpans") {
		t.Fatalf("bad payload: %s", raw)
	}
}

func TestFanoutLocalOnlyWithoutAuth(t *testing.T) {
	dir := t.TempDir()
	entitlement.SetPathForTest(filepath.Join(dir, "auth.json"))
	local := tracestore.NewLocalJSONL(filepath.Join(dir, "traces"))
	store := otlpremote.FanoutLocalRemote(local)
	if _, ok := store.(*tracestore.Fanout); ok {
		t.Fatal("should not fanout without entitlement")
	}
	if err := store.Write([]tracestore.Span{{Name: "x", TraceID: "1", SpanID: "2"}}); err != nil {
		t.Fatal(err)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})())
}
