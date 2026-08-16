package inject

import (
	"crypto/sha256"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/userpaths"
)

// SkillScopeDrift reports stale global skills that may shadow the current
// project copy. The reinstall command intentionally refreshes both scopes.
func SkillScopeDrift(repoRoot string) []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	want := sha256.Sum256([]byte(embeddedSkillMD))
	pairs := [][2]string{
		{filepath.Join(home, ".claude", "skills", "so", "SKILL.md"), filepath.Join(repoRoot, ".claude", "skills", "so", "SKILL.md")},
		{filepath.Join(home, ".cursor", "skills", "so", "SKILL.md"), filepath.Join(repoRoot, ".cursor", "skills", "so", "SKILL.md")},
		{filepath.Join(home, ".codex", "skills", "so", "SKILL.md"), filepath.Join(repoRoot, ".codex", "skills", "so", "SKILL.md")},
		{filepath.Join(home, ".gemini", "skills", "so", "SKILL.md"), filepath.Join(repoRoot, ".gemini", "skills", "so", "SKILL.md")},
		{filepath.Join(home, ".config", "opencode", "skills", "so", "SKILL.md"), filepath.Join(repoRoot, ".opencode", "skills", "so", "SKILL.md")},
		{filepath.Join(home, ".pi", "agent", "skills", "so", "SKILL.md"), filepath.Join(repoRoot, ".pi", "skills", "so", "SKILL.md")},
	}
	warnings := []string{}
	for _, pair := range pairs {
		globalBody, globalErr := os.ReadFile(pair[0])
		projectBody, projectErr := os.ReadFile(pair[1])
		if globalErr != nil || projectErr != nil {
			continue
		}
		globalHash, projectHash := sha256.Sum256(globalBody), sha256.Sum256(projectBody)
		if globalHash != want || globalHash != projectHash {
			warnings = append(warnings, pair[0]+" differs from the current project skill; run `so install --global --project`")
		}
	}
	return warnings
}

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
	Global       bool
	Vendors      []string
	SharedAgents bool
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
		"This project uses Superopen. Prefer AGENTS.md (including nested dir/AGENTS.md), existing vendor rules & skills dirs when present, and `so graph query` before raw exploration.",
		"",
		"Invoke with `/so` (Claude Code, Cursor, Gemini, Copilot, OpenCode, Pi) or `$so` (Codex):",
		"Chat syntax and shell syntax are different: `/so ...` invokes the chat skill; inside Bash or a terminal, always run `so ...` with no leading slash.",
		"- `/so` - help",
		"- `/so init` - bootstrap Superopen if missing",
		"- `/so graph query \"<question>\"` - ask the repo knowledge graph",
		"- `/so graph` - rebuild `.so/graph/` (local, regenerable)",
		"- `/so doctor` - health check",
		"",
		"Rules:",
		"- Never type `/so ...` into Bash; the leading slash is only for chat skill invocation.",
		"- For codebase questions, run `so graph query \"<question>\"` when `.so/graph/graph.json` exists.",
		"- Read `AGENTS.md` (and nested `*/AGENTS.md`), project rules, and matching skills for the task.",
		"- When updating guidance: edit existing rule/skill files in the dirs this repo already uses; prune obsolete lines instead of only appending.",
		"- Read `.so/memory/context.md` when present (generated session context shared across coding agents).",
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
		lines = append(lines, "- Obey `.so/guardrails.yaml`.")
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
	}
	return out, nil
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
		paths, err := writeSkillBundleFor(home, skillBody, opts.Vendors, opts.SharedAgents, true)
		if err != nil {
			return out, err
		}
		out.Paths = append(out.Paths, paths...)
	}
	if opts.ProjectRoot != "" {
		paths, err := writeSkillBundleFor(opts.ProjectRoot, skillBody, opts.Vendors, opts.SharedAgents, false)
		if err != nil {
			return out, err
		}
		out.Paths = append(out.Paths, paths...)
	}
	if len(out.Paths) == 0 {
		if opts.Global || opts.ProjectRoot != "" {
			return out, nil
		}
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
	cfg, _ := config.Load(filepath.Join(repoRoot, ".so", "config.yaml"))
	vendors := cfg.Vendors.Enabled
	if len(vendors) == 0 {
		vendors = DetectVendors(repoRoot)
	}
	for _, vendor := range vendors {
		switch strings.ToLower(vendor) {
		case "claude", "claude-code":
			if err := upsertFile(filepath.Join(repoRoot, "CLAUDE.md"), block); err != nil {
				return err
			}
		case "cursor":
			mdc := "---\ndescription: Superopen always-on context. Prefer AGENTS.md, Cursor rules/skills, and so graph query.\nalwaysApply: true\n---\n\n" + Brief()
			if err := writeFileIfChanged(filepath.Join(repoRoot, ".cursor", "rules", "superopen.mdc"), []byte(mdc)); err != nil {
				return err
			}
		}
	}
	if _, err := InstallSkills(InstallOptions{ProjectRoot: repoRoot, Vendors: vendors, SharedAgents: cfg.Vendors.SharedAgents}); err != nil {
		return err
	}
	if cfg.Vendors.SharedAgents {
		return installHarnessSkillsIndex(repoRoot)
	}
	return nil
}

