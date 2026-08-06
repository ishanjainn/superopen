package session

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// ComputeAttribution estimates agent vs human lines between base and head using
// a simple diffstat heuristic (not byte-perfect authorship).
func ComputeAttribution(repoRoot, baseSHA, headSHA string, agentTouched []string) AttributionSummary {
	if baseSHA == "" || headSHA == "" || baseSHA == headSHA {
		return AttributionSummary{}
	}
	cmd := exec.Command("git", "-C", repoRoot, "diff", "--numstat", baseSHA, headSHA)
	out, err := cmd.Output()
	if err != nil {
		return AttributionSummary{}
	}
	touched := map[string]bool{}
	for _, f := range agentTouched {
		touched[f] = true
	}
	agentLines, humanLines := 0, 0
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		add, _ := strconv.Atoi(fields[0])
		del, _ := strconv.Atoi(fields[1])
		path := fields[2]
		changed := add + del
		if touched[path] {
			agentLines += changed
		} else {
			humanLines += changed
		}
	}
	total := agentLines + humanLines
	pct := 0.0
	if total > 0 {
		pct = 100 * float64(agentLines) / float64(total)
	}
	sum := AttributionSummary{
		AgentPercent: pct,
		AgentLines:   agentLines,
		HumanLines:   humanLines,
		TotalChanged: total,
	}
	if total > 0 {
		sum.Display = fmt.Sprintf("%.0f%% agent (%d/%d lines)", pct, agentLines, total)
	}
	return sum
}
