package engine

import (
	"encoding/gob"
	"os"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/graph/api"
)

type cachedExtract struct {
	File       ParsedSyntaxFile
	Coverage   *api.CoverageRow
	Generation string
}

func extractCacheDir(repoRoot string) string {
	paths, err := CachePaths(repoRoot)
	if err != nil {
		return ""
	}
	stamp := strings.ReplaceAll(fingerprintStamp(), ":", "-")
	return filepath.Join(paths.Root, "extract", stamp)
}

func extractCachePath(repoRoot, digest string) string {
	dir := extractCacheDir(repoRoot)
	if dir == "" || digest == "" {
		return ""
	}
	if len(digest) < 2 {
		return filepath.Join(dir, digest+".gob")
	}
	return filepath.Join(dir, digest[:2], digest+".gob")
}

func loadExtractCache(repoRoot, digest string) *cachedExtract {
	path := extractCachePath(repoRoot, digest)
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var got cachedExtract
	if err := gob.NewDecoder(f).Decode(&got); err != nil {
		return nil
	}
	got.File.Body = nil
	return &got
}

func saveExtractCache(repoRoot string, file ParsedSyntaxFile, coverage *api.CoverageRow, generation string) {
	digest := strings.TrimSpace(file.File.SHA256)
	path := extractCachePath(repoRoot, digest)
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	stored := file
	stored.Body = nil
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return
	}
	encErr := gob.NewEncoder(f).Encode(cachedExtract{File: stored, Coverage: coverage, Generation: generation})
	closeErr := f.Close()
	if encErr != nil || closeErr != nil {
		_ = os.Remove(tmp)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
	}
}

func restoreExtractBody(root string, file *ParsedSyntaxFile) {
	if file == nil || len(file.Body) > 0 {
		return
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file.File.Path)))
	if err != nil {
		return
	}
	file.Body = body
}

func dirtyParsePaths(changes api.ChangeSet) map[string]bool {
	out := make(map[string]bool, len(changes.Added)+len(changes.Modified)+len(changes.Renamed))
	for _, c := range changes.Added {
		out[filepath.ToSlash(c.Path)] = true
	}
	for _, c := range changes.Modified {
		out[filepath.ToSlash(c.Path)] = true
	}
	for _, c := range changes.Renamed {
		out[filepath.ToSlash(c.Path)] = true
	}
	return out
}

func dropAssemblePaths(changes api.ChangeSet) map[string]bool {
	out := make(map[string]bool, len(changes.Deleted)+len(changes.Renamed))
	for _, c := range changes.Deleted {
		out[filepath.ToSlash(c.Path)] = true
	}
	for _, c := range changes.Renamed {
		if old := filepath.ToSlash(c.OldPath); old != "" {
			out[old] = true
		}
	}
	return out
}

func fileDigestForCache(root, rel string, fp *fingerprintDoc) string {
	key := filepath.ToSlash(rel)
	if fp != nil {
		if entry, ok := fp.Files[key]; ok && entry.SHA256 != "" {
			abs := filepath.Join(root, filepath.FromSlash(rel))
			info, err := os.Stat(abs)
			if err == nil && entry.Size == info.Size() && entry.MtimeMs == mtimeMs(info) {
				return entry.SHA256
			}
		}
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return ""
	}
	return fileContentDigest(body)
}
