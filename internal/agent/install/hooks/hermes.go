package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type HermesOptions struct {
	Level    Level
	LogPath  string
	UserMode bool
}

type HermesStatus struct {
	Installed  bool   `json:"installed"`
	BinaryPath string `json:"binary_path,omitempty"`
	ConfigPath string `json:"config_path,omitempty"`
	Message    string `json:"message,omitempty"`
}

type hermesConfig struct {
	values map[string]interface{}
	hooks  map[string][]hermesHookRef
}

type hermesHookRef struct {
	Matcher string `yaml:"matcher,omitempty"`
	Command string `yaml:"command"`
	Timeout int    `yaml:"timeout,omitempty"`
}

var hermesRuntime = hookRuntime{
	displayName: "Hermes Agent",
	configPath:  hermesConfigPath,
	install:     installHermesConfig,
	uninstall:   removeHermesEndpointHooks,
	isInstalled: isHermesInstalledAt,
}

func InstallHermes(opts HermesOptions) (HermesStatus, error) {
	status, err := installRuntimeHooks(hermesRuntime, RuntimeOptions(opts))
	if err != nil {
		return HermesStatus{}, err
	}
	return hermesStatusFromRuntime(status), nil
}

func UninstallHermes(opts HermesOptions) (HermesStatus, error) {
	status, err := uninstallRuntimeHooks(hermesRuntime, RuntimeOptions(opts))
	if err != nil {
		return HermesStatus{}, err
	}
	return hermesStatusFromRuntime(status), nil
}

func HermesHookStatus(opts HermesOptions) HermesStatus {
	return hermesStatusFromRuntime(runtimeHookStatus(hermesRuntime, RuntimeOptions(opts)))
}

func IsHermesInstalled(opts HermesOptions) bool {
	return isRuntimeInstalled(hermesRuntime, RuntimeOptions(opts))
}

func hermesStatusFromRuntime(status runtimeStatus) HermesStatus {
	return HermesStatus{
		Installed:  status.Installed,
		BinaryPath: status.BinaryPath,
		ConfigPath: status.ConfigPath,
		Message:    status.Message,
	}
}

func installHermesConfig(path, binaryPath, logPath, configPath string) error {
	config, err := readHermesConfig(path)
	if err != nil {
		return err
	}
	if err := installHermesPlugin(filepath.Dir(path), binaryPath); err != nil {
		return err
	}
	enableHermesPlugin(config.values)
	prefix := endpointCommandPrefix("hermes", binaryPath, logPath, configPath)
	endpointHooks := map[string]hermesHookRef{
		"on_session_start":       {Command: hookLine(prefix, "session-start")},
		"pre_llm_call":           {Command: hookLine(prefix, "prompt-submit"), Timeout: 30},
		"pre_tool_call":          {Matcher: ".*", Command: hookLine(prefix, "pre-tool"), Timeout: 10},
		"post_tool_call":         {Matcher: ".*", Command: hookLine(prefix, "post-tool"), Timeout: 10},
		"pre_approval_request":   {Command: hookLine(prefix, "permission-request"), Timeout: 10},
		"post_approval_response": {Command: hookLine(prefix, "permission-request"), Timeout: 10},
		"subagent_stop":          {Command: hookLine(prefix, "subagent-stop"), Timeout: 10},
		"on_session_end":         {Command: hookLine(prefix, "session-end")},
		"on_session_finalize":    {Command: hookLine(prefix, "session-end")},
	}
	for eventName, ref := range endpointHooks {
		config.hooks[eventName] = mergeHermesEndpointHook(config.hooks[eventName], ref)
	}
	return writeHermesConfig(path, config)
}

// Hermes used to need its own builder, and now it does not.
//
// It prefixed the command with `env`, which is what an inline `VAR=value cmd` prefix requires when the
// command is executed without a shell to interpret it. Passing the settings as flags removes the need
// for a prefix at all, so there is nothing left for `env` to set and the special case collapses into
// the shared builder.
//
// One pre-existing question this does not answer: if Hermes really does invoke without a shell, the
// quoting around each path is literal there rather than syntax. That was equally true of the old form,
// and Hermes is not a runtime this repository can exercise, so it stays as it was rather than being
// changed on a guess.

