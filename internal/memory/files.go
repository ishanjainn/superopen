package memory

import (
	"path"
	"strings"
)

// NormalizePath slash-normalizes episode file paths so FTS and --file
// filters match Windows and POSIX spellings of the same location.
func NormalizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = strings.ReplaceAll(p, `\`, `/`)
	p = path.Clean(p)
	if p == "." {
		return ""
	}
	return p
}

func normalizeFiles(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, p := range in {
		p = NormalizePath(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}
