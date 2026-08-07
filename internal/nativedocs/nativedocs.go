package nativedocs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/harness"
)

var (
	errInvalidPath  = errors.New("invalid path")
	errReservedRule = errors.New("reserved Superopen injector rule name")
)

const (
	LearnStart = "<!-- superopen:learned:start -->"
	LearnEnd   = "<!-- superopen:learned:end -->"
)

// WriteOpts controls where rules/skills mutations land.
// Policy: update-in-place across every existing vendor copy (keep in sync);
// if none exist, create under the session vendor tree (else preferred roots).
// AGENTS.md is always shared and ignores Vendor.
type WriteOpts struct {
	Vendor string // session meta.Vendor (claude-code, cursor, codex, …)
}

func (o WriteOpts) vendor() string { return strings.TrimSpace(o.Vendor) }


// EnsureAgentsMD creates root AGENTS.md from body if missing (or force).
func EnsureAgentsMD(paths harness.Paths, body string, force bool) error {
	return EnsureAgentsAt(paths.AgentsMD, body, force)
}

// EnsureAgentsAt creates AGENTS.md at an absolute path if missing (or force).
// Use for incremental nested docs (e.g. cmd/so/AGENTS.md).
func EnsureAgentsAt(absPath, body string, force bool) error {
	if !force {
		if _, err := os.Stat(absPath); err == nil {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(absPath, []byte(strings.TrimSpace(body)+"\n"), 0o644)
}

// AgentsFile resolves AGENTS.md for an optional repo-relative directory.
// Empty relDir → repo-root AGENTS.md. "internal/foo" → internal/foo/AGENTS.md.
func AgentsFile(repoRoot, relDir string) (string, error) {
	relDir = filepath.Clean(strings.TrimSpace(relDir))
	if relDir == "." || relDir == "" {
		return filepath.Join(repoRoot, "AGENTS.md"), nil
	}
	if filepath.IsAbs(relDir) || strings.Contains(relDir, "..") {
		return "", errInvalidPath
	}
	return filepath.Join(repoRoot, relDir, "AGENTS.md"), nil
}

// AppendLearned appends text into the learned section of root AGENTS.md.
func AppendLearned(paths harness.Paths, text string) error {
	return AppendLearnedAt(paths.AgentsMD, text)
}

// AppendLearnedAt appends into a specific AGENTS.md (root or nested).
func AppendLearnedAt(agentsPath, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	existing, _ := os.ReadFile(agentsPath)
	s := string(existing)
	if strings.Contains(s, text) {
		return nil // already present
	}
	block := "\n" + text + "\n"
	if strings.Contains(s, LearnStart) && strings.Contains(s, LearnEnd) {
		i := strings.Index(s, LearnEnd)
		s = s[:i] + block + s[i:]
	} else {
		if err := os.MkdirAll(filepath.Dir(agentsPath), 0o755); err != nil {
			return err
		}
		s = strings.TrimRight(s, "\n") + "\n\n" + LearnStart + "\n## Superopen learned\n" + block + LearnEnd + "\n"
	}
	return os.WriteFile(agentsPath, []byte(s), 0o644)
}

// RemoveLearnedContaining drops learned bullets/paragraphs that contain needle.
func RemoveLearnedContaining(agentsPath, needle string) error {
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return nil
	}
	data, err := os.ReadFile(agentsPath)
	if err != nil {
		return err
	}
	s := string(data)
	start := strings.Index(s, LearnStart)
	end := strings.Index(s, LearnEnd)
	if start < 0 || end <= start {
		return nil
	}
	inner := s[start+len(LearnStart) : end]
	var kept []string
	for _, line := range strings.Split(inner, "\n") {
		if strings.Contains(line, needle) {
			continue
		}
		kept = append(kept, line)
	}
	newInner := strings.Join(kept, "\n")
	s = s[:start+len(LearnStart)] + newInner + s[end:]
	return os.WriteFile(agentsPath, []byte(s), 0o644)
}

// UpsertRule writes or replaces a rule file. Identical content is a no-op.
// When the stem already exists in multiple vendor trees, all copies are synced.
func UpsertRule(paths harness.Paths, rel, body string, opts ...WriteOpts) error {
	opt := firstOpts(opts)
	body = strings.TrimSpace(body) + "\n"
	targets, err := ruleWriteTargets(paths, rel, opt)
	if err != nil {
		return err
	}
	for _, full := range targets {
		if prev, err := os.ReadFile(full); err == nil && string(prev) == body {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// AppendRule appends text to a rule file, skipping if the exact text already exists.
// Syncs across every existing vendor copy; creates under session vendor when none exist.
func AppendRule(paths harness.Paths, rel, text string, opts ...WriteOpts) error {
	opt := firstOpts(opts)
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	targets, err := ruleWriteTargets(paths, rel, opt)
	if err != nil {
		return err
	}
	for _, full := range targets {
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		existing, _ := os.ReadFile(full)
		if strings.Contains(string(existing), text) {
			continue
		}
		f, err := os.OpenFile(full, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		_, werr := f.WriteString("\n" + text + "\n")
		_ = f.Close()
		if werr != nil {
			return werr
		}
	}
	return nil
}

// RemoveRuleContaining removes lines from rule files that contain needle.
// Applies across every existing vendor copy of the stem.
func RemoveRuleContaining(paths harness.Paths, rel, needle string, opts ...WriteOpts) error {
	_ = opts // removal is always cross-vendor for existing copies
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return nil
	}
	existing := harness.FindExistingRules(paths.RepoRoot, rel)
	if len(existing) == 0 {
		full, err := RulePath(paths, rel)
		if err != nil {
			return err
		}
		existing = []string{full}
	}
	for _, full := range existing {
		data, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		var kept []string
		changed := false
		for _, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, needle) {
				changed = true
				continue
			}
			kept = append(kept, line)
		}
		if !changed {
			continue
		}
		if err := os.WriteFile(full, []byte(strings.Join(kept, "\n")), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// WriteSkillCreateOnly writes skills/<name>/SKILL.md if missing everywhere.
// If any vendor already has the skill, this is a no-op (use UpsertSkill to update + sync).
func WriteSkillCreateOnly(paths harness.Paths, name, body string, opts ...WriteOpts) error {
	opt := firstOpts(opts)
	name, err := normalizeSkillName(name)
	if err != nil {
		return err
	}
	if len(harness.FindExistingSkills(paths.RepoRoot, name)) > 0 {
		return nil
	}
	full := filepath.Join(skillCreateDir(paths, opt), name, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, []byte(strings.TrimSpace(body)+"\n"), 0o644)
}

// UpsertSkill replaces skills/<name>/SKILL.md across every vendor copy (sync).
// Creates under the session vendor tree when none exist.
func UpsertSkill(paths harness.Paths, name, body string, opts ...WriteOpts) error {
	opt := firstOpts(opts)
	name, err := normalizeSkillName(name)
	if err != nil {
		return err
	}
	body = strings.TrimSpace(body) + "\n"
	targets := harness.FindExistingSkills(paths.RepoRoot, name)
	if len(targets) == 0 {
		targets = []string{filepath.Join(skillCreateDir(paths, opt), name, "SKILL.md")}
	}
	for _, full := range targets {
		if prev, err := os.ReadFile(full); err == nil && string(prev) == body {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// RemoveSkill deletes a non-/so skill directory from every vendor tree.
func RemoveSkill(paths harness.Paths, name string, opts ...WriteOpts) error {
	_ = opts
	name, err := normalizeSkillName(name)
	if err != nil {
		return err
	}
	targets := harness.FindExistingSkills(paths.RepoRoot, name)
	if len(targets) == 0 {
		targets = []string{filepath.Join(paths.SkillsDir, name, "SKILL.md")}
	}
	for _, full := range targets {
		dir := filepath.Dir(full)
		base := filepath.Base(dir)
		if base == "so" || base == "superopen" {
			return fmt.Errorf("refusing to remove reserved skill %s", base)
		}
		_ = os.RemoveAll(dir)
	}
	return nil
}

func firstOpts(opts []WriteOpts) WriteOpts {
	if len(opts) == 0 {
		return WriteOpts{}
	}
	return opts[0]
}

func ruleWriteTargets(paths harness.Paths, rel string, opt WriteOpts) ([]string, error) {
	existing := harness.FindExistingRules(paths.RepoRoot, rel)
	if len(existing) > 0 {
		return existing, nil
	}
	dir := paths.RulesDir
	if opt.vendor() != "" {
		dir = harness.RulesDirForVendor(paths.RepoRoot, opt.vendor())
	}
	stem := harness.RuleStem(rel)
	if stem == "" || stem == "superopen" {
		return nil, errReservedRule
	}
	name := harness.DefaultRuleFilename(dir, stem)
	return []string{filepath.Join(dir, name)}, nil
}

func skillCreateDir(paths harness.Paths, opt WriteOpts) string {
	if opt.vendor() != "" {
		return harness.SkillsDirForVendor(paths.RepoRoot, opt.vendor())
	}
	return paths.SkillsDir
}

func normalizeSkillName(name string) (string, error) {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, ".md")
	name = strings.ReplaceAll(name, " ", "-")
	if name == "" || name == "so" || name == "superopen" || strings.Contains(name, "..") || strings.Contains(name, "/") {
		return "", fmt.Errorf("invalid skill name")
	}
	return name, nil
}

// DefaultAgentsBody builds initial AGENTS.md content from template fragments.
func DefaultAgentsBody(arch, conventions, decisions string) string {
	var b strings.Builder
	b.WriteString("# Agent instructions\n\n")
	b.WriteString("Prefer `so graph query` and this file before broad code search.\n\n")
	if strings.TrimSpace(arch) != "" {
		b.WriteString("## Architecture\n\n")
		b.WriteString(stripTitle(arch))
		b.WriteString("\n\n")
	}
	if strings.TrimSpace(conventions) != "" {
		b.WriteString("## Conventions\n\n")
		b.WriteString(stripTitle(conventions))
		b.WriteString("\n\n")
	}
	if strings.TrimSpace(decisions) != "" {
		b.WriteString("## Decisions\n\n")
		b.WriteString(stripTitle(decisions))
		b.WriteString("\n\n")
	}
	b.WriteString(LearnStart + "\n## Superopen learned\n\n" + LearnEnd + "\n")
	return b.String()
}

func stripTitle(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > 0 && strings.HasPrefix(lines[0], "#") {
		return strings.TrimSpace(strings.Join(lines[1:], "\n"))
	}
	return strings.TrimSpace(s)
}