func readHermesConfig(path string) (hermesConfig, error) {
	config := hermesConfig{
		values: map[string]interface{}{},
		hooks:  map[string][]hermesHookRef{},
	}
	data, err := os.ReadFile(path)
	if err == nil {
		if len(data) > 0 {
			if err := yaml.Unmarshal(data, &config.values); err != nil {
				return hermesConfig{}, err
			}
		}
	} else if !os.IsNotExist(err) {
		return hermesConfig{}, err
	}
	if config.values == nil {
		config.values = map[string]interface{}{}
	}
	if rawHooks, ok := config.values["hooks"]; ok && rawHooks != nil {
		data, err := yaml.Marshal(rawHooks)
		if err != nil {
			return hermesConfig{}, err
		}
		if err := yaml.Unmarshal(data, &config.hooks); err != nil {
			return hermesConfig{}, err
		}
	}
	if config.hooks == nil {
		config.hooks = map[string][]hermesHookRef{}
	}
	return config, nil
}

func writeHermesConfig(path string, config hermesConfig) error {
	out := make(map[string]interface{}, len(config.values)+1)
	for key, value := range config.values {
		if key != "hooks" {
			out[key] = value
		}
	}
	if len(config.hooks) > 0 {
		out["hooks"] = config.hooks
	}
	data, err := yaml.Marshal(out)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func mergeHermesEndpointHook(existing []hermesHookRef, ref hermesHookRef) []hermesHookRef {
	out := make([]hermesHookRef, 0, len(existing)+1)
	for _, item := range existing {
		if !isEndpointHookCommand(item.Command, "hermes") {
			out = append(out, item)
		}
	}
	return append(out, ref)
}

func removeHermesEndpointHooks(path string) (bool, error) {
	config, err := readHermesConfig(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	changed := false
	if hermesPluginManaged(filepath.Dir(path)) {
		changed = true
		removeHermesPlugin(config.values)
		if err := removeHermesPluginFiles(filepath.Dir(path)); err != nil {
			return false, err
		}
	}
	for eventName, refs := range config.hooks {
		filtered := refs[:0]
		for _, ref := range refs {
			if isEndpointHookCommand(ref.Command, "hermes") {
				changed = true
				continue
			}
			filtered = append(filtered, ref)
		}
		if len(filtered) == 0 {
			delete(config.hooks, eventName)
		} else {
			config.hooks[eventName] = filtered
		}
	}
	if !changed {
		return false, nil
	}
	if len(config.hooks) == 0 {
		delete(config.values, "hooks")
	}
	if len(config.values) == 0 && len(config.hooks) == 0 {
		return true, os.Remove(path)
	}
	return true, writeHermesConfig(path, config)
}

const hermesPluginMarker = "# Managed by Superopen."

const hermesPluginSource = `# Managed by Superopen.
import json
import os
import subprocess

SO_BINARY = __SO_BINARY__


def _transform(tool_name="", args=None, result=None, status=None, **kwargs):
    if tool_name != "terminal" or not isinstance(args, dict) or not isinstance(result, str) or len(result) < 2048:
        return None
    if status in ("error", "failed", "failure"):
        return None
    command = args.get("command")
    if not isinstance(command, str):
        return None
    cwd = kwargs.get("cwd") or args.get("working_directory") or os.getcwd()
    payload = {"cwd": cwd, "tool_name": "bash", "tool_input": {"command": command}, "tool_response": result}
    try:
        completed = subprocess.run(
            [SO_BINARY, "sessions", "hook", "--vendor=hermes", "--event=transformToolResult"],
            input=json.dumps(payload), text=True, capture_output=True, timeout=5,
        )
        if completed.returncode == 0:
            replacement = json.loads(completed.stdout).get("output_replacement")
            return replacement if isinstance(replacement, str) else None
    except (OSError, ValueError, subprocess.TimeoutExpired):
        pass
    return None


def register(ctx):
    ctx.register_hook("transform_tool_result", _transform)
`

func installHermesPlugin(home, binaryPath string) error {
	dir := filepath.Join(home, "plugins", "superopen")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	module := filepath.Join(dir, "__init__.py")
	if existing, err := os.ReadFile(module); err == nil && !strings.HasPrefix(string(existing), hermesPluginMarker) {
		return fmt.Errorf("refusing to overwrite unmanaged Hermes plugin %s", module)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	manifest := filepath.Join(dir, "plugin.yaml")
	if _, err := os.Stat(module); os.IsNotExist(err) {
		if _, err := os.Stat(manifest); err == nil {
			return fmt.Errorf("refusing to overwrite unmanaged Hermes plugin %s", manifest)
		} else if !os.IsNotExist(err) {
			return err
		}
	} else if existing, err := os.ReadFile(manifest); err == nil && !strings.HasPrefix(string(existing), hermesPluginMarker) {
		return fmt.Errorf("refusing to overwrite unmanaged Hermes plugin %s", manifest)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	quoted, err := json.Marshal(binaryPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(module, []byte(strings.Replace(hermesPluginSource, "__SO_BINARY__", string(quoted), 1)), 0600); err != nil {
		return err
	}
	return os.WriteFile(manifest, []byte(hermesPluginMarker+"\nname: superopen\nversion: 1.0.0\ndescription: Compact noisy terminal results\nprovides_hooks:\n  - transform_tool_result\n"), 0600)
}

func enableHermesPlugin(values map[string]interface{}) {
	plugins, _ := values["plugins"].(map[string]interface{})
	if plugins == nil {
		plugins = map[string]interface{}{}
	}
	enabled, _ := plugins["enabled"].([]interface{})
	for _, name := range enabled {
		if name == "superopen" {
			values["plugins"] = plugins
			return
		}
	}
	plugins["enabled"] = append(enabled, "superopen")
	values["plugins"] = plugins
}

func removeHermesPlugin(values map[string]interface{}) bool {
	plugins, _ := values["plugins"].(map[string]interface{})
	if plugins == nil {
		return false
	}
	enabled, _ := plugins["enabled"].([]interface{})
	filtered := make([]interface{}, 0, len(enabled))
	changed := false
	for _, name := range enabled {
		if name == "superopen" {
			changed = true
			continue
		}
		filtered = append(filtered, name)
	}
	if changed {
		if len(filtered) == 0 {
			delete(plugins, "enabled")
		} else {
			plugins["enabled"] = filtered
		}
		if len(plugins) == 0 {
			delete(values, "plugins")
		}
	}
	return changed
}

func removeHermesPluginFiles(home string) error {
	dir := filepath.Join(home, "plugins", "superopen")
	module := filepath.Join(dir, "__init__.py")
	data, err := os.ReadFile(module)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || !strings.HasPrefix(string(data), hermesPluginMarker) {
		return err
	}
	if err := os.Remove(module); err != nil {
		return err
	}
	manifest := filepath.Join(dir, "plugin.yaml")
	if body, err := os.ReadFile(manifest); err == nil && strings.HasPrefix(string(body), hermesPluginMarker) {
		if err := os.Remove(manifest); err != nil {
			return err
		}
	}
	_ = os.Remove(dir) // Leave any user files in place.
	return nil
}

func hermesPluginManaged(home string) bool {
	data, err := os.ReadFile(filepath.Join(home, "plugins", "superopen", "__init__.py"))
	return err == nil && strings.HasPrefix(string(data), hermesPluginMarker)
}

func isHermesInstalledAt(path string) bool {
	config, err := readHermesConfig(path)
	if err != nil {
		return false
	}
	for _, refs := range config.hooks {
		for _, ref := range refs {
			if isEndpointHookCommand(ref.Command, "hermes") {
				return true
			}
		}
	}
	return false
}

func hermesConfigPath(level Level) (string, error) {
	switch level {
	case "", LevelUser:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".hermes", "config.yaml"), nil
	case LevelProject:
		return "", fmt.Errorf("Hermes Agent endpoint hooks support user-level config only")
	default:
		return "", fmt.Errorf("unknown hook level %q", level)
	}
}
