package main

import (
	"os"
	"path/filepath"

	"github.com/ishanjainn/superopen/internal/paths"
)

func writeInitGitignores(root string) error {
	layout := paths.Resolve(root)
	ignorePath := filepath.Join(layout.Root, ".gitignore")
	if _, err := os.Stat(ignorePath); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.WriteFile(ignorePath, []byte(paths.GitignoreContents), 0o644); err != nil {
			return err
		}
	}
	_, err := paths.EnsureRepoIgnore(root)
	return err
}