func skillMarkdown(repoRoot string) string {
	if strings.TrimSpace(embeddedSkillMD) != "" {
		return embeddedSkillMD
	}
	return "---\nname: so\ndescription: Superopen\n---\n\n# /so\n\nRun `so init` then `so graph query`.\n"
}

func writeSkillBundle(root, skillBody string, globalHome bool) ([]string, error) {
	return writeSkillBundleFor(root, skillBody, DetectVendors(root), false, globalHome)
}

func writeSkillBundleFor(root, skillBody string, vendors []string, sharedAgents, globalHome bool) ([]string, error) {
	var written []string
	var skillTargets []string
	if sharedAgents {
		skillTargets = append(skillTargets, filepath.Join(root, ".agents", "skills", "so", "SKILL.md"))
	}
	for _, vendor := range vendors {
		if globalHome {
			if target := globalSkillTarget(strings.ToLower(strings.TrimSpace(vendor)), root); target != "" {
				skillTargets = append(skillTargets, target)
				continue
			}
		}
		var rel string
		switch strings.ToLower(strings.TrimSpace(vendor)) {
		case "claude", "claude-code":
			rel = filepath.Join(".claude", "skills")
		case "cursor":
			rel = filepath.Join(".cursor", "skills")
		case "codex":
			rel = filepath.Join(".codex", "skills")
		case "gemini":
			rel = filepath.Join(".gemini", "skills")
		case "opencode":
			if globalHome {
				rel = filepath.Join(".config", "opencode", "skills")
			} else {
				rel = filepath.Join(".opencode", "skills")
			}
		case "copilot", "copilot-cli":
			if globalHome {
				rel = filepath.Join(".copilot", "skills")
			} else {
				rel = filepath.Join(".github", "skills")
			}
		case "pi":
			if globalHome {
				rel = filepath.Join(".pi", "agent", "skills")
			} else {
				rel = filepath.Join(".pi", "skills")
			}
		case "agents":
			rel = filepath.Join(".agents", "skills")
		case "kilo":
			rel = filepath.Join(".kilo", "skills")
		case "aider":
			rel = filepath.Join(".aider")
		case "claw", "openclaw":
			rel = filepath.Join(".openclaw", "skills")
		case "droid", "factory":
			rel = filepath.Join(".factory", "skills")
		case "trae":
			rel = filepath.Join(".trae", "skills")
		case "trae-cn":
			rel = filepath.Join(".trae-cn", "skills")
		case "hermes":
			rel = filepath.Join(".hermes", "skills")
		case "kiro":
			rel = filepath.Join(".kiro", "skills")
		case "devin":
			rel = filepath.Join(".devin", "skills")
		case "codebuddy":
			rel = filepath.Join(".codebuddy", "skills")
		case "kimi":
			rel = filepath.Join(".kimi", "skills")
		case "amp", "antigravity", "vscode", "windows":
			rel = filepath.Join(".agents", "skills")
		}
		if rel != "" {
			skillTargets = append(skillTargets, filepath.Join(root, rel, "so", "SKILL.md"))
		}
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
		if err := writeFileIfChanged(path, []byte(skillBody)); err != nil {
			return written, err
		}
		written = append(written, path)
	}

	return written, nil
}

