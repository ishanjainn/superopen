package agent

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ishanjainn/superopen/internal/agent/config"
	"github.com/ishanjainn/superopen/internal/agent/install"
	"github.com/ishanjainn/superopen/internal/agent/skills"
	"github.com/ishanjainn/superopen/internal/agent/steer"
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
func Install(repoRoot string, vendors []string, opts Options) (InstallReport, error) {
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
	} else if look, err := exec.LookPath("so"); err == nil {
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
	targets := vendors
	if len(targets) == 0 {
		targets = []string{"claude-code", "cursor", "codex", "gemini", "opencode", "copilot-cli", "pi"}
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
		case "kilo", "aider", "claw", "openclaw", "droid", "factory", "trae", "trae-cn", "hermes", "kiro", "devin", "codebuddy", "kimi", "amp", "antigravity", "vscode", "windows":
			// These agents do not expose a stable telemetry hook contract yet.
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

// Write prints a grouped, non-interactive summary of what install wrote.
func (r InstallReport) Write(w io.Writer) {
	fmt.Fprintln(w, "Installing Superopen…")
	writeGroup(w, "Skill", r.Skills)
	writeGroup(w, "Guidance", r.Guidance)
	if r.CursorRule != "" {
		fmt.Fprintln(w, "Cursor rule")
		fmt.Fprintf(w, "  %s\n", r.CursorRule)
	}
	if len(r.Hooks) > 0 {
		fmt.Fprintln(w, "Hooks")
		for _, vendor := range []string{"claude-code", "cursor", "codex", "gemini", "opencode", "copilot-cli", "pi"} {
			paths := r.Hooks[vendor]
			if len(paths) == 0 {
				continue
			}
			fmt.Fprintf(w, "  %s\n", vendor)
			for _, p := range paths {
				fmt.Fprintf(w, "    %s\n", p)
			}
		}
		for vendor, paths := range r.Hooks {
			switch vendor {
			case "claude-code", "cursor", "codex", "gemini", "opencode", "copilot-cli", "pi":
				continue
			}
			fmt.Fprintf(w, "  %s\n", vendor)
			for _, p := range paths {
				fmt.Fprintf(w, "    %s\n", p)
			}
		}
	}
	if r.Strict {
		fmt.Fprintln(w, "Strict mode: first in-repo source Read is denied once per session until a graph query.")
	}
}

func writeGroup(w io.Writer, title string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintln(w, title)
	for _, p := range items {
		fmt.Fprintf(w, "  %s\n", p)
	}
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
			out["cursor"] = e == nil && (strings.Contains(string(data), "so coding hook --vendor=cursor") ||
				strings.Contains(string(data), " coding hook --vendor=cursor"))
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
			out["gemini"] = e == nil && strings.Contains(string(data), "coding hook --vendor=gemini")
		case "opencode":
			// Host loads ~/.config/opencode/plugins (not ~/.opencode/plugins).
			base, _ := paths.OpenCodeConfigDir()
			_, e := os.Stat(filepath.Join(base, "plugins", "superopen.ts"))
			out["opencode"] = e == nil
		case "copilot-cli", "copilot":
			// Copilot CLI: ~/.copilot/hooks (not ~/.github/hooks).
			base, _ := paths.CopilotHome()
			data, e := os.ReadFile(filepath.Join(base, "hooks", "superopen.json"))
			out["copilot-cli"] = e == nil && strings.Contains(string(data), "coding hook --vendor=copilot")
		case "pi":
			// Host loads ~/.pi/agent/extensions (not ~/.pi/extensions).
			_, e := os.Stat(filepath.Join(home, ".pi", "agent", "extensions", "superopen", "index.ts"))
			out["pi"] = e == nil
		}
	}
	return out
}

func hookBinaryAvailable(manifest, vendor string) bool {
	marker := " coding hook --vendor=" + vendor
	idx := strings.Index(manifest, marker)
	if idx < 0 {
		return false
	}
	lineStart := strings.LastIndex(manifest[:idx], "\"")
	if lineStart < 0 {
		return false
	}
	bin := strings.TrimSpace(manifest[lineStart+1 : idx])
	bin = strings.Trim(bin, "'\"")
	if bin == "so" {
		_, err := exec.LookPath(bin)
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
