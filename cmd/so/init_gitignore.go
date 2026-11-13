package main

import "github.com/ishanjainn/superopen/internal/paths"

func clearLegacyInitIgnores(root string) error {
	return paths.RemoveRepoIgnore(root)
}