func globalSkillTarget(vendor, fallbackHome string) string {
	switch vendor {
	case "codex":
		if strings.TrimSpace(os.Getenv("CODEX_HOME")) != "" {
			if base, err := userpaths.CodexHome(); err == nil {
				return filepath.Join(base, "skills", "so", "SKILL.md")
			}
		}
		return filepath.Join(fallbackHome, ".codex", "skills", "so", "SKILL.md")
	case "opencode":
		if strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")) != "" {
			if base, err := userpaths.OpenCodeConfigDir(); err == nil {
				return filepath.Join(base, "skills", "so", "SKILL.md")
			}
		}
		return filepath.Join(fallbackHome, ".config", "opencode", "skills", "so", "SKILL.md")
	case "copilot", "copilot-cli":
		if strings.TrimSpace(os.Getenv("COPILOT_HOME")) != "" {
			if base, err := userpaths.CopilotHome(); err == nil {
				return filepath.Join(base, "skills", "so", "SKILL.md")
			}
		}
		return filepath.Join(fallbackHome, ".copilot", "skills", "so", "SKILL.md")
	case "kilo":
		return filepath.Join(fallbackHome, ".config", "kilo", "skills", "so", "SKILL.md")
	case "aider":
		return filepath.Join(fallbackHome, ".aider", "so", "SKILL.md")
	case "claw", "openclaw":
		return filepath.Join(fallbackHome, ".openclaw", "skills", "so", "SKILL.md")
	case "droid", "factory":
		return filepath.Join(fallbackHome, ".factory", "skills", "so", "SKILL.md")
	case "trae":
		return filepath.Join(fallbackHome, ".trae", "skills", "so", "SKILL.md")
	case "trae-cn":
		return filepath.Join(fallbackHome, ".trae-cn", "skills", "so", "SKILL.md")
	case "hermes":
		return filepath.Join(fallbackHome, ".hermes", "skills", "so", "SKILL.md")
	case "kiro":
		return filepath.Join(fallbackHome, ".kiro", "skills", "so", "SKILL.md")
	case "devin":
		return filepath.Join(fallbackHome, ".config", "devin", "skills", "so", "SKILL.md")
	case "codebuddy":
		return filepath.Join(fallbackHome, ".codebuddy", "skills", "so", "SKILL.md")
	case "kimi":
		return filepath.Join(fallbackHome, ".kimi", "skills", "so", "SKILL.md")
	case "amp", "antigravity", "vscode", "windows", "agents":
		return filepath.Join(fallbackHome, ".agents", "skills", "so", "SKILL.md")
	}
	return ""
}

// DetectVendors returns only coding agents that are visibly installed or have
// an existing native project directory. Shared .agents is never auto-detected.
func DetectVendors(root string) []string {
	type probe struct{ name, bin, dir string }
	probes := []probe{
		{"claude-code", "claude", ".claude"}, {"cursor", "cursor", ".cursor"}, {"codex", "codex", ".codex"}, {"gemini", "gemini", ".gemini"},
		{"opencode", "opencode", ".opencode"}, {"copilot-cli", "copilot", filepath.Join(".github", "skills")}, {"pi", "pi", ".pi"},
		{"kilo", "kilo", ".kilo"}, {"aider", "aider", ".aider"}, {"claw", "openclaw", ".openclaw"}, {"droid", "droid", ".factory"},
		{"trae", "trae", ".trae"}, {"trae-cn", "trae", ".trae-cn"}, {"hermes", "hermes", ".hermes"}, {"kiro", "kiro", ".kiro"},
		{"devin", "devin", ".devin"}, {"codebuddy", "codebuddy", ".codebuddy"}, {"kimi", "kimi", ".kimi"}, {"amp", "amp", ".agents"},
	}
	var out []string
	for _, p := range probes {
		_, binErr := exec.LookPath(p.bin)
		_, dirErr := os.Stat(filepath.Join(root, p.dir))
		installed := binErr == nil || dirErr == nil
		// GUI installs do not always put their CLI on the terminal PATH. Native
		// app/config locations are therefore also reliable detection signals.
		if !installed && (p.name == "codex" || p.name == "cursor") {
			installed = vendorHomeOrAppExists(p.name)
		}
		if installed {
			out = append(out, p.name)
		}
	}
	return out
}

