package port

import (
	"fmt"
	"strings"
)

// RenderWorkingState formats the recovered side effects of a ported session as
// a markdown block for the resume inject. Returns "" when nothing was recovered
// and no turns were dropped, so clean ports stay quiet.
func RenderWorkingState(sess PortableSession) string {
	ws := sess.WorkingState
	if ws.Empty() && sess.DroppedTurns == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Working state carried from " + string(sess.SourceHarness) + "\n\n")
	b.WriteString("The turns below are text only. Tool calls and their results were not\n")
	b.WriteString("ported, so file contents and command output are NOT in context.\n")
	b.WriteString("Re-read any file before editing it.\n\n")

	if ws.GitBranch != "" {
		b.WriteString(fmt.Sprintf("- Branch: `%s`\n", ws.GitBranch))
	}
	if len(ws.FilesEdited) > 0 {
		b.WriteString(fmt.Sprintf("- Edited (%d): %s\n", len(ws.FilesEdited), joinCode(ws.FilesEdited)))
	}
	if len(ws.FilesRead) > 0 {
		b.WriteString(fmt.Sprintf("- Read (%d): %s\n", len(ws.FilesRead), joinCode(ws.FilesRead)))
	}
	if len(ws.Commands) > 0 {
		b.WriteString(fmt.Sprintf("- Commands (%d):\n", len(ws.Commands)))
		for _, c := range ws.Commands {
			status := ""
			if c.ExitCode != nil {
				status = fmt.Sprintf("  → exit %d", *c.ExitCode)
			}
			b.WriteString(fmt.Sprintf("    - `%s`%s\n", c.Cmd, status))
		}
	}
	if sess.DroppedTurns > 0 {
		b.WriteString(fmt.Sprintf("- %d non-text turns omitted (tool calls, results, reasoning)\n", sess.DroppedTurns))
	}
	b.WriteString("\n")
	return b.String()
}

func joinCode(items []string) string {
	quoted := make([]string, 0, len(items))
	for _, s := range items {
		quoted = append(quoted, "`"+s+"`")
	}
	return strings.Join(quoted, ", ")
}
