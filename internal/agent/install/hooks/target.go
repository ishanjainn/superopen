package hooks

import (
	"fmt"
	"path/filepath"
)

// TargetPaths is where an install for id writes, without writing.
func TargetPaths(id string, level Level) ([]string, error) {
	rt, err := runtimeFor(id)
	if err != nil {
		return nil, err
	}
	path, err := rt.configPath(level)
	if err != nil {
		return nil, err
	}
	switch id {
	case "dsh":
		return []string{path, dshPatchPathFor(path)}, nil
	case "muse":
		return []string{path, filepath.Join(filepath.Dir(path), "settings.json")}, nil
	default:
		return []string{path}, nil
	}
}

func runtimeFor(id string) (hookRuntime, error) {
	switch id {
	case "antigravity":
		return antigravityRuntime, nil
	case "cline":
		return clineRuntime, nil
	case "dsh":
		return dshRuntime, nil
	case "devin":
		return devinRuntime, nil
	case "factory":
		return factoryRuntime, nil
	case "grok":
		return grokRuntime, nil
	case "hermes":
		return hermesRuntime, nil
	case "kimi":
		return kimiRuntime, nil
	case "kiro":
		return kiroRuntime, nil
	case "muse":
		return museRuntime, nil
	case "omp":
		return ompRuntime, nil
	case "openclaw":
		return openClawRuntime, nil
	case "openhands":
		return openHandsRuntime, nil
	case "prime":
		return primeRuntime, nil
	case "qwen":
		return qwenRuntime, nil
	case "senpi":
		return omoRuntime, nil
	case "vscode":
		return vscodeRuntime, nil
	default:
		return hookRuntime{}, fmt.Errorf("unsupported catalog vendor %q", id)
	}
}
