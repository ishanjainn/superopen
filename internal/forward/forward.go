package forward

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ishanjainn/superopen/internal/paths"
)

// Config selects where new session bytes are copied. It lives in .so/forward/config.json.
type Config struct {
	File string `json:"file,omitempty"`
	URL  string `json:"url,omitempty"`
}

type cursorFile struct {
	Offsets map[string]int64 `json:"offsets"`
}

// Run copies new bytes from each events.jsonl to the configured file or URL.
// The read cursor is .so/forward/cursor.json, not a file inside a session directory.
func Run(root string) (int, error) {
	layout := paths.Resolve(root)
	cfg, err := readConfig(filepath.Join(layout.Root, "forward", "config.json"))
	if err != nil {
		return 0, err
	}
	if cfg.File == "" && cfg.URL == "" {
		return 0, fmt.Errorf("forward config needs file or url")
	}
	curPath := filepath.Join(layout.Root, "forward", "cursor.json")
	cur, err := readCursor(curPath)
	if err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(layout.SessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	n := 0
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		path := filepath.Join(layout.SessionsDir, ent.Name(), "events.jsonl")
		copied, next, err := copyNew(path, cur.Offsets[ent.Name()], cfg, client)
		if err != nil {
			return n, err
		}
		if cur.Offsets == nil {
			cur.Offsets = map[string]int64{}
		}
		cur.Offsets[ent.Name()] = next
		n += copied
	}
	if err := writeCursor(curPath, cur); err != nil {
		return n, err
	}
	return n, nil
}

func readConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("forward config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func readCursor(path string) (cursorFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cursorFile{Offsets: map[string]int64{}}, nil
		}
		return cursorFile{}, err
	}
	var cur cursorFile
	if err := json.Unmarshal(data, &cur); err != nil {
		return cursorFile{}, err
	}
	if cur.Offsets == nil {
		cur.Offsets = map[string]int64{}
	}
	return cur, nil
}

func writeCursor(path string, cur cursorFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cur, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func copyNew(path string, offset int64, cfg Config, client *http.Client) (int, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, offset, nil
		}
		return 0, offset, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return 0, offset, err
	}
	size := info.Size()
	if offset > size {
		offset = 0
	}
	if offset == size {
		return 0, size, nil
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return 0, offset, err
	}
	buf, err := io.ReadAll(f)
	if err != nil {
		return 0, offset, err
	}
	if cfg.File != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.File), 0o755); err != nil && filepath.Dir(cfg.File) != "." {
			return 0, offset, err
		}
		out, err := os.OpenFile(cfg.File, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return 0, offset, err
		}
		_, werr := out.Write(buf)
		cerr := out.Close()
		if werr != nil {
			return 0, offset, werr
		}
		if cerr != nil {
			return 0, offset, cerr
		}
	}
	if cfg.URL != "" {
		resp, err := client.Post(cfg.URL, "application/x-ndjson", bytes.NewReader(buf))
		if err != nil {
			return 0, offset, err
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			return 0, offset, fmt.Errorf("forward post: %s", resp.Status)
		}
	}
	lines := bytes.Count(buf, []byte("\n"))
	return lines, size, nil
}
