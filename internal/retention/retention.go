// Package retention prunes aged Superopen artifacts under .so/.
package retention

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/superopen/so/internal/audit"
	"github.com/superopen/so/internal/config"
	"github.com/superopen/so/internal/eval"
	"github.com/superopen/so/internal/harness"
	"github.com/superopen/so/internal/recommend"
	"github.com/superopen/so/internal/session"
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
					rep.ExpiredSessions++
				}
			}
		}
	}

	n, err := pruneEvalHistory(paths.EvalsHistory, cutoff)
	if err == nil {
		rep.EvalHistory = n
	}
	n, err = pruneAudit(paths, cutoff)
	if err == nil {
		rep.AuditEvents = n
	}
	n, err = pruneRecommendations(paths, cutoff)
	if err == nil {
		rep.Recommendations = n
	}
	n, err = pruneTraceFiles(paths.TracesDir, cutoff)
	if err == nil {
		rep.TraceFiles = n
	}
	return rep, nil
}

func pruneEvalHistory(path string, cutoff time.Time) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	var hist []eval.Result
	if err := json.Unmarshal(data, &hist); err != nil {
		return 0, err
	}
	kept := make([]eval.Result, 0, len(hist))
	removed := 0
	for _, r := range hist {
		if r.At.IsZero() || !r.At.Before(cutoff) {
			kept = append(kept, r)
			continue
		}
		removed++
	}
	if removed == 0 {
		return 0, nil
	}
	out, err := json.MarshalIndent(kept, "", "  ")
	if err != nil {
		return removed, err
	}
	return removed, os.WriteFile(path, out, 0o644)
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

func pruneRecommendations(paths harness.Paths, cutoff time.Time) (int, error) {
	removed := 0
	pending, err := recommend.LoadPending(paths)
	if err != nil {
		return 0, err
	}
	keptP := make([]recommend.Recommendation, 0, len(pending))
	for _, r := range pending {
		if !r.CreatedAt.IsZero() && r.CreatedAt.Before(cutoff) {
			removed++
			continue
		}
		keptP = append(keptP, r)
	}
	if removed > 0 {
		_ = recommend.SavePending(paths, keptP)
	}

	hist, err := recommend.LoadHistory(paths)
	if err != nil {
		return removed, err
	}
	keptH := make([]recommend.Recommendation, 0, len(hist))
	hRemoved := 0
	for _, r := range hist {
		if !r.CreatedAt.IsZero() && r.CreatedAt.Before(cutoff) {
			hRemoved++
			continue
		}
		keptH = append(keptH, r)
	}
	if hRemoved > 0 {
		data, err := json.MarshalIndent(keptH, "", "  ")
		if err != nil {
			return removed + hRemoved, err
		}
		if err := os.WriteFile(paths.RecsHistory, data, 0o644); err != nil {
			return removed + hRemoved, err
		}
	}
	return removed + hRemoved, nil
}

func pruneTraceFiles(dir string, cutoff time.Time) (int, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	removed := 0
	dayCutoff := cutoff.Truncate(24 * time.Hour)
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		stem := strings.TrimSuffix(e.Name(), ".jsonl")
		if t, err := time.ParseInLocation("2006-01-02", stem, time.UTC); err == nil {
			if t.Before(dayCutoff) {
				if os.Remove(filepath.Join(dir, e.Name())) == nil {
					removed++
				}
				continue
			}
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().UTC().Before(cutoff) {
			if os.Remove(filepath.Join(dir, e.Name())) == nil {
				removed++
			}
		}
	}
	return removed, nil
}
