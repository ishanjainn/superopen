package nativedocs

import (
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/harness"
)

// DiscoverRoots reports which guidance dirs Resolve selected (for CLI/UI).
func DiscoverRoots(repoRoot string) Roots {
	p := harness.Resolve(repoRoot)
	return Roots{
		RulesDir:   p.RulesDir,
		SkillsDir:  p.SkillsDir,
		RulesKind:  harness.KindForRulesDir(p.RulesDir),
		SkillsKind: harness.KindForSkillsDir(p.SkillsDir),
	}
}

// Roots describes where this repo keeps agent guidance for writes.
type Roots struct {
	RulesDir   string
	SkillsDir  string
	RulesKind  string
	SkillsKind string
}

// RulePath returns the absolute path for a rule file under the preferred rules dir.
func RulePath(paths harness.Paths, rel string) (string, error) {
	rel = filepath.Clean(rel)
	if rel == "." || strings.Contains(rel, "..") {
		return "", errInvalidPath
	}
	base := filepath.Base(rel)
	if !strings.Contains(base, ".") {
		base += defaultRuleExt(paths.RulesDir)
		rel = filepath.Join(filepath.Dir(rel), base)
		if filepath.Dir(rel) == "." {
			rel = base
		}
	}
	if base == "superopen.mdc" || base == "superopen.md" {
		return "", errReservedRule
	}
	return filepath.Join(paths.RulesDir, rel), nil
}

func defaultRuleExt(rulesDir string) string {
	switch harness.KindForRulesDir(rulesDir) {
	case "cursor":
		return ".mdc"
	case "copilot":
		return ".instructions.md"
	default:
		return ".md"
	}
}
