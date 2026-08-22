// Package repofile is the shared choke point for coding-agent file identity.
// Emit, session footprint, and memory ingest all call Accept so a shell
// command never becomes coding_agent.file_path.
package repofile

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// IsShell reports tools whose args are a command line, not a repo path.
func IsShell(tool string) bool {
	switch strings.ToLower(strings.TrimSpace(tool)) {
	case "shell", "bash", "zsh", "sh", "cmd", "powershell", "pwsh", "terminal":
		return true
	default:
		return false
	}
}

// IsRead reports Read-family file tools (not "read" as a substring of a command).
func IsRead(tool string) bool {
	switch canonicalTool(tool) {
	case "read", "read_file", "readfile", "beforereadfile":
		return true
	default:
		return false
	}
}

// IsEdit reports Edit/Write-family file tools.
func IsEdit(tool string) bool {
	switch canonicalTool(tool) {
	case "edit", "write", "strreplace", "afterfileedit", "apply_patch", "editnotebook",
		"notebookedit", "create", "delete":
		return true
	default:
		return false
	}
}

// State classifies footprint state from the tool name only.
func State(tool, spanName string) string {
	name := strings.TrimSpace(tool)
	if name == "" {
		name = spanName
	}
	switch {
	case IsEdit(name):
		return "edited"
	case IsRead(name):
		return "read"
	default:
		return "seen"
	}
}

// PathFromJSON extracts a file path from tool-argument JSON. Non-JSON
// bodies (raw shell commands) are never treated as paths.
func PathFromJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) != nil {
		return ""
	}
	for _, key := range []string{"file_path", "filePath", "path", "notebook_path"} {
		if v, ok := m[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// Accept returns a repo-relative slash path, or empty if the value is a
// command, a shell tool, or escapes the working directory.
func Accept(path, tool, cwd string) string {
	if IsShell(tool) {
		return ""
	}
	path = strings.TrimSpace(path)
	if path == "" || CommandShaped(path) {
		return ""
	}
	return Rel(path, cwd)
}

// Rel slash-normalizes path and, when cwd is set, makes absolute paths
// repo-relative. Paths that escape cwd with ".." are rejected.
func Rel(path, cwd string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = filepath.Clean(path)
	cwd = strings.TrimSpace(cwd)
	if cwd != "" && filepath.IsAbs(path) {
		rel, err := filepath.Rel(cwd, path)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return ""
		}
		return filepath.ToSlash(rel)
	}
	slash := filepath.ToSlash(path)
	if strings.HasPrefix(slash, "../") || slash == ".." {
		return ""
	}
	return slash
}

// CommandShaped is a CLI invocation, not a repo file path.
func CommandShaped(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if strings.Contains(s, " --") || strings.ContainsAny(s, "\n\t") {
		return true
	}
	fields := strings.Fields(s)
	if len(fields) < 2 {
		return false
	}
	for _, f := range fields[1:] {
		if strings.HasPrefix(f, "-") {
			return true
		}
	}
	first := strings.ToLower(filepath.Base(fields[0]))
	first = strings.TrimSuffix(first, ".exe")
	switch first {
	case "so", "npx", "npm", "yarn", "pnpm", "git", "bash", "sh", "zsh",
		"cmd", "powershell", "pwsh", "python", "python3", "node", "go", "cargo", "make":
		return true
	default:
		return false
	}
}

func canonicalTool(tool string) string {
	n := strings.ToLower(strings.TrimSpace(tool))
	n = strings.ReplaceAll(n, "-", "")
	n = strings.ReplaceAll(n, "_", "")
	return n
}
