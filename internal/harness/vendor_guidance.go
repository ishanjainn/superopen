package harness

import (
	"os"
	"path/filepath"
	"strings"
)

// NormalizeVendorKind maps session meta.Vendor (coding_agent.client) to a
// guidance-tree kind used by VendorRulesRel / VendorSkillsRel.
func NormalizeVendorKind(vendor string) string {
	v := strings.ToLower(strings.TrimSpace(vendor))
	v = strings.ReplaceAll(v, "_", "-")
	switch v {
	case "claude-code", "claudecode", "claude", "cc":
		return "claude"
	case "cursor":
		return "cursor"
	case "codex":
		return "codex"
	case "gemini":
		return "gemini"
	case "opencode", "open-code":
		return "opencode"
	case "copilot-cli", "copilot", "github-copilot":
		return "copilot"
	case "pi":
		return "pi"
	case "agents", "agent-skills":
		return "agents"
	default:
		return ""
	}
}

// RulesRelForKind returns the repo-relative rules dir for a kind, or "".
func RulesRelForKind(kind string) string {
	switch kind {
	case "cursor":
		return ".cursor/rules"
	case "claude":
		return ".claude/rules"
	case "agents":
		return ".agents/rules"
	case "gemini":
		return ".gemini/rules"
	case "codex":
		return ".codex/rules"
	case "opencode":
		return ".opencode/rules"
	case "copilot":
		return ".github/instructions"
	case "pi":
		return ".pi/rules"
	default:
		return ""
	}
}

// SkillsRelForKind returns the repo-relative skills dir for a kind, or "".
func SkillsRelForKind(kind string) string {
	switch kind {
	case "claude":
		return ".claude/skills"
	case "cursor":
		return ".cursor/skills"
	case "agents":
		return ".agents/skills"
	case "gemini":
		return ".gemini/skills"
	case "opencode":
		return ".opencode/skills"
	case "codex":
		return ".codex/skills"
	case "copilot":
		return ".github/skills"
	case "pi":
		return ".pi/skills"
	default:
		return ""
	}
}

// RulesDirForVendor returns the absolute rules dir for a session vendor.
// Falls back to discoverNativeRoots when vendor is unknown.
func RulesDirForVendor(repoRoot, vendor string) string {
	if rel := RulesRelForKind(NormalizeVendorKind(vendor)); rel != "" {
		return filepath.Join(repoRoot, filepath.FromSlash(rel))
	}
	rules, _ := discoverNativeRoots(repoRoot)
	return rules
}

// SkillsDirForVendor returns the absolute skills dir for a session vendor.
func SkillsDirForVendor(repoRoot, vendor string) string {
	if rel := SkillsRelForKind(NormalizeVendorKind(vendor)); rel != "" {
		return filepath.Join(repoRoot, filepath.FromSlash(rel))
	}
	_, skills := discoverNativeRoots(repoRoot)
	return skills
}

// RuleStem is the logical rule id (filename without vendor-specific extension).
func RuleStem(name string) string {
	base := filepath.Base(strings.TrimSpace(name))
	lower := strings.ToLower(base)
	switch {
	case strings.HasSuffix(lower, ".instructions.md"):
		return base[:len(base)-len(".instructions.md")]
	case strings.HasSuffix(lower, ".mdc"):
		return base[:len(base)-len(".mdc")]
	case strings.HasSuffix(lower, ".md"):
		return base[:len(base)-len(".md")]
	default:
		return base
	}
}

// FindExistingRules returns absolute paths of rule files whose stem matches,
// across every vendor rules tree (excluding Superopen injectors).
func FindExistingRules(repoRoot, stemOrRel string) []string {
	stem := RuleStem(stemOrRel)
	if stem == "" || stem == "superopen" {
		return nil
	}
	var out []string
	for _, dir := range RulesCandidates(repoRoot) {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			name := d.Name()
			if isReservedInjectorRule(name) || !isRuleFilename(name) {
				return nil
			}
			if !strings.EqualFold(RuleStem(name), stem) {
				return nil
			}
			out = append(out, path)
			return nil
		})
	}
	return out
}

// FindExistingSkills returns absolute SKILL.md paths for a skill name across
// every vendor skills tree (skips so/superopen).
func FindExistingSkills(repoRoot, name string) []string {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, ".md")
	name = strings.ReplaceAll(name, " ", "-")
	if name == "" || name == "so" || name == "superopen" {
		return nil
	}
	var out []string
	for _, dir := range SkillsCandidates(repoRoot) {
		p := filepath.Join(dir, name, "SKILL.md")
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// DefaultRuleFilename returns the filename for a stem in a given rules dir.
func DefaultRuleFilename(rulesDir, stem string) string {
	stem = RuleStem(stem)
	switch KindForRulesDir(rulesDir) {
	case "cursor":
		return stem + ".mdc"
	case "copilot":
		return stem + ".instructions.md"
	default:
		return stem + ".md"
	}
}

// GuidanceKindFromPath returns the vendor kind for an indexed guidance path,
// or "shared" for AGENTS.md / unknown corpus docs.
func GuidanceKindFromPath(path string) string {
	norm := filepath.ToSlash(path)
	if filepath.Base(norm) == "AGENTS.md" || strings.HasSuffix(norm, "/AGENTS.md") {
		return "shared"
	}
	if strings.Contains(norm, "/rules/") || strings.Contains(norm, "/instructions/") {
		return KindForRulesDir(norm)
	}
	if strings.Contains(norm, "/skills/") {
		return KindForSkillsDir(norm)
	}
	return "shared"
}

// VendorWeight returns a retrieve score multiplier for a corpus path given the
// active session vendor. AGENTS.md / shared always stay high; matching vendor
// trees are boosted; other vendors are down-weighted but still searchable.
func VendorWeight(path, sessionVendor string) float64 {
	kind := NormalizeVendorKind(sessionVendor)
	if kind == "" {
		return 1
	}
	pk := GuidanceKindFromPath(path)
	switch pk {
	case "shared":
		return 1.25
	case "agents":
		return 1.0
	case kind:
		return 2.0
	default:
		return 0.55
	}
}
