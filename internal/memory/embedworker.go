package memory

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
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

// DefaultEmbedListen is the shared loopback the UI and CLI probe.
const DefaultEmbedListen = "127.0.0.1:18765"

const (
	healthRequestTimeout = 800 * time.Millisecond
	spawnReadyWait       = 15 * time.Second
)

var (
	workerOnce          sync.Once
	workerURL           string
	workerMu            sync.Mutex
	embedRequestTimeout = 30 * time.Second
	healthHTTP          = &http.Client{Timeout: healthRequestTimeout}
)

//go:embed bge_serve.py
var bgeServeFS embed.FS

func testingBinary() bool {
	return strings.HasSuffix(os.Args[0], ".test")
}

func workerConfigured() bool {
	workerMu.Lock()
	defer workerMu.Unlock()
	return workerURL != ""
}

func setWorkerURL(u string) {
	workerMu.Lock()
	workerURL = u
	workerMu.Unlock()
}

func currentWorkerURL() string {
	workerMu.Lock()
	defer workerMu.Unlock()
	return workerURL
}

func stampEmbedder(model string) {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "bge":
		activeEmbedderID = bgeEmbedderID
	case "minilm":
		activeEmbedderID = miniLMEmbedderID
	case "hash", "prose":
		activeEmbedderID = EmbedderID
	case "":
		return
	default:
		activeEmbedderID = "so-" + strings.ToLower(model) + "-384"
	}
}

func defaultEmbedURL() string {
	return "http://" + DefaultEmbedListen
}

// EnsureEmbedWorker points capture/recall at the shared loopback worker.
// Users do not set SO_EMBED_URL for the normal path; that env remains an override.
func EnsureEmbedWorker() {
	workerOnce.Do(func() {
		if testingBinary() {
			return
		}
		if u := strings.TrimSpace(os.Getenv("SO_EMBED_URL")); u != "" {
			setWorkerURL(u)
			probeWorkerModel()
			return
		}
		if attachDefaultWorker() {
			return
		}
		_ = FetchModels()
		_ = spawnDefaultWorker()
		attachDefaultWorker()
	})
}

func attachDefaultWorker() bool {
	url := defaultEmbedURL()
	if !healthOK(url) {
		return false
	}
	setWorkerURL(url)
	probeWorkerModel()
	return true
}

func spawnDefaultWorker() bool {
	if healthOK(defaultEmbedURL()) {
		return true
	}
	if waitLockedWorker() {
		return true
	}
	if !tryWorkerLock() {
		return waitLockedWorker() || healthOK(defaultEmbedURL())
	}
	defer releaseWorkerLock()
	self, err := os.Executable()
	if err != nil {
		return false
	}
	cmd := exec.Command(self, "memory", "embed-worker", "--listen", DefaultEmbedListen)
	cmd.Env = os.Environ()
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return false
	}
	_ = writeWorkerPID(cmd.Process.Pid)
	deadline := time.Now().Add(spawnReadyWait)
	for time.Now().Before(deadline) {
		if healthOK(defaultEmbedURL()) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return healthOK(defaultEmbedURL())
}

func workerLockPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".superopen", "embed-worker.lock")
}

func tryWorkerLock() bool {
	path := workerLockPath()
	if path == "" {
		return true
	}
	if err := os.Mkdir(path, 0o755); err == nil {
		return true
	}
	pid := readWorkerPID()
	if healthOK(defaultEmbedURL()) || (pid > 0 && pidAlive(pid)) {
		return false
	}
	_ = os.Remove(path)
	return os.Mkdir(path, 0o755) == nil
}

func releaseWorkerLock() {
	if path := workerLockPath(); path != "" {
		_ = os.Remove(path)
	}
}

func waitLockedWorker() bool {
	pid := readWorkerPID()
	if pid <= 0 || !pidAlive(pid) {
		return false
	}
	deadline := time.Now().Add(spawnReadyWait)
	for time.Now().Before(deadline) {
		if healthOK(defaultEmbedURL()) {
			return true
		}
		if !pidAlive(pid) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
	return healthOK(defaultEmbedURL())
}

func workerPIDPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".superopen", "embed-worker.pid")
}

