package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type FactoryOptions struct {
	Level    Level
	LogPath  string
	UserMode bool
}

type FactoryStatus struct {
	Installed  bool   `json:"installed"`
	BinaryPath string `json:"binary_path,omitempty"`
	HooksPath  string `json:"hooks_path,omitempty"`
	Message    string `json:"message,omitempty"`
}

type factoryHookGroup = settingsHookGroup
type factoryHookRef = settingsHookRef

var factoryRuntime = hookRuntime{
	displayName: "Factory",
	configPath:  factoryHooksPath,
	install:     installFactoryHooks,
	uninstall:   removeFactoryEndpointHooks,
	isInstalled: isFactoryInstalledAt,
}

func InstallFactory(opts FactoryOptions) (FactoryStatus, error) {
	status, err := installRuntimeHooks(factoryRuntime, RuntimeOptions(opts))
	if err != nil {
		return FactoryStatus{}, err
	}
	if _, err := removeSettingsEndpointHooks(factoryLegacySettingsPath(status.ConfigPath), "factory"); err != nil {
		return FactoryStatus{}, err
	}
	return factoryStatusFromRuntime(status), nil
}

func UninstallFactory(opts FactoryOptions) (FactoryStatus, error) {
	path, err := factoryHooksPath(opts.Level)
	if err != nil {
		return FactoryStatus{}, err
	}
	if _, err := removeSettingsEndpointHooks(factoryLegacySettingsPath(path), "factory"); err != nil {
		return FactoryStatus{}, err
	}
	status, err := uninstallRuntimeHooks(factoryRuntime, RuntimeOptions(opts))
	if err != nil {
		return FactoryStatus{}, err
	}
	return factoryStatusFromRuntime(status), nil
}

func FactoryHookStatus(opts FactoryOptions) FactoryStatus {
	return factoryStatusFromRuntime(runtimeHookStatus(factoryRuntime, RuntimeOptions(opts)))
}

func IsFactoryInstalled(opts FactoryOptions) bool {
	return isRuntimeInstalled(factoryRuntime, RuntimeOptions(opts))
}

func factoryStatusFromRuntime(status runtimeStatus) FactoryStatus {
	return FactoryStatus{
		Installed:  status.Installed,
		BinaryPath: status.BinaryPath,
		HooksPath:  status.ConfigPath,
		Message:    status.Message,
	}
}

func installFactoryHooks(path, binaryPath, logPath, configPath string) error {
	prefix := endpointCommandPrefix("factory", binaryPath, logPath, configPath)
	endpointHooks := map[string]factoryHookGroup{
		"SessionStart":     {Hooks: []factoryHookRef{{Type: "command", Command: hookLine(prefix, "session-start")}}},
		"UserPromptSubmit": {Hooks: []factoryHookRef{{Type: "command", Command: hookLine(prefix, "prompt-submit"), Timeout: 30}}},
		"PreToolUse":       {Hooks: []factoryHookRef{{Type: "command", Command: hookLine(prefix, "pre-tool")}}},
		"PostToolUse":      {Hooks: []factoryHookRef{{Type: "command", Command: hookLine(prefix, "post-tool")}}},
		"Stop":             {Hooks: []factoryHookRef{{Type: "command", Command: hookLine(prefix, "stop"), Timeout: 45}}},
		"SessionEnd":       {Hooks: []factoryHookRef{{Type: "command", Command: hookLine(prefix, "session-end")}}},
	}
	doc, err := readFactoryHooks(path)
	if err != nil {
		return err
	}
	for event, group := range endpointHooks {
		var existing []settingsHookGroup
		if raw, ok := doc[event]; ok {
			if err := json.Unmarshal(raw, &existing); err != nil {
				return fmt.Errorf("factory %s hooks: %w", event, err)
			}
		}
		merged, err := json.Marshal(mergeSettingsEndpointHook(existing, group, "factory"))
		if err != nil {
			return err
		}
		doc[event] = merged
	}
	return writeFactoryHooks(path, doc)
}

func readFactoryHooks(path string) (map[string]json.RawMessage, error) {
	doc := map[string]json.RawMessage{}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return doc, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, fmt.Errorf("factory hooks must be a JSON object")
	}
	return doc, nil
}

func writeFactoryHooks(path string, doc map[string]json.RawMessage) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

func removeFactoryEndpointHooks(path string) (bool, error) {
	doc, err := readFactoryHooks(path)
	if err != nil {
		return false, err
	}
	changed := false
	for event, raw := range doc {
		var groups []settingsHookGroup
		if json.Unmarshal(raw, &groups) != nil {
			continue
		}
		kept := make([]settingsHookGroup, 0, len(groups))
		eventChanged := false
		for _, group := range groups {
			filtered, removed := filterSettingsEndpointHooks(group, "factory")
			eventChanged = eventChanged || removed
			if !removed || len(filtered.Hooks) > 0 {
				kept = append(kept, filtered)
			}
		}
		if !eventChanged {
			continue
		}
		changed = true
		if len(kept) == 0 {
			delete(doc, event)
			continue
		}
		encoded, err := json.Marshal(kept)
		if err != nil {
			return false, err
		}
		doc[event] = encoded
	}
	if !changed {
		return false, nil
	}
	return true, writeFactoryHooks(path, doc)
}

func isFactoryInstalledAt(path string) bool {
	doc, err := readFactoryHooks(path)
	if err != nil {
		return false
	}
	for _, raw := range doc {
		var groups []settingsHookGroup
		if json.Unmarshal(raw, &groups) == nil {
			for _, group := range groups {
				if isSettingsEndpointHookGroup(group, "factory") {
					return true
				}
			}
		}
	}
	return false
}

func factoryHooksPath(level Level) (string, error) {
	switch level {
	case "", LevelUser:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".factory", "hooks.json"), nil
	case LevelProject:
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return filepath.Join(cwd, ".factory", "hooks.json"), nil
	default:
		return "", fmt.Errorf("unknown hook level %q", level)
	}
}

func factoryLegacySettingsPath(hooksPath string) string {
	return filepath.Join(filepath.Dir(hooksPath), "settings.json")
}
