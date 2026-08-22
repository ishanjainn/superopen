package engine

import (
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

func dataLanguageFile(path string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(path))) {
	case ".json", ".json5", ".yaml", ".yml", ".toml", ".ini", ".hcl", ".properties":
		return true
	default:
		return false
	}
}

func skipDataLanguageVariable(node api.Node) bool {
	return node.Label == "Variable" && dataLanguageFile(node.Location.File)
}

func countBodyLines(body []byte) int {
	if len(body) == 0 {
		return 1
	}
	n := 0
	for _, b := range body {
		if b == '\n' {
			n++
		}
	}
	if body[len(body)-1] != '\n' {
		n++
	}
	if n < 1 {
		return 1
	}
	return n
}

func fileEndLine(parsed ParsedSyntaxFile) int {
	if parsed.File.LineCount > 0 {
		return parsed.File.LineCount
	}
	return countBodyLines(parsed.Body)
}
