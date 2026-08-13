package memory

import (
	"os"
	"path/filepath"
	"strings"
)

// IsStubMarkdown reports empty or heading-only memory markdown (safe to replace once).
func IsStubMarkdown(content string) bool {
	c := strings.TrimSpace(content)
	if c == "" {
		return true
	}
	lines := 0
	nonHeading := 0
	for _, line := range strings.Split(c, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		lines++
		if strings.HasPrefix(t, "#") {
			continue
		}
		nonHeading++
	}
	if nonHeading == 0 {
		return true
	}
	// Very short stubs like "# preferences\n\n" after trim
	return len(c) < 40 && nonHeading == 0
}

// FindTemplatesRoot locates superopen/templates (same search pattern as init).
func FindTemplatesRoot() string {
	candidates := []string{
		"templates",
		filepath.Join("superopen", "templates"),
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append([]string{filepath.Join(filepath.Dir(exe), "templates")}, candidates...)
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "..", "templates"))
	}
	wd, _ := os.Getwd()
	candidates = append(candidates, filepath.Join(wd, "templates"))
	dir := wd
	for i := 0; i < 8; i++ {
		candidates = append(candidates, filepath.Join(dir, "superopen", "templates"), filepath.Join(dir, "templates"))
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	for _, c := range candidates {
		if info, err := os.Stat(filepath.Join(c, "memory", "preferences.md")); err == nil && !info.IsDir() {
			abs, _ := filepath.Abs(c)
			return abs
		}
		if info, err := os.Stat(filepath.Join(c, "knowledge")); err == nil && info.IsDir() {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	return ""
}

func defaultTemplate(name string) string {
	switch name {
	case "preferences.md":
		return `# Preferences

- Prefer focused diffs; run tests for packages you change.
- Never commit secrets or log credentials.
- Ask before force-push or making a repo public.
`
	case "projects.md":
		return `# Projects

## Current focus

- (What are you shipping this week?)

## Active areas

- (Packages under active change)

## Do not touch

- (Fragile or frozen areas)
`
	default:
		return "# " + strings.TrimSuffix(name, ".md") + "\n"
	}
}

// SeedFromTemplates initializes the consolidated state document. The legacy
// name is retained for callers; v2 never creates per-category memory files.
func (s *Store) SeedFromTemplates() error {
	return s.Ensure()
}
