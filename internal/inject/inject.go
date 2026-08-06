package inject

import (
	"github.com/superopen/so/internal/config"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed skill.md
var embeddedSkillMD string

const (
	startMarker = "<!-- superopen:start -->"
	endMarker   = "<!-- superopen:end -->"
)

// InstallOptions controls where the /so skill is registered.
type InstallOptions struct {
	// ProjectRoot installs into the repo (./.agents/skills/so, .cursor, …). Empty = skip project.
	ProjectRoot string
	// Global installs into the user's home agent skill dirs (~/.agents/skills/so, …).
	Global bool
}

// InstallResult lists paths written.
type InstallResult struct {
	Paths []string
}

// Brief is the short pointer injected into agent instruction files (after init).
func Brief() string {
	lines := []string{
		"## Superopen",
		"",
		"This project is managed by Superopen (`.so/`). Prefer `.so/` before raw exploration to save tokens.",
		"",
		"Invoke with `/so` (Claude Code, Cursor, Gemini, Copilot, OpenCode, Pi) or `$so` (Codex):",
		"- `/so` - help",
		"- `/so init` - bootstrap Superopen if missing",
		"- `/so graph query \"<question>\"` - ask the repo knowledge graph",
		"- `/so graph` - rebuild `.so/graph/` (never leave `graphify-out/` at repo root)",
		"- `/so doctor` - health check",
		"",
		"Rules:",
		"- For codebase questions, run `so graph query \"<question>\"` when `.so/graph/graph.json` exists.",
		"- Read relevant files under `.so/knowledge/` and `.so/rules/`, plus matching skills in `.so/skills/` for the task.",
		"- Read `.so/memory/active-context.md` when present (session memory pack shared across coding agents).",
	}
	cfg := config.Default()
	if root := os.Getenv("SUPEROPEN_ROOT"); root != "" {
		if c, err := config.Load(filepath.Join(root, ".so", "config.yaml")); err == nil {
			cfg = c
		}
	} else if wd, err := os.Getwd(); err == nil {
		if c, err := config.Load(filepath.Join(wd, ".so", "config.yaml")); err == nil {
			cfg = c
		}
	}
	if cfg.InjectRulesEnabled() {
		lines = append(lines, "- Obey `.so/guardrails/guardrails.yaml`.")
	}
	lines = append(lines,
		"- Do not dump the entire `.so/` directory into context - load only what the task needs.",
		"- After meaningful Superopen edits by a human, they will run `so sync`.",
		"",
	)
	return strings.Join(lines, "\n")
}

// EnsureSkills registers the /so skill globally and in the current git project (if any).
// Idempotent: only writes missing files unless force is true.
func EnsureSkills(force bool) (InstallResult, error) {
	var out InstallResult
	home, err := os.UserHomeDir()
	if err == nil {
		need := force || !fileExists(filepath.Join(home, ".agents", "skills", "so", "SKILL.md")) ||
			!fileExists(filepath.Join(home, ".cursor", "skills", "so", "SKILL.md")) ||
			!fileExists(filepath.Join(home, ".gemini", "skills", "so", "SKILL.md")) ||
			!fileExists(filepath.Join(home, ".config", "opencode", "skills", "so", "SKILL.md")) ||
			!fileExists(filepath.Join(home, ".copilot", "skills", "so", "SKILL.md")) ||
			!fileExists(filepath.Join(home, ".pi", "agent", "skills", "so", "SKILL.md"))
		if need {
			paths, err := writeSkillBundle(home, skillMarkdown(""), true)
			if err != nil {
				return out, err
			}
			out.Paths = append(out.Paths, paths...)
		}
		// Always scrub legacy Cursor command duplicate when ensuring.
		_ = os.Remove(filepath.Join(home, ".cursor", "commands", "so.md"))
	}
	if root := findGitRoot(""); root != "" {
		need := force || !fileExists(filepath.Join(root, ".cursor", "skills", "so", "SKILL.md")) ||
			!fileExists(filepath.Join(root, ".agents", "skills", "so", "SKILL.md")) ||
			!fileExists(filepath.Join(root, ".gemini", "skills", "so", "SKILL.md")) ||
			!fileExists(filepath.Join(root, ".opencode", "skills", "so", "SKILL.md")) ||
			!fileExists(filepath.Join(root, ".github", "skills", "so", "SKILL.md")) ||
			!fileExists(filepath.Join(root, ".pi", "skills", "so", "SKILL.md"))
		if need {
			paths, err := writeSkillBundle(root, skillMarkdown(root), false)
			if err != nil {
				return out, err
			}
			out.Paths = append(out.Paths, paths...)
		}
		_ = os.Remove(filepath.Join(root, ".cursor", "commands", "so.md"))
	}
	return out, nil
}

// EnsureGlobalSkill installs the /so skill if missing (compat wrapper).
func EnsureGlobalSkill() (InstallResult, error) {
	return EnsureSkills(false)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func findGitRoot(start string) string {
	if start == "" {
		start, _ = os.Getwd()
	}
	dir := start
	for {
		if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil && (info.IsDir() || info.Mode().IsRegular()) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// GlobalSkillInstalled reports whether the user-global /so skill is present.
func GlobalSkillInstalled() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	return fileExists(filepath.Join(home, ".agents", "skills", "so", "SKILL.md"))
}

// InstallSkills registers the /so skill so coding agents can invoke it before (and after) init.
func InstallSkills(opts InstallOptions) (InstallResult, error) {
	skillBody := skillMarkdown("")
	var out InstallResult
	if opts.Global {
		home, err := os.UserHomeDir()
		if err != nil {
			return out, err
		}
		paths, err := writeSkillBundle(home, skillBody, true)
		if err != nil {
			return out, err
		}
		out.Paths = append(out.Paths, paths...)
	}
	if opts.ProjectRoot != "" {
		paths, err := writeSkillBundle(opts.ProjectRoot, skillBody, false)
		if err != nil {
			return out, err
		}
		out.Paths = append(out.Paths, paths...)
	}
	if len(out.Paths) == 0 {
		return out, fmt.Errorf("nothing to install - pass --global and/or --project")
	}
	return out, nil
}

// Apply writes project injectors (AGENTS.md, CLAUDE.md, Cursor rule) + project /so skill.
// Call from so init / so sync after the harness exists.
func Apply(repoRoot string) error {
	block := startMarker + "\n" + Brief() + endMarker + "\n"

	if err := upsertFile(filepath.Join(repoRoot, "AGENTS.md"), block); err != nil {
		return err
	}
	if err := upsertFile(filepath.Join(repoRoot, "CLAUDE.md"), block); err != nil {
		return err
	}

	cursorDir := filepath.Join(repoRoot, ".cursor", "rules")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		return err
	}
	mdc := "---\ndescription: Superopen always-on context. Prefer /so and .so/ before broad search.\nalwaysApply: true\n---\n\n" + Brief()
	if err := os.WriteFile(filepath.Join(cursorDir, "superopen.mdc"), []byte(mdc), 0o644); err != nil {
		return err
	}

	if _, err := InstallSkills(InstallOptions{ProjectRoot: repoRoot}); err != nil {
		return err
	}
	return installHarnessSkillsIndex(repoRoot)
}

func skillMarkdown(repoRoot string) string {
	if strings.TrimSpace(embeddedSkillMD) != "" {
		return embeddedSkillMD
	}
	return "---\nname: so\ndescription: Superopen\n---\n\n# /so\n\nRun `so init` then `so graph query`.\n"
}

func writeSkillBundle(root, skillBody string, globalHome bool) ([]string, error) {
	var written []string
	// Shared Agent Skills locations + vendor-native discovery paths so /so
	// (or equivalent skill load) works for every supported coding agent.
	skillTargets := []string{
		filepath.Join(root, ".agents", "skills", "so", "SKILL.md"),
		filepath.Join(root, ".claude", "skills", "so", "SKILL.md"),
		filepath.Join(root, ".cursor", "skills", "so", "SKILL.md"),
		filepath.Join(root, ".codex", "skills", "so", "SKILL.md"),
		filepath.Join(root, ".gemini", "skills", "so", "SKILL.md"),
		filepath.Join(root, ".opencode", "skills", "so", "SKILL.md"),
		filepath.Join(root, ".github", "skills", "so", "SKILL.md"), // Copilot CLI project skills
		filepath.Join(root, ".pi", "skills", "so", "SKILL.md"),
	}
	if globalHome {
		skillTargets = append(skillTargets,
			filepath.Join(root, ".config", "so", "SKILL.md"),
			filepath.Join(root, ".config", "opencode", "skills", "so", "SKILL.md"),
			filepath.Join(root, ".gemini", "skills", "so", "SKILL.md"),
			filepath.Join(root, ".copilot", "skills", "so", "SKILL.md"),
			filepath.Join(root, ".pi", "agent", "skills", "so", "SKILL.md"),
		)
	}
	seen := map[string]bool{}
	for _, path := range skillTargets {
		if seen[path] {
			continue
		}
		seen[path] = true
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return written, err
		}
		if err := os.WriteFile(path, []byte(skillBody), 0o644); err != nil {
			return written, err
		}
		written = append(written, path)
	}

	// Do NOT write .claude/commands/so.md or .cursor/commands/so.md.
	// Cursor surfaces both Agent Skills and Claude "commands" as /so, so a
	// companion command stub duplicates the skill entry in the slash menu.
	for _, legacy := range []string{
		filepath.Join(root, ".claude", "commands", "so.md"),
		filepath.Join(root, ".cursor", "commands", "so.md"),
	} {
		_ = os.Remove(legacy)
	}
	return written, nil
}

func installHarnessSkillsIndex(repoRoot string) error {
	skillsSrc := filepath.Join(repoRoot, ".so", "skills")
	agentsSkills := filepath.Join(repoRoot, ".agents", "skills", "superopen")
	if err := os.MkdirAll(agentsSkills, 0o755); err != nil {
		return err
	}
	skillMD := "# Superopen skills\n\nTask skills live in `.so/skills/`. For the `/so` slash skill see `.agents/skills/so/SKILL.md`.\n\n"
	if entries, err := os.ReadDir(skillsSrc); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				skillMD += fmt.Sprintf("- `%s`\n", e.Name())
			}
		}
	}
	return os.WriteFile(filepath.Join(agentsSkills, "SKILL.md"), []byte(skillMD), 0o644)
}

