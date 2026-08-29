package memory

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func restoreEmbedder(t *testing.T) {
	t.Helper()
	old := activeEmbedderID
	t.Cleanup(func() { activeEmbedderID = old })
}

func restoreWorkerURL(t *testing.T) {
	t.Helper()
	workerMu.Lock()
	old := workerURL
	workerMu.Unlock()
	t.Cleanup(func() {
		workerMu.Lock()
		workerURL = old
		workerMu.Unlock()
	})
}

func unitVectorJSON(dim int) []float64 {
	out := make([]float64, dim)
	out[0] = 1
	return out
}

func TestStampEmbedderFromHealthBGE(t *testing.T) {
	restoreEmbedder(t)
	restoreWorkerURL(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "health") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"model":"bge"}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	setWorkerURL(srv.URL)
	probeWorkerModel()
	if got := CurrentEmbedder(); got != bgeEmbedderID {
		t.Fatalf("CurrentEmbedder=%s want %s", got, bgeEmbedderID)
	}
}

func TestStampEmbedderFromHealthHash(t *testing.T) {
	restoreEmbedder(t)
	restoreWorkerURL(t)
	activeEmbedderID = bgeEmbedderID
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"model":"hash"}`))
	}))
	t.Cleanup(srv.Close)
	setWorkerURL(srv.URL)
	probeWorkerModel()
	if got := CurrentEmbedder(); got != EmbedderID {
		t.Fatalf("CurrentEmbedder=%s want %s", got, EmbedderID)
	}
}

func TestEmbedViaWorkerSlowSucceeds(t *testing.T) {
	restoreEmbedder(t)
	restoreWorkerURL(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "embed") {
			time.Sleep(2500 * time.Millisecond)
			_ = json.NewEncoder(w).Encode(embedResponse{Vectors: [][]float64{unitVectorJSON(384)}, Model: "bge"})
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"model":"bge"}`))
	}))
	t.Cleanup(srv.Close)
	setWorkerURL(srv.URL)
	vec, ok := embedViaWorker("hello")
	if !ok || isZero(vec) {
		t.Fatal("slow embed within 30s should return a vector")
	}
	if CurrentEmbedder() != bgeEmbedderID {
		t.Fatalf("embed should stamp BGE, got %s", CurrentEmbedder())
	}
}

func TestEmbedViaWorkerTimesOut(t *testing.T) {
	restoreWorkerURL(t)
	oldTimeout := embedRequestTimeout
	embedRequestTimeout = 2 * time.Second
	t.Cleanup(func() { embedRequestTimeout = oldTimeout })

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	t.Cleanup(srv.Close)
	setWorkerURL(srv.URL)

	start := time.Now()
	_, ok := embedViaWorker("hello")
	if ok {
		t.Fatal("hung worker should not return a vector")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("embedViaWorker took %v, want <= %v", elapsed, embedRequestTimeout)
	}
}

func TestEmbedQueryRetriesThenSucceeds(t *testing.T) {
	restoreEmbedder(t)
	restoreWorkerURL(t)
	resetEmbedFailWarnForTest()
	t.Cleanup(resetEmbedFailWarnForTest)
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "embed") {
			_, _ = w.Write([]byte(`{"ok":true,"model":"bge"}`))
			return
		}
		n++
		if n == 1 {
			http.Error(w, "nope", http.StatusBadGateway)
			return
		}
		_ = json.NewEncoder(w).Encode(embedResponse{Vectors: [][]float64{unitVectorJSON(384)}, Model: "bge"})
	}))
	t.Cleanup(srv.Close)
	setWorkerURL(srv.URL)
	vec := EmbedQuery("how long is my commute")
	if n != 2 {
		t.Fatalf("want one retry, got %d requests", n)
	}
	if isZero(vec) {
		t.Fatal("retry must keep the dense channel")
	}
}

func TestEmbedQueryFailedWorkerDoesNotPanicAndWarnsOnce(t *testing.T) {
	restoreWorkerURL(t)
	resetEmbedFailWarnForTest()
	t.Cleanup(resetEmbedFailWarnForTest)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)
	setWorkerURL(srv.URL)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	vec := EmbedQuery("where did I buy the kettle")
	vec2 := EmbedQuery("where did I buy the kettle")
	_ = w.Close()
	os.Stderr = old
	logged, _ := io.ReadAll(r)
	_ = r.Close()
	if !isZero(vec) || !isZero(vec2) {
		t.Fatal("failed worker must not silently hash")
	}
	text := string(logged)
	if !strings.Contains(text, "embed worker request failed") {
		t.Fatalf("want stderr once, got %q", text)
	}
	if strings.Count(text, "embed worker request failed") != 1 {
		t.Fatalf("stderr must fire once, got %q", text)
	}
}

func TestEmbedTextNoSilentHashWhenWorkerFails(t *testing.T) {
	restoreWorkerURL(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)
	setWorkerURL(srv.URL)
	vec := EmbedText("a distinct diary sentence about login timeout in sqlite")
	if !isZero(vec) {
		t.Fatal("failed worker must not silently hash")
	}
}

func TestEmbedQueryHashFallbackUnprefixed(t *testing.T) {
	restoreWorkerURL(t)
	setWorkerURL("")
	q := "jwt expiry is 15m"
	got := EmbedQuery(q)
	want := EmbedSentence(q)
	if got != want {
		t.Fatal("hash fallback must not prefix the BGE query instruction")
	}
	prefixed := EmbedSentence("Represent this sentence for searching relevant passages: " + q)
	if Cosine(got, prefixed.Bytes()) > 0.99 {
		t.Fatal("query prefix should change hash features; test cannot assert the invariant")
	}
}

func TestEmbedQuerySendsInputTypeQuery(t *testing.T) {
	restoreWorkerURL(t)
	restoreEmbedder(t)
	var gotType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "embed") {
			var req embedRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			gotType = req.InputType
			_ = json.NewEncoder(w).Encode(embedResponse{Vectors: [][]float64{unitVectorJSON(384)}, Model: "bge"})
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"model":"bge"}`))
	}))
	t.Cleanup(srv.Close)
	setWorkerURL(srv.URL)
	if _, ok := embedQueryViaWorker("hello"); !ok {
		t.Fatal("query embed failed")
	}
	if gotType != "query" {
		t.Fatalf("input_type=%q want query", gotType)
	}
	gotType = ""
	if _, ok := embedViaWorker("hello"); !ok {
		t.Fatal("document embed failed")
	}
	if gotType != "document" {
		t.Fatalf("ingest input_type=%q want document", gotType)
	}
}

func TestPythonCommandPrefersPython3(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}
	cmd := pythonCommand("script.py", "--listen", "127.0.0.1:9")
	if cmd == nil {
		t.Fatal("expected a python command")
	}
	if filepath.Base(cmd.Args[0]) != "python3" {
		t.Fatalf("argv0=%q want python3 first so bench interpreter resolution stays identical", cmd.Args[0])
	}
}
