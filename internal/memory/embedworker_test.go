package memory

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEmbedViaWorkerTimesOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	t.Cleanup(srv.Close)

	workerMu.Lock()
	old := workerURL
	workerURL = srv.URL
	workerMu.Unlock()
	t.Cleanup(func() {
		workerMu.Lock()
		workerURL = old
		workerMu.Unlock()
	})

	start := time.Now()
	_, ok := embedViaWorker("hello")
	if ok {
		t.Fatal("hung worker should not return a vector")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("embedViaWorker took %v, want <= %v", elapsed, embedHTTPTimeout)
	}
}
