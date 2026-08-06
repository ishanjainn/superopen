package harnessvalid

import (
	"fmt"
	"strings"
)

// Canonical projects.md H2 sections (order preserved).
var ProjectsSections = []string{"Current focus", "Active areas", "Do not touch", "Notes"}

// ValidatePreferences checks preferences.md shape.
func ValidatePreferences(md string) error {
	md = strings.TrimSpace(md)
	if md == "" {
		return fmt.Errorf("preferences.md is empty")
	}
	if !strings.HasPrefix(md, "# Preferences") {
		return fmt.Errorf("preferences.md must start with # Preferences")
	}
	// No second H1
	lines := strings.Split(md, "\n")
	h1 := 0
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "# ") && !strings.HasPrefix(strings.TrimSpace(line), "## ") {
			h1++
		}
	}
	if h1 > 1 {
		return fmt.Errorf("preferences.md must have exactly one H1")
	}
	return nil
}

// ValidateProjects checks projects.md required H1 + H2s.
func ValidateProjects(md string) error {
	md = strings.TrimSpace(md)
	if md == "" {
		return fmt.Errorf("projects.md is empty")
	}
	if !strings.HasPrefix(md, "# Projects") {
		return fmt.Errorf("projects.md must start with # Projects")
	}
	lower := md
	for _, sec := range ProjectsSections {
		needle := "## " + sec
		if !strings.Contains(lower, needle) {
			return fmt.Errorf("projects.md missing section %q", needle)
		}
	}
	return nil
}

// NormalizeProjects re-inserts missing H2 sections without wiping bullets.
func NormalizeProjects(md string) string {
	md = strings.TrimSpace(md)
	if md == "" {
		return defaultProjects()
	}
	if !strings.HasPrefix(md, "# Projects") {
		md = "# Projects\n\n" + md
	}
	existing := map[string]string{}
	order := []string{}
	var cur string
	var buf strings.Builder
	flush := func() {
		if cur == "" {
			return
		}
		existing[cur] = strings.TrimSpace(buf.String())
		buf.Reset()
	}
	for _, line := range strings.Split(md, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "## ") {
			flush()
			cur = strings.TrimPrefix(trim, "## ")
			order = append(order, cur)
			continue
		}
		if strings.HasPrefix(trim, "# Projects") {
			continue
		}
		if cur != "" {
			buf.WriteString(line)
			buf.WriteByte('\n')
		}
	}
	flush()

	var out strings.Builder
	out.WriteString("# Projects\n")
	seen := map[string]bool{}
	for _, sec := range ProjectsSections {
		out.WriteString("\n## ")
		out.WriteString(sec)
		out.WriteString("\n\n")
		if body, ok := existing[sec]; ok && body != "" {
			out.WriteString(body)
			if !strings.HasSuffix(body, "\n") {
				out.WriteByte('\n')
			}
		} else {
			out.WriteString("- (…)\n")
		}
		seen[sec] = true
	}
	// Preserve unknown H2s after Notes
	for _, sec := range order {
		if seen[sec] {
			continue
		}
		out.WriteString("\n## ")
		out.WriteString(sec)
		out.WriteString("\n\n")
		if body := existing[sec]; body != "" {
			out.WriteString(body)
			if !strings.HasSuffix(body, "\n") {
				out.WriteByte('\n')
			}
		}
	}
	return out.String()
}

// NormalizePreferences ensures H1 + intro.
func NormalizePreferences(md string) string {
	md = strings.TrimSpace(md)
	if md == "" {
		return defaultPreferences()
	}
	if !strings.HasPrefix(md, "# Preferences") {
		md = "# Preferences\n\nHow agents should work in this workspace.\n\n" + md
	}
	return md + "\n"
}

// AppendToProjectsSection appends a bullet under a named H2 (creates section if missing).
func AppendToProjectsSection(md, section, bullet string) string {
	bullet = strings.TrimSpace(bullet)
	if bullet == "" {
		return md
	}
	if !strings.HasPrefix(bullet, "- ") {
		bullet = "- " + bullet
	}
	md = NormalizeProjects(md)
	needle := "## " + section
	idx := strings.Index(md, needle)
	if idx < 0 {
		return md + "\n## " + section + "\n\n" + bullet + "\n"
	}
	rest := md[idx+len(needle):]
	next := strings.Index(rest, "\n## ")
	if next < 0 {
		body := strings.TrimRight(rest, "\n") + "\n" + bullet + "\n"
		return md[:idx+len(needle)] + body
	}
	body := strings.TrimRight(rest[:next], "\n") + "\n" + bullet + "\n"
	return md[:idx+len(needle)] + body + rest[next:]
}

// AppendPreferencesBullet appends a standing preference bullet.
func AppendPreferencesBullet(md, bullet string) string {
	bullet = strings.TrimSpace(bullet)
	if bullet == "" {
		return md
	}
	if !strings.HasPrefix(bullet, "- ") {
		bullet = "- " + bullet
	}
	md = strings.TrimRight(NormalizePreferences(md), "\n")
	return md + "\n" + bullet + "\n"
}

func defaultProjects() string {
	var b strings.Builder
	b.WriteString("# Projects\n")
	for _, sec := range ProjectsSections {
		b.WriteString("\n## ")
		b.WriteString(sec)
		b.WriteString("\n\n- (…)\n")
	}
	return b.String()
}

func defaultPreferences() string {
	return `# Preferences

How agents should work in this workspace.

- Prefer focused diffs; run tests for packages you change.
- Never commit secrets or log credentials.
`
}
