package blocks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/compact"
	"github.com/ishanjainn/superopen/third_party/caveman/engine"
)

// HTTPJSON fetches a URL and returns the shrunk JSON body. The original stays
// in the local recovery store when the transform is smaller.
func HTTPJSON(root, rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("url must be http or https")
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(rawURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, compact.MaxInput+1))
	if err != nil {
		return "", err
	}
	if len(body) > compact.MaxInput {
		return "", compact.ErrTooLarge
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("http status %d", resp.StatusCode)
	}
	if !json.Valid(bytes.TrimSpace(body)) {
		return "", fmt.Errorf("response is not JSON")
	}
	out := compact.CompressAny(root, body, engine.TypeJSON)
	if out.PassedThrough {
		return string(body), nil
	}
	return "recover: so retrieve --raw " + out.Handle + "\ncontent:\n" + string(out.Output), nil
}

// FileContains reports whether path contains pattern. Used by wait-for.
func FileContains(path, pattern string) (bool, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return strings.Contains(string(body), pattern), nil
}
