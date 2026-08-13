// Package agentcli runs coding-agent CLIs in sealed non-interactive mode.
// Used by backend evals/recommendations - reuses the user's coding-agent
// login without a separate API key.
package agentcli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Result is the CLI reply text plus which model produced it when known.
type Result struct {
	Text  string
	Model string
}

// Supported lists CLIs in detection preference order.
var Supported = []string{"claude", "codex", "opencode", "pi"}

// Detect returns the first supported CLI on PATH.
func Detect() (string, error) {
	if clis := DetectAll(); len(clis) > 0 {
		return clis[0], nil
	}
	return "", fmt.Errorf("no coding-agent CLI found on PATH (looked for %s)", strings.Join(Supported, ", "))
}

// DetectAll returns every supported CLI on PATH, in preference order.
func DetectAll() []string {
	var clis []string
	for _, cli := range Supported {
		if _, err := exec.LookPath(cli); err == nil {
			clis = append(clis, cli)
		}
	}
	return clis
}

// WorkDir is ~/.so/agentcli - neutral, no repo instructions.
func WorkDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".so", "agentcli")
}

// IsWorkDir reports whether path is the sealed CLI working directory.
func IsWorkDir(path string) bool {
	dir := WorkDir()
	return dir != "" && path != "" && filepath.Clean(path) == dir
}

func ensureWorkDir() (string, error) {
	dir := WorkDir()
	if dir == "" {
		return "", fmt.Errorf("agentcli workdir: cannot resolve home directory")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("agentcli workdir: %w", err)
	}
	return dir, nil
}

// Runner shells out to a local agent CLI in non-interactive sealed mode:
// no tools, no MCP, no user/project settings - prompt injection in eval
// input must not reach the filesystem or network.
type Runner struct {
	CLI   string
	Model string // optional override; empty uses the agent's default
}

func (r Runner) Name() string { return r.CLI }

func (r Runner) Run(ctx context.Context, system, user string) (Result, error) {
	workdir, err := ensureWorkDir()
	if err != nil {
		return Result{}, err
	}
	var cmd *exec.Cmd
	switch r.CLI {
	case "claude":
		args := []string{"-p",
			"--no-session-persistence",
			"--tools", "",
			"--strict-mcp-config",
			"--setting-sources", "",
			"--output-format", "json",
		}
		if r.Model != "" {
			args = append(args, "--model", r.Model)
		}
		cmd = exec.CommandContext(ctx, "claude", append(args, system)...)
		cmd.Stdin = strings.NewReader(user)
	case "codex":
		args := codexExecArgs(workdir)
		if r.Model != "" {
			args = append(args, "-c", "model="+r.Model)
		}
		cmd = exec.CommandContext(ctx, "codex", append(args, "-")...)
		cmd.Stdin = strings.NewReader(system + "\n\n" + user)
	case "opencode":
		args := opencodeRunArgs(workdir)
		if r.Model != "" {
			args = append(args, "--model", r.Model)
		}
		cmd = exec.CommandContext(ctx, "opencode", args...)
		cmd.Stdin = strings.NewReader(system + "\n\n" + user)
		cmd.Env = opencodeSealedEnv()
	case "pi":
		args := piPrintArgs()
		if r.Model != "" {
			args = append(args, "--model", r.Model)
		}
		if system != "" {
			args = append(args, "--system-prompt", system)
		}
		cmd = exec.CommandContext(ctx, "pi", args...)
		cmd.Stdin = strings.NewReader(user)
	default:
		return Result{}, fmt.Errorf("unsupported agent CLI %q", r.CLI)
	}
	cmd.Dir = workdir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		detail = truncateRunes(detail, 500)
		return Result{}, fmt.Errorf("%s failed: %w: %s", r.CLI, err, detail)
	}
	switch r.CLI {
	case "claude":
		return parseClaudeEnvelope(stdout.String()), nil
	case "opencode":
		return parseOpenCodeJSON(stdout.String()), nil
	case "codex":
		model := codexModel(stderr.String())
		if model == "" {
			model = codexModel(stdout.String())
		}
		return Result{Text: stdout.String(), Model: model}, nil
	default:
		return Result{Text: stdout.String()}, nil
	}
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		if utf8.ValidString(s) {
			return s
		}
		return string(runes) // repairs invalid UTF-8 via rune decode
	}
	return string(runes[:max])
}

