package install

import (
	"fmt"

	"github.com/ishanjainn/superopen/internal/agent/install/hooks"
	"github.com/ishanjainn/superopen/internal/agent/vendors"
)

func installCatalogVendor(spec vendors.Spec, dryRun bool) ([]string, error) {
	if dryRun {
		return hooks.TargetPaths(spec.ID, hooks.LevelUser)
	}
	opts := hooks.LevelUser
	switch spec.ID {
	case "antigravity":
		st, err := hooks.InstallAntigravity(hooks.AntigravityOptions{Level: opts})
		return pathsOf(st.ConfigPath), err
	case "cline":
		st, err := hooks.InstallCline(hooks.ClineOptions{Level: opts})
		return pathsOf(st.PluginPath), err
	case "dsh":
		st, err := hooks.InstallDsh(hooks.DshOptions{Level: opts})
		return pathsOf(st.HooksPath, st.PatchPath), err
	case "devin":
		st, err := hooks.InstallDevin(hooks.DevinOptions{Level: opts})
		return pathsOf(st.ConfigPath), err
	case "factory":
		st, err := hooks.InstallFactory(hooks.FactoryOptions{Level: opts})
		return pathsOf(st.SettingsPath), err
	case "grok":
		st, err := hooks.InstallGrok(hooks.GrokOptions{Level: opts})
		return pathsOf(st.HooksPath), err
	case "hermes":
		st, err := hooks.InstallHermes(hooks.HermesOptions{Level: opts})
		return pathsOf(st.ConfigPath), err
	case "kimi":
		st, err := hooks.InstallKimi(hooks.KimiOptions{Level: opts})
		return pathsOf(st.HooksPath), err
	case "kiro":
		st, err := hooks.InstallKiro(hooks.KiroOptions{Level: opts})
		return pathsOf(st.HooksPath), err
	case "muse":
		st, err := hooks.InstallMuse(hooks.MuseOptions{Level: opts})
		return pathsOf(st.HooksPath, st.SettingsPath), err
	case "omp":
		st, err := hooks.InstallOmp(hooks.OmpOptions{Level: opts})
		return pathsOf(st.ExtensionPath), err
	case "openclaw":
		st, err := hooks.InstallOpenClaw(hooks.OpenClawOptions{Level: opts})
		return pathsOf(st.PluginPath), err
	case "openhands":
		st, err := hooks.InstallOpenHands(hooks.OpenHandsOptions{Level: opts})
		return pathsOf(st.HooksPath), err
	case "prime":
		st, err := hooks.InstallPrime(hooks.PrimeOptions{Level: opts})
		return pathsOf(st.ExtensionPath), err
	case "qwen":
		st, err := hooks.InstallQwen(hooks.QwenOptions{Level: opts})
		return pathsOf(st.SettingsPath), err
	case "senpi":
		st, err := hooks.InstallOmo(hooks.OmoOptions{Level: opts})
		return pathsOf(st.ExtensionPath), err
	case "vscode":
		st, err := hooks.InstallVSCode(hooks.VSCodeOptions{Level: opts})
		return pathsOf(st.HooksPath), err
	default:
		return nil, fmt.Errorf("unsupported catalog vendor %q", spec.ID)
	}
}

// UninstallCatalog removes hooks written for one extra agent.
func UninstallCatalog(spec vendors.Spec, dryRun bool) ([]string, error) {
	if dryRun {
		return hooks.TargetPaths(spec.ID, hooks.LevelUser)
	}
	opts := hooks.LevelUser
	switch spec.ID {
	case "antigravity":
		st, err := hooks.UninstallAntigravity(hooks.AntigravityOptions{Level: opts})
		return pathsOf(st.ConfigPath), err
	case "cline":
		st, err := hooks.UninstallCline(hooks.ClineOptions{Level: opts})
		return pathsOf(st.PluginPath), err
	case "dsh":
		st, err := hooks.UninstallDsh(hooks.DshOptions{Level: opts})
		return pathsOf(st.HooksPath, st.PatchPath), err
	case "devin":
		st, err := hooks.UninstallDevin(hooks.DevinOptions{Level: opts})
		return pathsOf(st.ConfigPath), err
	case "factory":
		st, err := hooks.UninstallFactory(hooks.FactoryOptions{Level: opts})
		return pathsOf(st.SettingsPath), err
	case "grok":
		st, err := hooks.UninstallGrok(hooks.GrokOptions{Level: opts})
		return pathsOf(st.HooksPath), err
	case "hermes":
		st, err := hooks.UninstallHermes(hooks.HermesOptions{Level: opts})
		return pathsOf(st.ConfigPath), err
	case "kimi":
		st, err := hooks.UninstallKimi(hooks.KimiOptions{Level: opts})
		return pathsOf(st.HooksPath), err
	case "kiro":
		st, err := hooks.UninstallKiro(hooks.KiroOptions{Level: opts})
		return pathsOf(st.HooksPath), err
	case "muse":
		st, err := hooks.UninstallMuse(hooks.MuseOptions{Level: opts})
		return pathsOf(st.HooksPath, st.SettingsPath), err
	case "omp":
		st, err := hooks.UninstallOmp(hooks.OmpOptions{Level: opts})
		return pathsOf(st.ExtensionPath), err
	case "openclaw":
		st, err := hooks.UninstallOpenClaw(hooks.OpenClawOptions{Level: opts})
		return pathsOf(st.PluginPath), err
	case "openhands":
		st, err := hooks.UninstallOpenHands(hooks.OpenHandsOptions{Level: opts})
		return pathsOf(st.HooksPath), err
	case "prime":
		st, err := hooks.UninstallPrime(hooks.PrimeOptions{Level: opts})
		return pathsOf(st.ExtensionPath), err
	case "qwen":
		st, err := hooks.UninstallQwen(hooks.QwenOptions{Level: opts})
		return pathsOf(st.SettingsPath), err
	case "senpi":
		st, err := hooks.UninstallOmo(hooks.OmoOptions{Level: opts})
		return pathsOf(st.ExtensionPath), err
	case "vscode":
		st, err := hooks.UninstallVSCode(hooks.VSCodeOptions{Level: opts})
		return pathsOf(st.HooksPath), err
	default:
		return nil, fmt.Errorf("unsupported catalog vendor %q", spec.ID)
	}
}

func pathsOf(paths ...string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if path != "" {
			out = append(out, path)
		}
	}
	return out
}
