package adapters

import "github.com/ishanjainn/superopen/internal/port"

// RegisterAll registers Claude, Codex, OpenCode, Cursor, Pi, and .so hub adapters.
func RegisterAll(reg *port.Registry, repoRoot string) {
	reg.RegisterImport(ClaudeImport{})
	reg.RegisterExport(ClaudeExport{})
	reg.RegisterImport(CodexImport{})
	reg.RegisterExport(CodexExport{})
	reg.RegisterImport(OpenCodeImport{})
	reg.RegisterExport(OpenCodeExport{})
	reg.RegisterImport(CursorImport{})
	reg.RegisterExport(CursorExport{RepoRoot: repoRoot})
	reg.RegisterImport(PiImport{})
	reg.RegisterExport(PiExport{})
	reg.RegisterImport(SOHubImport{RepoRoot: repoRoot})
	reg.RegisterExport(SOHubExport{RepoRoot: repoRoot})
}