func writeWorkerPID(pid int) error {
	path := workerPIDPath()
	if path == "" {
		return nil
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	return os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"), 0o644)
}

func readWorkerPID() int {
	path := workerPIDPath()
	if path == "" {
		return 0
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 0 {
		return 0
	}
	return pid
}

func pidAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

func healthOK(url string) bool {
	resp, err := healthHTTP.Get(url + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

func probeWorkerModel() {
	url := currentWorkerURL()
	if url == "" {
		return
	}
	resp, err := healthHTTP.Get(url + "/health")
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
	stampEmbedder(body.Model)
}

func embedViaWorker(text string) (Vector, bool) {
	return embedViaWorkerTyped(text, "document")
}

func embedQueryViaWorker(text string) (Vector, bool) {
	return embedViaWorkerTyped(text, "query")
}

func embedViaWorkerTyped(text, inputType string) (Vector, bool) {
	url := currentWorkerURL()
	if url == "" {
		return Vector{}, false
	}
	if inputType == "" {
		inputType = "document"
	}
	body, _ := json.Marshal(embedRequest{Texts: []string{text}, InputType: inputType})
	client := &http.Client{Timeout: embedRequestTimeout}
	resp, err := client.Post(url+"/embed", "application/json", bytes.NewReader(body))
	if err != nil {
		return Vector{}, false
	}
	defer resp.Body.Close()
	var out embedResponse
	if json.NewDecoder(resp.Body).Decode(&out) != nil || len(out.Vectors) == 0 {
		return Vector{}, false
	}
	if out.Model != "" {
		stampEmbedder(out.Model)
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
	if addr == "" {
		addr = DefaultEmbedListen
	}
	_ = FetchModels()
	if dir := bgeModelDir(); dir != "" && bgeReadyDir(dir) {
		if startBGEChild(addr, dir) {
			return nil
		}
	}
	model := "hash"
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
	return http.ListenAndServe(addr, mux)
}

func ModelDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".superopen", "models", "bge-small-en-v1.5-int8")
}

func EnsureModelDir() error {
	dir := ModelDir()
	if dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func legacyBGEDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".superopen", "models", "bge-small-en-v1.5")
}

func bgeModelDir() string {
	if dir := ModelDir(); bgeReadyDir(dir) {
		return dir
	}
	if dir := legacyBGEDir(); bgeReadyDir(dir) {
		return dir
	}
	return ModelDir()
}

func bgeReadyDir(dir string) bool {
	if dir == "" {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "tokenizer.json")); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "model.onnx")); err == nil {
		return true
	}
	_, err := os.Stat(filepath.Join(dir, "model_quantized.onnx"))
	return err == nil
}

func startBGEChild(addr, dir string) bool {
	script := filepath.Join(dir, "serve.py")
	if _, err := os.Stat(script); err != nil {
		if err := writeBGEScript(dir); err != nil {
			return false
		}
	}
	modelPath := filepath.Join(dir, "model.onnx")
	if _, err := os.Stat(modelPath); err != nil {
		quant := filepath.Join(dir, "model_quantized.onnx")
		if _, err := os.Stat(quant); err == nil {
			modelPath = quant
		}
	}
	cmd := exec.Command("python3", script, "--listen", addr, "--model-dir", dir)
	if filepath.Base(modelPath) != "model.onnx" {
		cmd.Env = append(os.Environ(), "SO_BGE_ONNX="+modelPath)
	}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return false
	}
	url := "http://" + addr
	deadline := time.Now().Add(spawnReadyWait)
	for time.Now().Before(deadline) {
		if healthOK(url) {
			_ = writeWorkerPID(cmd.Process.Pid)
			_ = cmd.Wait()
			return true
		}
		time.Sleep(40 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	return false
}

func FetchModels() error {
	if err := EnsureModelDir(); err != nil {
		return err
	}
	dir := ModelDir()
	if err := writeBGEScript(dir); err != nil {
		return err
	}
	tok := filepath.Join(dir, "tokenizer.json")
	mod := filepath.Join(dir, "model.onnx")
	if _, err := os.Stat(tok); err != nil {
		_ = downloadFile(bgeTokenizerURL, tok)
	}
	if _, err := os.Stat(mod); err != nil {
		_ = downloadFile(bgeQuantizedONNXURL, mod)
	}
	return nil
}

func writeBGEScript(dir string) error {
	raw, err := bgeServeFS.ReadFile("bge_serve.py")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "serve.py"), raw, 0o644)
}

func downloadFile(url, dest string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	tmp := dest + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, resp.Body)
	_ = f.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	return os.Rename(tmp, dest)
}

const (
	bgeTokenizerURL     = "https://huggingface.co/Xenova/bge-small-en-v1.5/resolve/main/tokenizer.json?download=true"
	bgeQuantizedONNXURL = "https://huggingface.co/Xenova/bge-small-en-v1.5/resolve/main/onnx/model_quantized.onnx?download=true"
)