func upsertFile(path, block string) error {
	existing := ""
	if data, err := os.ReadFile(path); err == nil {
		existing = string(data)
	}
	updated := replaceOrAppend(existing, block)
	return os.WriteFile(path, []byte(updated), 0o644)
}

func replaceOrAppend(existing, block string) string {
	start := strings.Index(existing, startMarker)
	end := strings.Index(existing, endMarker)
	if start >= 0 && end > start {
		end += len(endMarker)
		return existing[:start] + block + existing[end:]
	}
	if strings.TrimSpace(existing) == "" {
		return block
	}
	if !strings.HasSuffix(existing, "\n") {
		existing += "\n"
	}
	return existing + "\n" + block
}

// Status checks injector presence in a project.
func Status(repoRoot string) map[string]bool {
	checks := map[string]string{
		"AGENTS.md":       filepath.Join(repoRoot, "AGENTS.md"),
		"CLAUDE.md":       filepath.Join(repoRoot, "CLAUDE.md"),
		"cursor-rule":     filepath.Join(repoRoot, ".cursor", "rules", "superopen.mdc"),
		"skill-agents":    filepath.Join(repoRoot, ".agents", "skills", "so", "SKILL.md"),
		"skill-claude":    filepath.Join(repoRoot, ".claude", "skills", "so", "SKILL.md"),
		"skill-cursor":    filepath.Join(repoRoot, ".cursor", "skills", "so", "SKILL.md"),
		"skill-codex":     filepath.Join(repoRoot, ".codex", "skills", "so", "SKILL.md"),
		"skill-gemini":    filepath.Join(repoRoot, ".gemini", "skills", "so", "SKILL.md"),
		"skill-opencode":  filepath.Join(repoRoot, ".opencode", "skills", "so", "SKILL.md"),
		"skill-copilot":   filepath.Join(repoRoot, ".github", "skills", "so", "SKILL.md"),
		"skill-pi":        filepath.Join(repoRoot, ".pi", "skills", "so", "SKILL.md"),
	}
	out := map[string]bool{}
	for k, p := range checks {
		data, err := os.ReadFile(p)
		if err != nil {
			out[k] = false
			continue
		}
		out[k] = strings.Contains(string(data), startMarker) ||
			strings.Contains(string(data), "Superopen") ||
			strings.Contains(string(data), "name: so") ||
			strings.Contains(string(data), "/so")
	}
	return out
}


