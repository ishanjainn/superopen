package agent

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/ishanjainn/superopen/internal/agent/config"
	"github.com/ishanjainn/superopen/internal/agent/install"
	"github.com/ishanjainn/superopen/internal/agent/skills"
	"github.com/ishanjainn/superopen/internal/agent/steer"
	"github.com/ishanjainn/superopen/internal/agent/vendors"
	"github.com/ishanjainn/superopen/internal/paths"
)

// Options controls optional pieces of `so install`.
type Options struct {
	Strict bool
}

// InstallReport lists paths written by a successful `so install`.
type InstallReport struct {
	Skills     []string
	Guidance   []string
	CursorRule string
	Hooks      map[string][]string
	Strict     bool
}

// Install installs coding-agent observability for the given vendors and the
// user-global /so skill and durable graph-first guidance.
// Hooks and skills are user-scoped (not cwd-dependent).
func Install(repoRoot string, selected []string, opts Options) (InstallReport, error) {
	_ = repoRoot
	report := InstallReport{Hooks: map[string][]string{}}
	_ = removeNetworkTelemetryConfig()
	if opts.Strict {
		if err := enableHookStrict(); err != nil {
			return report, fmt.Errorf("enable strict hooks: %w", err)
		}
		report.Strict = true
	}
	soBin := ""
	if exe, err := os.Executable(); err == nil && paths.IsSoBinary(exe) {
		soBin = exe
	} else if look, err := paths.LookPathSo(); err == nil {
		soBin = look
	}
	skillPaths, err := skills.InstallAll(soBin)
	if err != nil {
		return report, fmt.Errorf("install skill: %w", err)
	}
	report.Skills = skillPaths
	steerPaths, err := steer.InstallAll()
	if err != nil {
		return report, fmt.Errorf("install durable guidance: %w", err)
	}
	for _, p := range steerPaths {
		if strings.HasSuffix(p, "superopen.mdc") {
			report.CursorRule = p
			continue
		}
		report.Guidance = append(report.Guidance, p)
	}
	targets := selected
	if len(targets) == 0 {
		for _, spec := range vendors.All() {
			targets = append(targets, spec.ID)
		}
	}
	seen := make(map[string]bool, len(targets))
	for _, v := range targets {
		v = strings.ToLower(strings.TrimSpace(v))
		switch v {
		case "claude", "claude-code":
			v = "claude-code"
		case "copilot":
			v = "copilot-cli"
		case "agents", "":
			continue
		}
		if seen[v] {
			continue
		}
		seen[v] = true
		written, err := install.InstallVendor(v, false)
		if err != nil {
			return report, fmt.Errorf("install %s: %w", v, err)
		}
		report.Hooks[v] = written
	}
	return report, nil
}

// Write prints a short checklist of what install finished.
func (r InstallReport) Write(w io.Writer) {
	green, reset := "", ""
	if writerIsTTY(w) {
		green, reset = "\033[32m", "\033[0m"
	}
	tick := func(label, detail string) {
		if detail == "" {
			fmt.Fprintf(w, "  %s✓%s  %s\n", green, reset, label)
			return
		}
		fmt.Fprintf(w, "  %s✓%s  %-14s %s\n", green, reset, label, detail)
	}
	if len(r.Skills) > 0 || len(r.Guidance) > 0 || r.CursorRule != "" {
		tick("Skill", "")
	}
	if names := hookNames(r.Hooks); len(names) > 0 {
		tick("Hooks", strings.Join(names, ", "))
	}
	if r.Strict {
		tick("Strict mode", "first source read waits for a graph query")
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  Next")
	fmt.Fprintln(w, "    so init        in a repository")
	fmt.Fprintln(w, "    so dev         open the UI")
	fmt.Fprintln(w, "  Restart the coding agent so it loads the hooks.")
}

func hookNames(hooks map[string][]string) []string {
	seen := map[string]bool{}
	var names []string
	for _, spec := range vendors.All() {
		if len(hooks[spec.ID]) == 0 {
			continue
		}
		seen[spec.ID] = true
		names = append(names, spec.Label)
	}
	var extra []string
	for id, paths := range hooks {
		if seen[id] || len(paths) == 0 {
			continue
		}
		extra = append(extra, vendors.Label(id))
	}
	sort.Strings(extra)
	return append(names, extra...)
}

func writerIsTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}