func codexExecArgs(workdir string) []string {
	return []string{"exec",
		"--ephemeral",
		"--ignore-user-config",
		"--ignore-rules",
		"--disable", "shell_tool",
		"--disable", "browser_use",
		"--disable", "browser_use_external",
		"--disable", "computer_use",
		"--disable", "in_app_browser",
		"--disable", "apps",
		"--disable", "plugins",
		"--disable", "hooks",
		"--disable", "multi_agent",
		"--disable", "multi_agent_v2",
		"--disable", "memories",
		"--disable", "image_generation",
		"-c", "include_apply_patch_tool=false",
		"-c", "tools.view_image=false",
		"-c", `web_search="disabled"`,
		"--sandbox", "read-only",
		"--skip-git-repo-check",
		"-C", workdir,
	}
}

func opencodeRunArgs(workdir string) []string {
	return []string{
		"--pure",
		"run",
		"--format", "json",
		"--dir", workdir,
	}
}

func opencodeSealedEnv() []string {
	return append(os.Environ(),
		`OPENCODE_PERMISSION={"*":"deny"}`,
		"OPENCODE_DISABLE_DEFAULT_PLUGINS=true",
		"OPENCODE_DISABLE_CLAUDE_CODE=true",
		"OPENCODE_DISABLE_AUTOUPDATE=true",
	)
}

func piPrintArgs() []string {
	return []string{
		"--print",
		"--no-tools",
		"--no-session",
		"--no-extensions",
		"--no-skills",
		"--no-prompt-templates",
		"--no-themes",
		"--no-context-files",
		"--no-approve",
	}
}

type claudeEnvelope struct {
	Result     string `json:"result"`
	ModelUsage map[string]struct {
		InputTokens              int64 `json:"inputTokens"`
		CacheReadInputTokens     int64 `json:"cacheReadInputTokens"`
		CacheCreationInputTokens int64 `json:"cacheCreationInputTokens"`
	} `json:"modelUsage"`
}

func parseClaudeEnvelope(raw string) Result {
	var envelope claudeEnvelope
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &envelope); err != nil || envelope.Result == "" {
		return Result{Text: raw}
	}
	model := ""
	var maxInput int64 = -1
	for name, usage := range envelope.ModelUsage {
		total := usage.InputTokens + usage.CacheReadInputTokens + usage.CacheCreationInputTokens
		if total > maxInput {
			maxInput = total
			model = name
		}
	}
	return Result{Text: envelope.Result, Model: model}
}

func parseOpenCodeJSON(raw string) Result {
	var texts []string
	model := ""
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev openCodeEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		if ev.Type == "text" && ev.Part.Text != "" {
			texts = append(texts, ev.Part.Text)
		}
		if model == "" {
			if id := strings.TrimSpace(ev.Part.ModelID); id != "" {
				model = id
			} else if id := strings.TrimSpace(ev.ModelID); id != "" {
				model = id
			}
		}
	}
	if len(texts) == 0 {
		return Result{Text: raw}
	}
	return Result{Text: strings.Join(texts, "\n"), Model: model}
}

type openCodeEvent struct {
	Type    string `json:"type"`
	ModelID string `json:"modelID"`
	Part    struct {
		Text    string `json:"text"`
		ModelID string `json:"modelID"`
	} `json:"part"`
}

var codexModelLine = regexp.MustCompile(`(?m)^model:\s+(\S+)`)

func codexModel(raw string) string {
	if match := codexModelLine.FindStringSubmatch(raw); match != nil {
		return match[1]
	}
	return ""
}
