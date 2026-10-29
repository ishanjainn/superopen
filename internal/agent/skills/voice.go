package skills

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/paths"
)

//go:embed short/SKILL.md short/references
var voiceFS embed.FS

const voiceName = "short"

// InstallVoice writes the opt-in short-reply skill. Default install does not call this.
func InstallVoice(soBin string) ([]string, error) {
	if strings.TrimSpace(soBin) == "" {
		resolved, err := resolveSoBin()
		if err != nil || resolved == "" {
			resolved = "so"
		}
		soBin = resolved
	}
	files, err := renderedVoiceFiles(soBin)
	if err != nil {
		return nil, err
	}
	var written []string
	for _, dir := range skillDirsNamed(voiceName) {
		for _, file := range files {
			dst := filepath.Join(dir, file.relPath)
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return written, fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
			}
			if err := os.WriteFile(dst, []byte(file.body), 0o644); err != nil {
				return written, err
			}
			written = append(written, dst)
		}
	}
	return written, nil
}

// RemoveVoice deletes the opt-in skill when it is present.
func RemoveVoice() []string {
	var removed []string
	for _, dir := range skillDirsNamed(voiceName) {
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		if err := os.RemoveAll(dir); err == nil {
			removed = append(removed, dir)
		}
	}
	return removed
}

func renderedVoiceFiles(soBin string) ([]skillFile, error) {
	var out []skillFile
	err := fs.WalkDir(voiceFS, "short", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		body, err := voiceFS.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel("short", filepath.FromSlash(path))
		if err != nil {
			return err
		}
		out = append(out, skillFile{
			relPath: rel,
			body:    strings.ReplaceAll(string(body), soBinPlaceholder, paths.QuoteForHook(soBin)),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
