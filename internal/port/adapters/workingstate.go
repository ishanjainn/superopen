package adapters

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/ishanjainn/superopen/internal/port"
)

// Caps on what a single session contributes, so a long session cannot blow up
// the resume inject. Overflow is reported as a count, never silently dropped.
const (
	maxFilesRead   = 40
	maxFilesEdited = 40
	maxCommands    = 30
	maxCmdLen      = 200
)

// wsCollector accumulates working state across a session's tool calls,
// de-duplicating paths while preserving first-seen order.
type wsCollector struct {
	cwd        string
	read       []string
	edited     []string
	commands   []port.RanCommand
	branch     string
	seenRead   map[string]bool
	seenEdited map[string]bool
	// commandIdx maps a command string to its index in commands, so a repeated
	// command updates its exit status in place instead of appending a duplicate.
	commandIdx map[string]int
	// lastCommandIdx is the index of the command a following tool output most
	// likely belongs to, tracked separately from len(commands)-1 because a
	// repeat command doesn't append.
	lastCommandIdx int
}

func newWSCollector(cwd string) *wsCollector {
	return &wsCollector{
		cwd:            cwd,
		seenRead:       map[string]bool{},
		seenEdited:     map[string]bool{},
		commandIdx:     map[string]int{},
		lastCommandIdx: -1,
	}
}

// editTools and readTools are matched against the lowercased, namespace-stripped
// tool name, so vendor prefixes (mcp__x__read, functions.read) still classify.
var editTools = []string{"edit", "write", "create", "patch", "apply", "update", "insert", "replace", "multiedit", "notebookedit"}

var readTools = []string{"read", "view", "open", "cat", "grep", "glob", "search", "find", "list", "ls"}

// normalizeToolName strips vendor namespacing and lowercases.
func normalizeToolName(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	// mcp__server__tool / functions.read / shell-exec → last segment
	for _, sep := range []string{"__", ".", ":"} {
		if i := strings.LastIndex(n, sep); i >= 0 && i+len(sep) < len(n) {
			n = n[i+len(sep):]
		}
	}
	return n
}

func matchesAny(name string, candidates []string) bool {
	for _, c := range candidates {
		if strings.Contains(name, c) {
			return true
		}
	}
	return false
}

// relPath makes a path repo-relative and slash-normalized for display.
func (w *wsCollector) relPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if w.cwd != "" && filepath.IsAbs(p) {
		if rel, err := filepath.Rel(w.cwd, p); err == nil && !strings.HasPrefix(rel, "..") {
			p = rel
		}
	}
	return filepath.ToSlash(p)
}

// addFile records a path against the read or edited set.
func (w *wsCollector) addFile(raw string, isEdit bool) {
	p := w.relPath(raw)
	if p == "" {
		return
	}
	if isEdit {
		if w.seenEdited[p] {
			return
		}
		w.seenEdited[p] = true
		w.edited = append(w.edited, p)
		return
	}
	// A file that was edited is more usefully reported as edited only.
	if w.seenRead[p] {
		return
	}
	w.seenRead[p] = true
	w.read = append(w.read, p)
}

// addCommand records a shell invocation and its exit status when known. A
// command seen again (e.g. `go test ./...` re-run after a fix) updates the
// existing entry's status in place, since the most recent run is what the
// destination agent needs to know, not the first.
func (w *wsCollector) addCommand(cmd string, exit *int) {
	cmd = strings.TrimSpace(strings.ReplaceAll(cmd, "\n", " "))
	if cmd == "" {
		return
	}
	if len(cmd) > maxCmdLen {
		cmd = cmd[:maxCmdLen] + "…"
	}
	if idx, ok := w.commandIdx[cmd]; ok {
		if exit != nil {
			w.commands[idx].ExitCode = exit
		}
		w.lastCommandIdx = idx
		return
	}
	w.commandIdx[cmd] = len(w.commands)
	w.lastCommandIdx = len(w.commands)
	w.commands = append(w.commands, port.RanCommand{Cmd: cmd, ExitCode: exit})
	w.noteBranch(cmd)
}

// attachExit sets the exit status on the command a just-seen tool output
// belongs to (the most recently touched one, first-run or repeat), always
// overwriting so the most recent run's status wins. No-op if none recorded.
func (w *wsCollector) attachExit(exit int) {
	if w.lastCommandIdx < 0 || w.lastCommandIdx >= len(w.commands) {
		return
	}
	w.commands[w.lastCommandIdx].ExitCode = &exit
}

// noteBranch recovers an intended branch from checkout/switch commands.
func (w *wsCollector) noteBranch(cmd string) {
	fields := strings.Fields(cmd)
	for i := 0; i < len(fields)-1; i++ {
		if fields[i] != "checkout" && fields[i] != "switch" {
			continue
		}
		cand := fields[i+1]
		if cand == "-b" || cand == "-c" {
			if i+2 >= len(fields) {
				return
			}
			cand = fields[i+2]
		}
		if cand != "" && !strings.HasPrefix(cand, "-") {
			w.branch = cand
		}
		return
	}
}

// observe classifies one tool call into working state. input is the tool's
// argument object; it is read defensively since shapes vary by harness.
func (w *wsCollector) observe(toolName string, input map[string]any, exit *int) {
	name := normalizeToolName(toolName)
	if name == "" {
		return
	}

	if cmd := firstStringField(input, "command", "cmd", "script", "shell_command"); cmd != "" {
		w.addCommand(cmd, exit)
		return
	}

	path := firstStringField(input, "file_path", "filePath", "path", "target_file", "filename", "file", "notebook_path")
	if path == "" {
		if pattern := firstStringField(input, "pattern", "query"); pattern != "" && matchesAny(name, readTools) {
			// Searches have no single path; the search itself is the useful signal.
			w.addCommand(name+" "+pattern, nil)
		}
		return
	}
	switch {
	case matchesAny(name, editTools):
		w.addFile(path, true)
	case matchesAny(name, readTools):
		w.addFile(path, false)
	}
}

// extractExitCode probes a tool-output value for an exit/status code. The value
// may be a JSON-encoded string (Codex) or an already-decoded map (OpenCode);
// key names vary across harnesses, so several plausible ones are tried.
func extractExitCode(output any) (int, bool) {
	if s, ok := output.(string); ok {
		var decoded map[string]any
		if json.Unmarshal([]byte(s), &decoded) == nil {
			output = decoded
		}
	}
	m, ok := output.(map[string]any)
	if !ok {
		return 0, false
	}
	for _, k := range []string{"exit_code", "exitCode", "status", "returncode"} {
		if v, ok := m[k].(float64); ok {
			return int(v), true
		}
	}
	return 0, false
}

// firstStringField returns the first non-empty string value among keys.
func firstStringField(m map[string]any, keys ...string) string {
	if m == nil {
		return ""
	}
	for _, k := range keys {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// result materializes the collected state, applying caps.
func (w *wsCollector) result() port.WorkingState {
	// Paths recorded as edited should not also appear under read.
	read := make([]string, 0, len(w.read))
	for _, p := range w.read {
		if !w.seenEdited[p] {
			read = append(read, p)
		}
	}
	return port.WorkingState{
		FilesRead:   capStrings(read, maxFilesRead),
		FilesEdited: capStrings(w.edited, maxFilesEdited),
		Commands:    capCommands(w.commands, maxCommands),
		GitBranch:   w.branch,
	}
}

func capStrings(in []string, max int) []string {
	if len(in) <= max {
		return in
	}
	return in[:max]
}

func capCommands(in []port.RanCommand, max int) []port.RanCommand {
	if len(in) <= max {
		return in
	}
	return in[:max]
}
