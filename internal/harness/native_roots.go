package harness

import (
	"os"
	"path/filepath"
	"strings"
)

// VendorRulesRel are repo-relative rules directories, preferred order when multiple
// have user content. Covers Cursor, Claude, Agent Skills, Gemini, Codex,
// OpenCode, Copilot, and Pi.
var VendorRulesRel = []string{
	".cursor/rules",
	".claude/rules",
	".agents/rules",
	".gemini/rules",
	".codex/rules",
	".opencode/rules",
	".github/instructions",
	".pi/rules",
}

// VendorSkillsRel are repo-relative skills trees (Agent Skills layout:
// <dir>/<name>/SKILL.md), preferred order when multiple have non-/so skills.
var VendorSkillsRel = []string{
	".claude/skills",
	".cursor/skills",
	".agents/skills",
	".gemini/skills",
	".opencode/skills",
	".codex/skills",
	".github/skills",
	".pi/skills",
}

// discoverNativeRoots picks rules/skills dirs from what the repo already uses.
// Prefer the first vendor tree that already has user guidance; otherwise
// default to .agents/{rules,skills}.
func discoverNativeRoots(repoRoot string) (rulesDir, skillsDir string) {
	rulesDir = filepath.Join(repoRoot, ".agents", "rules")
	skillsDir = filepath.Join(repoRoot, ".agents", "skills")

	for _, rel := range VendorRulesRel {
		dir := filepath.Join(repoRoot, filepath.FromSlash(rel))
		if hasUserRules(dir) {
			rulesDir = dir
			break
		}
	}

	for _, rel := range VendorSkillsRel {
		dir := filepath.Join(repoRoot, filepath.FromSlash(rel))
		if hasUserSkills(dir) {
			skillsDir = dir
			break
		}
	}
	return rulesDir, skillsDir
}

// RulesCandidates returns absolute vendor rules dirs in preference order.
func RulesCandidates(repoRoot string) []string {
	out := make([]string, 0, len(VendorRulesRel))
	for _, rel := range VendorRulesRel {
		out = append(out, filepath.Join(repoRoot, filepath.FromSlash(rel)))
	}
	return out
}

// SkillsCandidates returns absolute vendor skills dirs in preference order.
func SkillsCandidates(repoRoot string) []string {
	out := make([]string, 0, len(VendorSkillsRel))
	for _, rel := range VendorSkillsRel {
		out = append(out, filepath.Join(repoRoot, filepath.FromSlash(rel)))
	}
	return out
}

// KindForRulesDir returns a short vendor label for a rules directory.
func KindForRulesDir(dir string) string {
	norm := "/" + strings.TrimPrefix(filepath.ToSlash(dir), "/")
	switch {
	case strings.Contains(norm, "/.cursor/rules"):
		return "cursor"
	case strings.Contains(norm, "/.claude/rules"):
		return "claude"
	case strings.Contains(norm, "/.agents/rules"):
		return "agents"
	case strings.Contains(norm, "/.gemini/rules"):
		return "gemini"
	case strings.Contains(norm, "/.codex/rules"):
		return "codex"
	case strings.Contains(norm, "/.opencode/rules"):
		return "opencode"
	case strings.Contains(norm, "/.github/instructions"):
		return "copilot"
	case strings.Contains(norm, "/.pi/rules"):
		return "pi"
	default:
		return "agents"
	}
}

// KindForSkillsDir returns a short vendor label for a skills directory.
func KindForSkillsDir(dir string) string {
	norm := "/" + strings.TrimPrefix(filepath.ToSlash(dir), "/")
	switch {
	case strings.Contains(norm, "/.claude/skills"):
		return "claude"
	case strings.Contains(norm, "/.cursor/skills"):
		return "cursor"
	case strings.Contains(norm, "/.agents/skills"):
		return "agents"
	case strings.Contains(norm, "/.gemini/skills"):
		return "gemini"
	case strings.Contains(norm, "/.opencode/skills"):
		return "opencode"
	case strings.Contains(norm, "/.codex/skills"):
		return "codex"
	case strings.Contains(norm, "/.github/skills"):
		return "copilot"
	case strings.Contains(norm, "/.pi/skills"):
		return "pi"
	default:
		return "agents"
	}
}

// RelFromRepo returns path relative to repoRoot when possible.
func RelFromRepo(repoRoot, abs string) string {
	rel, err := filepath.Rel(repoRoot, abs)
	if err != nil {
		return abs
	}
	return filepath.ToSlash(rel)
}

func hasUserRules(dir string) bool {
	found := false
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || found {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if isReservedInjectorRule(d.Name()) {
			return nil
		}
		if isRuleFilename(d.Name()) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func hasUserSkills(dir string) bool {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "so" || name == "superopen" {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, name, "SKILL.md")); err == nil {
			return true
		}
	}
	return false
}

func isReservedInjectorRule(name string) bool {
	return name == "superopen.mdc" || name == "superopen.md"
}

func isRuleFilename(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".mdc") ||
		strings.HasSuffix(lower, ".md") ||
		strings.HasSuffix(lower, ".instructions.md")
}

// ListAgentsFiles returns AGENTS.md paths (root first), for nested incremental docs.
func ListAgentsFiles(repoRoot string) []string {
	var out []string
	root := filepath.Join(repoRoot, "AGENTS.md")
	if _, err := os.Stat(root); err == nil {
		out = append(out, root)
	}
	skip := map[string]bool{
		".git": true, "node_modules": true, "vendor": true, "dist": true,
		".so": true, ".next": true, "coverage": true,
	}
	_ = filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skip[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "AGENTS.md" || path == root {
			return nil
		}
		out = append(out, path)
		return nil
	})
	return out
}