// UninstallResult lists paths removed or scrubbed.
type UninstallResult struct {
	Removed []string
}

// Uninstall removes skills + project injectors that so install / so init wrote.
// projectRoot may be empty to skip project-scoped cleanup.
func Uninstall(projectRoot string) (UninstallResult, error) {
	var out UninstallResult
	home, err := os.UserHomeDir()
	if err == nil {
		out.Removed = append(out.Removed, removeSkillBundle(home, true)...)
	}
	if projectRoot != "" {
		out.Removed = append(out.Removed, removeSkillBundle(projectRoot, false)...)
		out.Removed = append(out.Removed, removeProjectInjectors(projectRoot)...)
	} else if root := findGitRoot(""); root != "" {
		out.Removed = append(out.Removed, removeSkillBundle(root, false)...)
		out.Removed = append(out.Removed, removeProjectInjectors(root)...)
	}
	return out, nil
}

func removeSkillBundle(root string, globalHome bool) []string {
	var removed []string
	dirs := []string{
		filepath.Join(root, ".agents", "skills", "so"),
		filepath.Join(root, ".claude", "skills", "so"),
		filepath.Join(root, ".cursor", "skills", "so"),
		filepath.Join(root, ".codex", "skills", "so"),
		filepath.Join(root, ".gemini", "skills", "so"),
		filepath.Join(root, ".opencode", "skills", "so"),
		filepath.Join(root, ".github", "skills", "so"),
		filepath.Join(root, ".pi", "skills", "so"),
		filepath.Join(root, ".agents", "skills", "superopen"),
	}
	files := []string{
		filepath.Join(root, ".claude", "commands", "so.md"),
		filepath.Join(root, ".cursor", "commands", "so.md"),
	}
	if globalHome {
		dirs = append(dirs,
			filepath.Join(root, ".config", "so"),
			filepath.Join(root, ".config", "opencode", "skills", "so"),
			filepath.Join(root, ".copilot", "skills", "so"),
			filepath.Join(root, ".pi", "agent", "skills", "so"),
		)
	}
	seen := map[string]bool{}
	for _, d := range dirs {
		if seen[d] {
			continue
		}
		seen[d] = true
		if _, err := os.Stat(d); err == nil {
			if err := os.RemoveAll(d); err == nil {
				removed = append(removed, d)
			}
		}
	}
	for _, f := range files {
		if err := os.Remove(f); err == nil {
			removed = append(removed, f)
		}
	}
	return removed
}

func removeProjectInjectors(repoRoot string) []string {
	var removed []string
	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		path := filepath.Join(repoRoot, name)
		if scrubbed, err := scrubInjectMarkers(path); err == nil && scrubbed {
			removed = append(removed, path+" (inject)")
		}
	}
	rule := filepath.Join(repoRoot, ".cursor", "rules", "superopen.mdc")
	if err := os.Remove(rule); err == nil {
		removed = append(removed, rule)
	}
	return removed
}

func scrubInjectMarkers(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	existing := string(data)
	start := strings.Index(existing, startMarker)
	end := strings.Index(existing, endMarker)
	if start < 0 || end < start {
		return false, nil
	}
	end += len(endMarker)
	updated := existing[:start] + existing[end:]
	updated = strings.TrimSpace(updated) + "\n"
	if updated == existing {
		return false, nil
	}
	return true, os.WriteFile(path, []byte(updated), 0o644)
}
