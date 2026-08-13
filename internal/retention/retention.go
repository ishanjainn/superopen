// Package retention prunes aged Superopen artifacts under .so/.
package retention

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/ishanjainn/superopen/internal/audit"
	"github.com/ishanjainn/superopen/internal/config"
	"github.com/ishanjainn/superopen/internal/harness"
	"github.com/ishanjainn/superopen/internal/memory"
	"github.com/ishanjainn/superopen/internal/session"
)

// Report summarizes what Prune removed.
type Report struct {
	EmptySessions   int `json:"empty_sessions"`
	ExpiredSessions int `json:"expired_sessions"`
	EvalHistory     int `json:"eval_history"`
	AuditEvents     int `json:"audit_events"`
	Recommendations int `json:"recommendations"`
	TraceFiles      int `json:"trace_files"`
}

// Prune removes empty sessions and artifacts older than cfg retention days.
func Prune(paths harness.Paths, cfg config.Config) (Report, error) {
	var rep Report
	days := cfg.RetentionDays()
	cutoff := time.Now().UTC().AddDate(0, 0, -days)

	ss := session.NewStore(paths)
	entries, err := ss.List()
	if err != nil {
		return rep, err
	}
	seen := map[string]bool{}
	for _, e := range entries {
		seen[e.ID] = true
		if ss.IsEmpty(e.ID) {
			if err := ss.Delete(e.ID); err == nil {
				_ = memory.NewStore(paths).RemoveSessionReferences(e.ID)
				rep.EmptySessions++
			}
			continue
		}
		when := e.StartedAt
		if e.EndedAt != nil && !e.EndedAt.IsZero() {
			when = *e.EndedAt
		}
		if when.IsZero() {
			if st, err := os.Stat(paths.SessionDir(e.ID)); err == nil {
				when = st.ModTime().UTC()
			}
		}
		if !when.IsZero() && when.Before(cutoff) {
			if err := ss.Delete(e.ID); err == nil {
				_ = memory.NewStore(paths).RemoveSessionReferences(e.ID)
				rep.ExpiredSessions++
			}
		}
	}
	// Orphan dirs (meta-only rows never indexed, or stale leftovers).
	if ents, err := os.ReadDir(paths.SessionsDir); err == nil {
		for _, e := range ents {
			if !e.IsDir() {
				continue
			}
			id := e.Name()
			if seen[id] || strings.HasPrefix(id, ".") {
				continue
			}
			if ss.IsEmpty(id) {
				if err := ss.Delete(id); err == nil {
					_ = memory.NewStore(paths).RemoveSessionReferences(id)
					rep.EmptySessions++
				}
				continue
			}
			st, err := e.Info()
			if err != nil {
				continue
			}
			if st.ModTime().UTC().Before(cutoff) {
				if err := ss.Delete(id); err == nil {
					_ = memory.NewStore(paths).RemoveSessionReferences(id)
					rep.ExpiredSessions++
				}
			}
		}
	}

	n, err := pruneAudit(paths, cutoff)
	if err == nil {
		rep.AuditEvents = n
	}
	return rep, nil
}

func pruneAudit(paths harness.Paths, cutoff time.Time) (int, error) {
	p := audit.Path(paths)
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	var kept [][]byte
	removed := 0
	start := 0
	for i := 0; i <= len(data); i++ {
		if i < len(data) && data[i] != '\n' {
			continue
		}
		line := data[start:i]
		start = i + 1
		if len(line) == 0 {
			continue
		}
		var ev audit.Event
		if json.Unmarshal(line, &ev) != nil {
			kept = append(kept, append([]byte(nil), line...))
			continue
		}
		if !ev.At.IsZero() && ev.At.Before(cutoff) {
			removed++
			continue
		}
		kept = append(kept, append([]byte(nil), line...))
	}
	if removed == 0 {
		return 0, nil
	}
	var b strings.Builder
	for _, line := range kept {
		b.Write(line)
		b.WriteByte('\n')
	}
	return removed, os.WriteFile(p, []byte(b.String()), 0o644)
}
