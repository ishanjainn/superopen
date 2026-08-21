package engine

import (
	"os"
	"path/filepath"
)

func readRepoFile(root, rel string) ([]byte, error) {
	if root == "" || rel == "" {
		return nil, os.ErrNotExist
	}
	return os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
}

func parsedSource(root string, parsed ParsedSyntaxFile) []byte {
	if len(parsed.Body) > 0 {
		return parsed.Body
	}
	body, err := readRepoFile(root, parsed.File.Path)
	if err != nil {
		return nil
	}
	return body
}