// Status reports whether each vendor looks installed.
func Status(repoRoot string, vendors []string) map[string]bool {
	_ = repoRoot
	out := map[string]bool{}
	home, err := os.UserHomeDir()
	if err != nil {
		return out
	}
	codexDir, _ := paths.CodexMarketplaceDir()

	for _, v := range vendors {
		switch v {
		case "claude", "claude-code":
			manifest := filepath.Join(home, ".claude", "plugins", "superopen-cc", "hooks", "hooks.json")
			data, e := os.ReadFile(manifest)
			out["claude-code"] = e == nil && hookBinaryAvailable(string(data), "cc")
		case "cursor":
			data, e := os.ReadFile(filepath.Join(home, ".cursor", "hooks.json"))
			out["cursor"] = e == nil && (strings.Contains(string(data), "sessions hook --vendor=cursor") ||
				strings.Contains(string(data), "coding hook --vendor=cursor"))
		case "codex":
			ok := false
			if codexDir != "" {
				manifest := filepath.Join(codexDir, "plugins", "superopen", "hooks", "hooks.json")
				if data, e := os.ReadFile(manifest); e == nil && hookBinaryAvailable(string(data), "codex") {
					ok = true
				}
			}
			out["codex"] = ok
		case "gemini":
			data, e := os.ReadFile(filepath.Join(home, ".gemini", "settings.json"))
			out["gemini"] = e == nil && (strings.Contains(string(data), "sessions hook --vendor=gemini") ||
				strings.Contains(string(data), "coding hook --vendor=gemini"))
		case "opencode":
			// Host loads ~/.config/opencode/plugins (not ~/.opencode/plugins).
			base, _ := paths.OpenCodeConfigDir()
			_, e := os.Stat(filepath.Join(base, "plugins", "superopen.ts"))
			out["opencode"] = e == nil
		case "copilot-cli", "copilot":
			// Copilot CLI: ~/.copilot/hooks (not ~/.github/hooks).
			base, _ := paths.CopilotHome()
			data, e := os.ReadFile(filepath.Join(base, "hooks", "superopen.json"))
			out["copilot-cli"] = e == nil && (strings.Contains(string(data), "sessions hook --vendor=copilot") ||
				strings.Contains(string(data), "coding hook --vendor=copilot"))
		case "pi":
			// Host loads ~/.pi/agent/extensions (not ~/.pi/extensions).
			_, e := os.Stat(filepath.Join(home, ".pi", "agent", "extensions", "superopen", "index.ts"))
			out["pi"] = e == nil
		}
	}
	return out
}

func hookBinaryAvailable(manifest, vendor string) bool {
	idx := strings.Index(manifest, " sessions hook --vendor="+vendor)
	if idx < 0 {
		idx = strings.Index(manifest, " coding hook --vendor="+vendor)
	}
	if idx < 0 {
		return false
	}
	lineStart := strings.LastIndex(manifest[:idx], "\"")
	if lineStart < 0 {
		return false
	}
	bin := strings.TrimSpace(manifest[lineStart+1 : idx])
	bin = strings.Trim(bin, "'\"")
	if bin == "so" || bin == "so.exe" {
		_, err := paths.LookPathSo()
		return err == nil
	}
	info, err := os.Stat(bin)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}

func enableHookStrict() error {
	_, err := config.Save(map[string]string{"SUPEROPEN_HOOK_STRICT": "1"})
	return err
}

func removeNetworkTelemetryConfig() error {
	dir, err := paths.ConfigDir()
	if err != nil {
		return err
	}
	// Older builds stored remote-export credentials here. No current command
	// reads this file, so remove the obsolete secret during install/sync.
	_ = os.Remove(filepath.Join(dir, "auth.json"))
	path := filepath.Join(dir, "config.env")
	prev, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var kept []string
	for _, line := range strings.Split(string(prev), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "SUPEROPEN_OTLP_ENDPOINT=") ||
			strings.HasPrefix(trimmed, "OTEL_EXPORTER_OTLP_ENDPOINT=") ||
			strings.HasPrefix(trimmed, "OTEL_EXPORTER_OTLP_HEADERS=") ||
			strings.HasPrefix(trimmed, "OTEL_RESOURCE_ATTRIBUTES=") ||
			strings.HasPrefix(trimmed, "SUPEROPEN_API_KEY=") ||
			trimmed == "# written by so init / so coding install" {
			continue
		}
		if trimmed != "" {
			kept = append(kept, line)
		}
	}
	if len(kept) == 0 {
		return os.Remove(path)
	}
	return os.WriteFile(path, []byte(strings.Join(kept, "\n")+"\n"), 0o600)
}
