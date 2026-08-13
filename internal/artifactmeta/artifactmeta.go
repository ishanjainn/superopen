// Package artifactmeta keeps every Superopen-owned artifact self-describing.
package artifactmeta

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type About struct {
	Purpose   string `json:"purpose"`
	Authority string `json:"authority"`
	UpdatedBy string `json:"updated_by"`
}

type JSONLManifest struct {
	Type      string `json:"type"`
	Purpose   string `json:"purpose"`
	Authority string `json:"authority"`
	UpdatedBy string `json:"updated_by"`
}

func Object(about About, fields map[string]any) map[string]any {
	out := map[string]any{"_about": about}
	for k, v := range fields {
		out[k] = v
	}
	return out
}

func WriteJSON(path string, about About, fields map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(Object(about, fields), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func EnsureJSONL(path string, about About) error {
	if st, err := os.Stat(path); err == nil && st.Size() > 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(JSONLManifest{
		Type: "superopen.file_manifest", Purpose: about.Purpose,
		Authority: about.Authority, UpdatedBy: about.UpdatedBy,
	})
}

func PrependComment(path, comment string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append([]byte(comment+"\n"), body...), 0o644)
}

func Validate(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	if !s.Scan() {
		return fmt.Errorf("%s is empty", path)
	}
	first := bytes.TrimSpace(s.Bytes())
	if filepath.Base(path) == ".gitignore" {
		if !bytes.HasPrefix(first, []byte("#")) {
			return fmt.Errorf("%s has no leading purpose comment", path)
		}
		return nil
	}
	switch filepath.Ext(path) {
	case ".yaml", ".yml":
		if !bytes.HasPrefix(first, []byte("#")) {
			return fmt.Errorf("%s has no leading purpose comment", path)
		}
	case ".json":
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var obj map[string]json.RawMessage
		if json.Unmarshal(data, &obj) != nil || len(obj["_about"]) == 0 {
			return fmt.Errorf("%s has no _about metadata", path)
		}
	case ".jsonl":
		var m map[string]any
		if json.Unmarshal(first, &m) != nil || m["type"] != "superopen.file_manifest" {
			return fmt.Errorf("%s has no file manifest", path)
		}
	case ".md", ".html":
		if !bytes.HasPrefix(first, []byte("<!--")) {
			return fmt.Errorf("%s has no leading purpose comment", path)
		}
	}
	return nil
}
