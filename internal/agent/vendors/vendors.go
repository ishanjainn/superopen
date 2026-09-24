// Package vendors lists coding agents whose hooks Superopen installs
// in addition to Claude Code, Cursor, Codex, Gemini, OpenCode, Copilot CLI, and Pi.
package vendors

// Protocol is the stdout shape a host reads from a hook.
const (
	ProtocolClaude  = "claude"
	ProtocolContext = "context"
	ProtocolPi      = "pi"
)

// Spec is one extra agent.
type Spec struct {
	ID       string
	Label    string
	Protocol string
	// Kind selects the installer. settings merges a Claude-style hooks
	// object. owned writes a file Superopen owns. The rest are named.
	Kind string
	// Rel is the config path under the user home directory.
	Rel string
}

// Core is the original set. Extra agents are appended by All.
func Core() []Spec {
	return []Spec{
		{ID: "claude-code", Label: "Claude Code"},
		{ID: "cursor", Label: "Cursor"},
		{ID: "codex", Label: "Codex"},
		{ID: "gemini", Label: "Gemini"},
		{ID: "opencode", Label: "OpenCode"},
		{ID: "copilot-cli", Label: "Copilot"},
		{ID: "pi", Label: "Pi"},
	}
}

// All is every harness so install can wire, core first, then Extra.
// New agents belong in Extra so installers pick them up without a second list.
func All() []Spec {
	return append(Core(), Extra()...)
}

// Extra is every local agent added beyond the original seven.
func Extra() []Spec {
	return []Spec{
		{ID: "antigravity", Label: "Antigravity", Protocol: ProtocolClaude, Kind: "antigravity", Rel: ".gemini/config/hooks.json"},
		{ID: "cline", Label: "Cline", Protocol: ProtocolContext, Kind: "cline", Rel: ".cline/plugins/superopen.ts"},
		{ID: "dsh", Label: "DeepSeek Harness", Protocol: ProtocolClaude, Kind: "dsh", Rel: ".dsh/superopen-hooks.json"},
		{ID: "devin", Label: "Devin", Protocol: ProtocolClaude, Kind: "settings", Rel: ".config/devin/config.json"},
		{ID: "factory", Label: "Factory Droid", Protocol: ProtocolClaude, Kind: "settings", Rel: ".factory/settings.json"},
		{ID: "grok", Label: "Grok", Protocol: ProtocolClaude, Kind: "owned", Rel: ".grok/hooks/superopen.json"},
		{ID: "hermes", Label: "Hermes", Protocol: ProtocolContext, Kind: "hermes", Rel: ".hermes/config.yaml"},
		{ID: "kimi", Label: "Kimi Code", Protocol: ProtocolClaude, Kind: "kimi", Rel: ".kimi-code/config.toml"},
		{ID: "kiro", Label: "Kiro", Protocol: ProtocolClaude, Kind: "kiro", Rel: ".kiro/hooks/superopen.json"},
		{ID: "muse", Label: "Muse", Protocol: ProtocolClaude, Kind: "muse", Rel: ".config/muse/superopen-hooks.json"},
		{ID: "omp", Label: "Oh My Pi", Protocol: ProtocolPi, Kind: "pi-fork", Rel: ".omp/agent/extensions/superopen/index.ts"},
		{ID: "openclaw", Label: "OpenClaw", Protocol: ProtocolContext, Kind: "openclaw", Rel: ".openclaw/extensions/superopen/index.js"},
		{ID: "openhands", Label: "OpenHands", Protocol: ProtocolClaude, Kind: "settings", Rel: ".openhands/hooks.json"},
		{ID: "prime", Label: "Prime", Protocol: ProtocolPi, Kind: "pi-fork", Rel: ".prime/agent/extensions/superopen/index.ts"},
		{ID: "qwen", Label: "Qwen Code", Protocol: ProtocolClaude, Kind: "settings", Rel: ".qwen/settings.json"},
		{ID: "senpi", Label: "Senpi", Protocol: ProtocolPi, Kind: "pi-fork", Rel: ".omo/agent/extensions/superopen/index.ts"},
		{ID: "vscode", Label: "VS Code", Protocol: ProtocolClaude, Kind: "owned", Rel: ".copilot/hooks/superopen-vscode.json"},
	}
}

// ByID returns one extra agent.
func ByID(id string) (Spec, bool) {
	for _, spec := range Extra() {
		if spec.ID == id {
			return spec, true
		}
	}
	return Spec{}, false
}

// IDs lists extra agent ids.
func IDs() []string {
	out := make([]string, 0, len(Extra()))
	for _, spec := range Extra() {
		out = append(out, spec.ID)
	}
	return out
}

// Label returns a display name for any known vendor id.
func Label(id string) string {
	switch id {
	case "claude-code":
		return "Claude Code"
	case "cursor":
		return "Cursor"
	case "codex":
		return "Codex"
	case "gemini":
		return "Gemini"
	case "opencode":
		return "OpenCode"
	case "copilot-cli":
		return "Copilot"
	case "pi":
		return "Pi"
	default:
		if spec, ok := ByID(id); ok {
			return spec.Label
		}
		return id
	}
}
