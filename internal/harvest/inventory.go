package harvest

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var playbookNames = map[string]bool{
	"agents.md": true, "claude.md": true, "gemini.md": true, "copilot.md": true,
	"skill.md": true, "copilot-instructions.md": true,
}

var playbookDirs = []string{
	".agents/rules", ".cursor/rules", ".claude/rules", ".codex", ".windsurf",
	".github/instructions",
}

var skipDirs = map[string]bool{
	".git": true, ".so": true, "node_modules": true, "vendor": true, "dist": true,
	"build": true, ".next": true, "target": true, "__pycache__": true, ".venv": true,
}

func Inventory(root string) ([]File, error) {
	var out []File
	seen := map[string]bool{}
	add := func(abs string) {
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			return
		}
		rel = filepath.ToSlash(rel)
		if seen[rel] {
			return
		}
		seen[rel] = true
		info, err := os.Stat(abs)
		if err != nil || info.IsDir() {
			return
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return
		}
		sum := sha256.Sum256(data)
		out = append(out, File{
			Path:      rel,
			Hash:      hex.EncodeToString(sum[:]),
			Bytes:     len(data),
			Protected: ProtectedPath(rel) || ProtectedContent(string(data)),
		})
	}

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if skipDirs[name] {
				return filepath.SkipDir
			}
			return nil
		}
		low := strings.ToLower(name)
		if playbookNames[low] {
			add(path)
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		slash := filepath.ToSlash(rel)
		for _, dir := range playbookDirs {
			if strings.HasPrefix(slash, dir+"/") && (strings.HasSuffix(low, ".md") || strings.HasSuffix(low, ".mdc")) {
				add(path)
				return nil
			}
		}
		if strings.HasSuffix(low, ".md") && strings.Contains(slash, "/references/") {
			parent := filepath.Base(filepath.Dir(filepath.Dir(path)))
			if strings.EqualFold(parent, "skills") || fileExists(filepath.Join(filepath.Dir(filepath.Dir(path)), "SKILL.md")) {
				add(path)
			}
		}
		return nil
	})
	return out, nil
}

func shortHash(h string) string {
	if len(h) <= 12 {
		return h
	}
	return h[:12]
}

func HashFile(root, rel string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
