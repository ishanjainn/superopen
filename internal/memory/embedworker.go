package memory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
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
	embedWarnOnce       sync.Once
	embedFailMu         sync.Mutex
	embedFailWarned     bool
	workerURL           string
	workerMu            sync.Mutex
	lastEmbedErr        error
	embedRequestTimeout = 30 * time.Second
	healthHTTP          = &http.Client{Timeout: healthRequestTimeout}
)

//go:embed bge_serve.py
var bgeServeFS embed.FS

func testingBinary() bool {
	if testing.Testing() {
		return true
	}
	return isTestExecutable(os.Args[0])
}

// isTestExecutable reports go test binaries. Unix names end in ".test";
// Windows names end in ".test.exe". Sniffing only ".test" lets Windows
// tests re-exec so.test.exe as the embed worker and lock the file until
// `go test` tries to delete it.
func isTestExecutable(path string) bool {
	base := filepath.Base(strings.TrimSpace(path))
	if ext := filepath.Ext(base); strings.EqualFold(ext, ".exe") {
		base = strings.TrimSuffix(base, ext)
	}
	return strings.HasSuffix(base, ".test")
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

func warnEmbedFallback(format string, args ...any) {
	embedWarnOnce.Do(func() {
		fmt.Fprintf(os.Stderr, "so memory: "+format+"\n", args...)
	})
}

func setLastEmbedErr(err error) {
	if err != nil {
		lastEmbedErr = err
	}
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
		if err := FetchModels(); err != nil {
			setLastEmbedErr(err)
			warnEmbedFallback("BGE model fetch failed (%v); using hash embeddings until python3+onnxruntime is available", err)
		}
		_ = spawnDefaultWorker()
		if attachDefaultWorker() {
			return
		}
		detail := "install python3 with numpy and onnxruntime, or set SO_EMBED_URL"
		if lastEmbedErr != nil {
			detail = lastEmbedErr.Error()
		}
		warnEmbedFallback("BGE worker did not start (%s); using hash embeddings. Dense recall is weaker until the worker is healthy", detail)
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
	if err != nil || isTestExecutable(self) {
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
	v, ok, timedOut := postEmbedOnce(url, text, inputType)
	if ok {
		return v, true
	}
	if timedOut {
		warnEmbedRequestFail()
		return Vector{}, false
	}
	v, ok, _ = postEmbedOnce(url, text, inputType)
	if ok {
		return v, true
	}
	warnEmbedRequestFail()
	return Vector{}, false
}

func postEmbedOnce(url, text, inputType string) (Vector, bool, bool) {
	body, _ := json.Marshal(embedRequest{Texts: []string{text}, InputType: inputType})
	client := &http.Client{Timeout: embedRequestTimeout}
	resp, err := client.Post(url+"/embed", "application/json", bytes.NewReader(body))
	if err != nil {
		var ne net.Error
		timedOut := errors.As(err, &ne) && ne.Timeout()
		setLastEmbedErr(err)
		return Vector{}, false, timedOut
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		_, _ = io.Copy(io.Discard, resp.Body)
		setLastEmbedErr(fmt.Errorf("embed worker HTTP %d", resp.StatusCode))
		return Vector{}, false, false
	}
	var out embedResponse
	if json.NewDecoder(resp.Body).Decode(&out) != nil || len(out.Vectors) == 0 {
		setLastEmbedErr(fmt.Errorf("embed worker returned no vectors"))
		return Vector{}, false, false
	}
	if out.Model != "" {
		stampEmbedder(out.Model)
	}
	return vectorFromFloats(out.Vectors[0]), true, false
}

func warnEmbedRequestFail() {
	embedFailMu.Lock()
	defer embedFailMu.Unlock()
	if embedFailWarned {
		return
	}
	embedFailWarned = true
	detail := "ranking without dense matches until the worker is healthy"
	if lastEmbedErr != nil {
		detail = lastEmbedErr.Error() + "; " + detail
	}
	fmt.Fprintf(os.Stderr, "so memory: embed worker request failed (%s)\n", detail)
}

func resetEmbedFailWarnForTest() {
	embedFailMu.Lock()
	embedFailWarned = false
	embedFailMu.Unlock()
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
	if strings.TrimSpace(os.Getenv("SO_EMBED_ALLOW_HASH")) != "1" {
		return fmt.Errorf("bge embed worker unavailable at %s (set SO_EMBED_ALLOW_HASH=1 for hash fallback)", addr)
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
			setLastEmbedErr(err)
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
	cmd := pythonCommand(script, "--listen", addr, "--model-dir", dir)
	if cmd == nil {
		setLastEmbedErr(fmt.Errorf("python3, python, or py -3 not found on PATH"))
		return false
	}
	if filepath.Base(modelPath) != "model.onnx" {
		cmd.Env = append(os.Environ(), "SO_BGE_ONNX="+modelPath)
	}
	stderr := &capBuffer{max: 4096}
	cmd.Stdout = io.Discard
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		setLastEmbedErr(fmt.Errorf("start python embed worker: %w", err))
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
	if tail := strings.TrimSpace(string(stderr.buf)); tail != "" {
		setLastEmbedErr(fmt.Errorf("embed worker failed to become healthy: %s", tail))
	} else {
		setLastEmbedErr(fmt.Errorf("embed worker at %s did not pass /health within %s", addr, spawnReadyWait))
	}
	return false
}

// pythonCommand prefers python3 so existing machines resolve identically,
// then python, then the Windows py launcher.
func pythonCommand(script string, extra ...string) *exec.Cmd {
	candidates := [][]string{{"python3"}, {"python"}, {"py", "-3"}}
	for _, c := range candidates {
		if _, err := exec.LookPath(c[0]); err != nil {
			continue
		}
		args := append(append([]string{}, c[1:]...), script)
		args = append(args, extra...)
		return exec.Command(c[0], args...)
	}
	return nil
}

type capBuffer struct {
	buf []byte
	max int
}

func (c *capBuffer) Write(p []byte) (int, error) {
	if c.max <= 0 {
		return len(p), nil
	}
	remain := c.max - len(c.buf)
	if remain > 0 {
		if len(p) > remain {
			c.buf = append(c.buf, p[:remain]...)
		} else {
			c.buf = append(c.buf, p...)
		}
	}
	return len(p), nil
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
	if err := ensurePinnedFile(bgeTokenizerURL, tok, bgeTokenizerSHA256); err != nil {
		return err
	}
	if err := ensurePinnedFile(bgeQuantizedONNXURL, mod, bgeQuantizedONNXSHA256); err != nil {
		return err
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

func downloadFile(url, dest, wantSHA string) error {
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
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	tmp := dest + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(f, h), resp.Body)
	_ = f.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	got := hex.EncodeToString(h.Sum(nil))
	if wantSHA != "" && got != wantSHA {
		_ = os.Remove(tmp)
		return fmt.Errorf("checksum mismatch for %s (got %s)", filepath.Base(dest), got)
	}
	return os.Rename(tmp, dest)
}

func fileSHA256(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func ensurePinnedFile(url, dest, wantSHA string) error {
	if got, err := fileSHA256(dest); err == nil && got == wantSHA {
		return nil
	}
	return downloadFile(url, dest, wantSHA)
}

const (
	bgeTokenizerURL        = "https://huggingface.co/Xenova/bge-small-en-v1.5/resolve/main/tokenizer.json?download=true"
	bgeQuantizedONNXURL    = "https://huggingface.co/Xenova/bge-small-en-v1.5/resolve/main/onnx/model_quantized.onnx?download=true"
	bgeTokenizerSHA256     = "d241a60d5e8f04cc1b2b3e9ef7a4921b27bf526d9f6050ab90f9267a1f9e5c66"
	bgeQuantizedONNXSHA256 = "6c9c6101a956d62dfb5e7190c538226c0c5bb9cb27b651234b6df063ee7dbfe4"
)