func vendorHomeOrAppExists(vendor string) bool {
	home, _ := os.UserHomeDir()
	paths := vendorInstallCandidates(vendor, runtime.GOOS, home, os.Getenv)
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

func vendorInstallCandidates(vendor, goos, home string, getenv func(string) string) []string {
	var paths []string
	if home != "" {
		switch vendor {
		case "codex":
			if configured := strings.TrimSpace(getenv("CODEX_HOME")); configured != "" {
				paths = append(paths, configured)
			}
			paths = append(paths, filepath.Join(home, ".codex"))
		case "cursor":
			paths = append(paths, filepath.Join(home, ".cursor"))
		}
	}
	switch goos {
	case "darwin":
		app := "Codex.app"
		if vendor == "cursor" {
			app = "Cursor.app"
		}
		paths = append(paths, filepath.Join("/Applications", app))
		if home != "" {
			paths = append(paths, filepath.Join(home, "Applications", app))
			if vendor == "cursor" {
				paths = append(paths, filepath.Join(home, "Library", "Application Support", "Cursor"))
			}
		}
	case "windows":
		local, roaming := getenv("LOCALAPPDATA"), getenv("APPDATA")
		if vendor == "cursor" {
			paths = append(paths,
				filepath.Join(local, "Programs", "cursor", "Cursor.exe"),
				filepath.Join(local, "Cursor", "Cursor.exe"),
				filepath.Join(roaming, "Cursor"),
			)
		} else if vendor == "codex" {
			paths = append(paths,
				filepath.Join(local, "Programs", "Codex", "Codex.exe"),
				filepath.Join(local, "Codex", "Codex.exe"),
				filepath.Join(roaming, "Codex"),
			)
		}
	default: // Linux and other Unix desktops.
		if vendor == "cursor" {
			paths = append(paths, "/opt/Cursor/cursor", "/usr/share/applications/cursor.desktop")
			if home != "" {
				paths = append(paths,
					filepath.Join(home, ".config", "Cursor"),
					filepath.Join(home, ".local", "share", "applications", "cursor.desktop"),
				)
			}
		}
	}
	var clean []string
	for _, path := range paths {
		if strings.TrimSpace(path) != "" && path != "." {
			clean = append(clean, path)
		}
	}
	return clean
}

func installHarnessSkillsIndex(repoRoot string) error {
	skillsSrc := filepath.Join(repoRoot, ".agents", "skills")
	agentsSkills := filepath.Join(repoRoot, ".agents", "skills", "superopen")
	if err := os.MkdirAll(agentsSkills, 0o755); err != nil {
		return err
	}
	skillMD := "# Superopen skills\n\nTask skills live in `.agents/skills/<name>/SKILL.md`. For the `/so` slash skill see `.agents/skills/so/SKILL.md`.\n\n"
	if entries, err := os.ReadDir(skillsSrc); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if name == "so" || name == "superopen" {
				continue
			}
			if _, err := os.Stat(filepath.Join(skillsSrc, name, "SKILL.md")); err == nil {
				skillMD += fmt.Sprintf("- `%s`\n", name)
			}
		}
	}
	return writeFileIfChanged(filepath.Join(agentsSkills, "SKILL.md"), []byte(skillMD))
}

func upsertFile(path, block string) error {
	existing := ""
	if data, err := os.ReadFile(path); err == nil {
		existing = string(data)
	}
	updated := replaceOrAppend(existing, block)
	if updated == existing {
		return nil
	}
	return os.WriteFile(path, []byte(updated), 0o644)
}

