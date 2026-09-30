package rules

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ishanjainn/superopen/internal/guards"
)

// ScanSessionDir reads events.jsonl files under sessionsDir and returns findings.
// only limits the scan to one session directory name. The function does not write
// anything under sessionsDir.
func ScanSessionDir(sessionsDir, storeDir, rulesDir, only string, minRank int) ([]guards.Finding, error) {
	loaded, err := LoadActive(storeDir, rulesDir)
	if err != nil {
		return nil, err
	}
	compiled := make([]*guards.CompiledRule, 0, len(loaded))
	for _, item := range loaded {
		c, err := guards.Compile(item.Rule)
		if err != nil {
			return nil, fmt.Errorf("compile %s: %w", item.Rule.ID, err)
		}
		compiled = append(compiled, c)
	}
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var events []guards.Event
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		if only != "" && ent.Name() != only {
			continue
		}
		path := filepath.Join(sessionsDir, ent.Name(), "events.jsonl")
		spans, err := guards.ReadSpans(path)
		if err != nil {
			return nil, err
		}
		events = append(events, guards.EventsFromSpans(spans)...)
	}
	found, err := guards.ScanEvents(compiled, events)
	if err != nil {
		return nil, err
	}
	if minRank > 0 {
		found = guards.FilterBySeverity(found, minRank)
	}
	guards.SortFindings(found)
	return found, nil
}
