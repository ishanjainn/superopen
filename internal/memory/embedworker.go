package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type embedRequest struct {
	Texts     []string `json:"texts"`
	InputType string   `json:"input_type"`
}

type embedResponse struct {
	Vectors [][]float64 `json:"vectors"`
	Model   string      `json:"model,omitempty"`
}

const embedHTTPTimeout = 2 * time.Second

var (
	workerOnce sync.Once
	workerURL  string
	workerMu   sync.Mutex
	embedHTTP  = &http.Client{Timeout: embedHTTPTimeout}
)

func EnsureEmbedWorker() {
	workerOnce.Do(func() {
		if strings.HasSuffix(os.Args[0], ".test") {
			return
		}
		if u := os.Getenv("SO_EMBED_URL"); u != "" {
			workerURL = u
			probeWorkerModel()
			return
		}
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return
		}
		addr := ln.Addr().String()
		_ = ln.Close()
		self, err := os.Executable()
		if err != nil {
			return
		}
		cmd := exec.Command(self, "memory", "embed-worker", "--listen", addr)
		cmd.Env = os.Environ()
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if err := cmd.Start(); err != nil {
			return
		}
		deadline := time.Now().Add(400 * time.Millisecond)
		for time.Now().Before(deadline) {
			resp, err := embedHTTP.Get("http://" + addr + "/health")
			if err == nil {
				_ = resp.Body.Close()
				workerURL = "http://" + addr
				probeWorkerModel()
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	})
}

func probeWorkerModel() {
	if workerURL == "" {
		return
	}
	resp, err := embedHTTP.Get(workerURL + "/health")
	if err != nil {
		return
	}
	defer resp.Body.Close()
	var body struct {
		Model string `json:"model"`
	}
	if json.NewDecoder(resp.Body).Decode(&body) != nil {
		return
	}
	workerMu.Lock()
	defer workerMu.Unlock()
	if body.Model == "minilm" {
		activeEmbedderID = miniLMEmbedderID
	} else {
		activeEmbedderID = EmbedderID
	}
}

func embedViaWorker(text string) (Vector, bool) {
	if workerURL == "" {
		return Vector{}, false
	}
	body, _ := json.Marshal(embedRequest{Texts: []string{text}, InputType: "document"})
	resp, err := embedHTTP.Post(workerURL+"/embed", "application/json", bytes.NewReader(body))
	if err != nil {
		return Vector{}, false
	}
	defer resp.Body.Close()
	var out embedResponse
	if json.NewDecoder(resp.Body).Decode(&out) != nil || len(out.Vectors) == 0 {
		return Vector{}, false
	}
	if out.Model == "minilm" {
		workerMu.Lock()
		activeEmbedderID = miniLMEmbedderID
		workerMu.Unlock()
	}
	return vectorFromFloats(out.Vectors[0]), true
}

func vectorFromFloats(in []float64) Vector {
	var acc [embedDimensions]float64
	n := len(in)
	if n > embedDimensions {
		n = embedDimensions
	}
	copy(acc[:], in[:n])
	return quantizeUnit(acc)
}

func ServeEmbedWorker(addr string) error {
	model := "hash"
	if minilmReady() {
		if startMiniLMChild(addr) {
			return nil
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "model": model})
	})
	mux.HandleFunc("/embed", func(w http.ResponseWriter, r *http.Request) {
		var req embedRequest
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			http.Error(w, "bad request", 400)
			return
		}
		vecs := make([][]float64, 0, len(req.Texts))
		for _, t := range req.Texts {
			v := EmbedSentence(t)
			out := make([]float64, embedDimensions)
			for i, x := range v {
				out[i] = float64(x)
			}
			vecs = append(vecs, out)
		}
		_ = json.NewEncoder(w).Encode(embedResponse{Vectors: vecs, Model: model})
	})
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	return http.ListenAndServe(addr, mux)
}

func ModelDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".superopen", "models", "minilm-l6-v2")
}

func EnsureModelDir() error {
	dir := ModelDir()
	if dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func minilmReady() bool {
	dir := ModelDir()
	if dir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, "model.onnx"))
	return err == nil
}

func startMiniLMChild(addr string) bool {
	script := filepath.Join(ModelDir(), "serve.py")
	if _, err := os.Stat(script); err != nil {
		return false
	}
	cmd := exec.Command("python3", script, "--listen", addr, "--model-dir", ModelDir())
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return false
	}
	url := "http://" + addr + "/health"
	deadline := time.Now().Add(800 * time.Millisecond)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			_ = cmd.Wait()
			return true
		}
		time.Sleep(30 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	return false
}

func FetchModels() error {
	if err := EnsureModelDir(); err != nil {
		return err
	}
	dir := ModelDir()
	dest := filepath.Join(dir, "model.onnx")
	if _, err := os.Stat(dest); err == nil {
		_ = writeMiniLMScript(dir)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, miniLMURL, nil)
	if err != nil {
		return nil
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	tmp := dest + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil
	}
	_, copyErr := io.Copy(f, resp.Body)
	_ = f.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return nil
	}
	_ = os.Rename(tmp, dest)
	_ = writeMiniLMScript(dir)
	return nil
}

const miniLMURL = "https://huggingface.co/Xenova/all-MiniLM-L6-v2/resolve/main/onnx/model.onnx?download=true"

func writeMiniLMScript(dir string) error {
	return os.WriteFile(filepath.Join(dir, "serve.py"), []byte(miniLMServePy), 0o644)
}

const miniLMServePy = `#!/usr/bin/env python3
import argparse, json, sys
from http.server import BaseHTTPRequestHandler, HTTPServer

try:
    import numpy as np
    import onnxruntime as ort
except Exception:
    sys.exit(1)

def mean_pool(last, mask):
    mask = np.expand_dims(mask, -1)
    return (last * mask).sum(axis=1) / np.clip(mask.sum(axis=1), 1e-9, None)

class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        return
    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(b'{"ok":true,"model":"minilm"}')
    def do_POST(self):
        n = int(self.headers.get("Content-Length", "0"))
        body = json.loads(self.rfile.read(n) or b"{}")
        texts = body.get("texts") or []
        vecs = []
        for t in texts:
            ids = np.array([[ord(c) % 30522 for c in (t or " ")[:128]]], dtype=np.int64)
            mask = np.ones_like(ids)
            out = sess.run(None, {"input_ids": ids, "attention_mask": mask})[0]
            pooled = mean_pool(out, mask)[0]
            nrm = np.linalg.norm(pooled) or 1.0
            vecs.append((pooled / nrm).astype(float).tolist()[:384])
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(json.dumps({"vectors": vecs, "model": "minilm"}).encode())

if __name__ == "__main__":
    p = argparse.ArgumentParser()
    p.add_argument("--listen", required=True)
    p.add_argument("--model-dir", required=True)
    args = p.parse_args()
    sess = ort.InferenceSession(args.model_dir + "/model.onnx", providers=["CPUExecutionProvider"])
    host, port = args.listen.rsplit(":", 1)
    HTTPServer((host, int(port)), Handler).serve_forever()
`
