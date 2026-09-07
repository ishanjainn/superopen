package main

import (
	"os"
	"path/filepath"

	"github.com/ishanjainn/superopen/internal/paths"
)

func writeInitGitignores(root string) error {
	layout := paths.Resolve(root)
	if err := os.WriteFile(filepath.Join(layout.Root, ".gitignore"), []byte(paths.GitignoreContents), 0o644); err != nil {
		return err
	}
	_, err := paths.EnsureRepoIgnore(root)
	return err
}