func writeFileIfChanged(path string, body []byte) error {
	if prev, err := os.ReadFile(path); err == nil && string(prev) == string(body) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func replaceOrAppend(existing, block string) string {
	block = strings.TrimRight(block, "\n") + "\n"
	start := strings.Index(existing, startMarker)
	end := strings.Index(existing, endMarker)
	if start >= 0 && end > start {
		end += len(endMarker)
		// Drop all trailing blank lines after the marker so re-inject is stable.
		for end < len(existing) && existing[end] == '\n' {
			end++
		}
		return existing[:start] + block + existing[end:]
	}
	if strings.TrimSpace(existing) == "" {
		return block
	}
	existing = strings.TrimRight(existing, "\n") + "\n\n"
	return existing + block
}

// Status checks injector presence in a project.
func Status(repoRoot string) map[string]bool {
	checks := map[string]string{
		"AGENTS.md":      filepath.Join(repoRoot, "AGENTS.md"),
		"CLAUDE.md":      filepath.Join(repoRoot, "CLAUDE.md"),
		"cursor-rule":    filepath.Join(repoRoot, ".cursor", "rules", "superopen.mdc"),
		"skill-agents":   filepath.Join(repoRoot, ".agents", "skills", "so", "SKILL.md"),
		"skill-claude":   filepath.Join(repoRoot, ".claude", "skills", "so", "SKILL.md"),
		"skill-cursor":   filepath.Join(repoRoot, ".cursor", "skills", "so", "SKILL.md"),
		"skill-codex":    filepath.Join(repoRoot, ".codex", "skills", "so", "SKILL.md"),
		"skill-gemini":   filepath.Join(repoRoot, ".gemini", "skills", "so", "SKILL.md"),
		"skill-opencode": filepath.Join(repoRoot, ".opencode", "skills", "so", "SKILL.md"),
		"skill-copilot":  filepath.Join(repoRoot, ".github", "skills", "so", "SKILL.md"),
		"skill-pi":       filepath.Join(repoRoot, ".pi", "skills", "so", "SKILL.md"),
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

// StatusFor checks only the shared files and configured vendor integrations.
// Optional .agents and unconfigured vendors must not make doctor fail.
func StatusFor(repoRoot string, vendors []string, sharedAgents bool) map[string]bool {
	checks := map[string]string{"AGENTS.md": filepath.Join(repoRoot, "AGENTS.md")}
	if sharedAgents {
		checks["skill-agents"] = filepath.Join(repoRoot, ".agents", "skills", "so", "SKILL.md")
	}
	for _, vendor := range vendors {
		switch strings.ToLower(strings.TrimSpace(vendor)) {
		case "claude", "claude-code":
			checks["CLAUDE.md"] = filepath.Join(repoRoot, "CLAUDE.md")
			checks["skill-claude"] = filepath.Join(repoRoot, ".claude", "skills", "so", "SKILL.md")
		case "cursor":
			checks["cursor-rule"] = filepath.Join(repoRoot, ".cursor", "rules", "superopen.mdc")
			checks["skill-cursor"] = filepath.Join(repoRoot, ".cursor", "skills", "so", "SKILL.md")
		case "codex":
			checks["skill-codex"] = filepath.Join(repoRoot, ".codex", "skills", "so", "SKILL.md")
		case "gemini":
			checks["skill-gemini"] = filepath.Join(repoRoot, ".gemini", "skills", "so", "SKILL.md")
		case "opencode":
			checks["skill-opencode"] = filepath.Join(repoRoot, ".opencode", "skills", "so", "SKILL.md")
		case "copilot", "copilot-cli":
			checks["skill-copilot"] = filepath.Join(repoRoot, ".github", "skills", "so", "SKILL.md")
		case "pi":
			checks["skill-pi"] = filepath.Join(repoRoot, ".pi", "skills", "so", "SKILL.md")
		}
	}
	out := make(map[string]bool, len(checks))
	for name, path := range checks {
		data, err := os.ReadFile(path)
		out[name] = err == nil && (strings.Contains(string(data), startMarker) ||
			strings.Contains(string(data), "Superopen") || strings.Contains(string(data), "name: so") || strings.Contains(string(data), "/so"))
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
