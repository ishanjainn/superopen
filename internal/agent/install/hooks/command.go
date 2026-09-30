package hooks

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/paths"
)

// Level is the install scope a runtime documents: the user home, or the
// working tree.
type Level string

const (
	LevelUser    Level = "user"
	LevelProject Level = "project"
)

type RuntimeOptions struct {
	Level    Level
	LogPath  string
	UserMode bool
}

type runtimeStatus struct {
	Installed  bool
	BinaryPath string
	ConfigPath string
	Message    string
}

type hookRuntime struct {
	displayName string
	configPath  func(Level) (string, error)
	install     func(path, binaryPath, logPath, configPath string) error
	uninstall   func(path string) (bool, error)
	isInstalled func(path string) bool
}

func installRuntimeHooks(runtime hookRuntime, opts RuntimeOptions) (runtimeStatus, error) {
	configPath, err := runtime.configPath(opts.Level)
	if err != nil {
		return runtimeStatus{}, err
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return runtimeStatus{}, err
	}
	soBin, err := resolveSoBin()
	if err != nil {
		soBin, err = writeSoStub()
		if err != nil {
			return runtimeStatus{}, err
		}
	}
	if err := runtime.install(configPath, soBin, "", ""); err != nil {
		return runtimeStatus{}, err
	}
	return runtimeStatus{
		Installed:  true,
		BinaryPath: soBin,
		ConfigPath: configPath,
		Message:    fmt.Sprintf("%s hooks installed", runtime.displayName),
	}, nil
}

func uninstallRuntimeHooks(runtime hookRuntime, opts RuntimeOptions) (runtimeStatus, error) {
	configPath, err := runtime.configPath(opts.Level)
	if err != nil {
		return runtimeStatus{}, err
	}
	updated, err := runtime.uninstall(configPath)
	if err != nil {
		return runtimeStatus{}, err
	}
	status := runtimeStatus{
		ConfigPath: configPath,
		Message:    fmt.Sprintf("%s hooks were not present", runtime.displayName),
	}
	if updated {
		status.Message = fmt.Sprintf("%s hooks removed", runtime.displayName)
	}
	status.Installed = isRuntimeInstalled(runtime, opts)
	return status, nil
}

func runtimeHookStatus(runtime hookRuntime, opts RuntimeOptions) runtimeStatus {
	configPath, err := runtime.configPath(opts.Level)
	if err != nil {
		return runtimeStatus{Message: err.Error()}
	}
	status := runtimeStatus{ConfigPath: configPath}
	status.Installed = isRuntimeInstalled(runtime, opts)
	if status.Installed {
		status.Message = fmt.Sprintf("%s hooks are installed", runtime.displayName)
	} else {
		status.Message = fmt.Sprintf("%s hooks are not installed", runtime.displayName)
	}
	if path, err := resolveSoBin(); err == nil {
		status.BinaryPath = path
	}
	return status
}

func isRuntimeInstalled(runtime hookRuntime, opts RuntimeOptions) bool {
	configPath, err := runtime.configPath(opts.Level)
	if err != nil {
		return false
	}
	return runtime.isInstalled(configPath)
}

// endpointCommandPrefix is the shell command up to the event. Callers append
// the event with hookLine.
func endpointCommandPrefix(platform, binaryPath, _, _ string) string {
	bin := strings.TrimSpace(binaryPath)
	if bin == "" {
		bin = "so"
	}
	return hookCommandQuote(bin) + " sessions hook --vendor=" + platform
}

func endpointCommandArgs(platform, binaryPath, _, _ string) []string {
	bin := strings.TrimSpace(binaryPath)
	if bin == "" {
		bin = "so"
	}
	return []string{bin, "sessions", "hook", "--vendor=" + platform}
}

var subcommandEvent = map[string]string{
	"session-start":      "SessionStart",
	"session-end":        "SessionEnd",
	"prompt-submit":      "UserPromptSubmit",
	"pre-tool":           "PreToolUse",
	"post-tool":          "PostToolUse",
	"permission-request": "PermissionRequest",
	"stop":               "Stop",
	"subagent-start":     "SubagentStart",
	"subagent-stop":      "SubagentStop",
	"pre-compact":        "PreCompact",
	"post-compact":       "PostCompact",
}

func hookLine(prefix, sub string) string {
	ev := subcommandEvent[sub]
	if ev == "" {
		ev = sub
	}
	return prefix + " --event=" + ev
}

func hookCommandQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func isEndpointHookCommand(command, platform string) bool {
	if platform == "" {
		return strings.Contains(command, "sessions hook") || commandNamesPlatform(command)
	}
	return commandHasPlatform(command, platform)
}

func commandNamesPlatform(command string) bool {
	for _, field := range commandFields(command) {
		if field == "--vendor" || strings.HasPrefix(field, "--vendor=") {
			return true
		}
	}
	return false
}

func commandHasPlatform(command, platform string) bool {
	fields := commandFields(command)
	for i, field := range fields {
		if field == "--vendor" {
			return i+1 < len(fields) && fields[i+1] == platform
		}
		if value, found := strings.CutPrefix(field, "--vendor="); found {
			return value == platform
		}
	}
	return false
}

func commandFields(command string) []string {
	var (
		fields  []string
		current strings.Builder
		quote   rune
		started bool
	)
	flush := func() {
		if started {
			fields = append(fields, current.String())
			current.Reset()
			started = false
		}
	}
	for _, r := range command {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
				break
			}
			current.WriteRune(r)
		case r == '"' || r == '\'':
			quote = r
			started = true
		case r == ' ' || r == '\t':
			flush()
		default:
			current.WriteRune(r)
			started = true
		}
	}
	flush()
	return fields
}

func resolveSoBin() (string, error) {
	if exe, err := os.Executable(); err == nil && paths.IsSoBinary(exe) {
		return exe, nil
	}
	if path, err := paths.LookPathSo(); err == nil {
		return path, nil
	}
	if path, err := exec.LookPath("so"); err == nil {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate so binary")
	}
	stub := filepath.Join(home, ".superopen", "bin", "so")
	if _, err := os.Stat(stub); err == nil {
		return stub, nil
	}
	return "", fmt.Errorf("locate so binary")
}

func writeSoStub() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	stub := filepath.Join(home, ".superopen", "bin", "so")
	if err := os.MkdirAll(filepath.Dir(stub), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		return "", err
	}
	return stub, nil
}
