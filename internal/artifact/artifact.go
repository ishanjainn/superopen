// Package artifactmeta keeps every Superopen-owned artifact self-describing.
package artifact

import (
	"encoding/json"
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
