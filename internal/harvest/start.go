package harvest

import (
	"strings"

	"github.com/ishanjainn/superopen/internal/paths"
)

// PendingSessionStartLine is at most one pending-generate pointer.
// OPEN review stays on-demand (`so harvest review`), never SessionStart.
func PendingSessionStartLine(root string) string {
	if JevMode() {
		return ""
	}
	store, err := OpenRoot(root)
	if err != nil {
		return ""
	}
	defer store.Close()
	pending := store.PendingSession()
	if pending == "" {
		return ""
	}
	return LiveHarvestInstruction(pending)
}

// LiveHarvestInstruction tells the in-chat agent to brief, then propose or skip.
func LiveHarvestInstruction(sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ""
	}
	bin := paths.ResolveSoBin()
	return "HARVEST pending session " + sessionID + " — you are the live agent: before answering, `" + bin + " harvest brief " + sessionID + "` then `" + bin + " harvest propose` (JSON on stdin, session_id=" + sessionID + "). If that session has nothing to change, `" + bin + " harvest skip " + sessionID + "`. The current user message is a note for this turn (`" + bin + " memory capture`). It is not a playbook diff for this session. Do not write a file. Pipe JSON to propose."
}

func estTokens(s string) int {
	return (len([]rune(s)) + 3) / 4
}

func joinStart(core, extra string) string {
	extra = strings.TrimSpace(extra)
	core = strings.TrimSpace(core)
	if extra == "" {
		return core
	}
	if core == "" {
		return extra
	}
	combined := core + "\n" + extra
	if estTokens(combined) <= 350 {
		return combined
	}
	first, _, _ := strings.Cut(core, "\n")
	return strings.TrimSpace(first + "\n" + extra)
}

// JoinStart keeps a SessionStart core plus at most one extra line in budget.
func JoinStart(core, extra string) string {
	return joinStart(core, extra)
}
